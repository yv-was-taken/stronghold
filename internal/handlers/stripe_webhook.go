package handlers

import (
	"log/slog"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82/webhook"
)

// webhookTimestampTolerance is the maximum age of a webhook before it's rejected
// to prevent replay attacks
const webhookTimestampTolerance = 5 * time.Minute

// StripeWebhookHandler handles Stripe webhook events
type StripeWebhookHandler struct {
	db           *db.DB
	stripeConfig *config.StripeConfig
}

// NewStripeWebhookHandler creates a new Stripe webhook handler
func NewStripeWebhookHandler(database *db.DB, stripeConfig *config.StripeConfig) *StripeWebhookHandler {
	return &StripeWebhookHandler{
		db:           database,
		stripeConfig: stripeConfig,
	}
}

// HandleWebhook handles incoming Stripe webhook events
func (h *StripeWebhookHandler) HandleWebhook(c fiber.Ctx) error {
	signature := c.Get("Stripe-Signature")
	if signature == "" {
		slog.Warn("stripe webhook missing signature header")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Missing Stripe-Signature header",
		})
	}

	body := c.Body()
	event, err := webhook.ConstructEventWithOptions(body, signature, h.stripeConfig.WebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		slog.Warn("stripe webhook signature verification failed", "error", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid signature",
		})
	}

	// Validate webhook timestamp to prevent replay attacks
	eventTime := time.Unix(event.Created, 0)
	if time.Since(eventTime) > webhookTimestampTolerance {
		slog.Warn("stripe webhook rejected: timestamp too old",
			"event_id", event.ID,
			"event_time", eventTime,
			"age", time.Since(eventTime),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Webhook timestamp too old",
		})
	}

	slog.Info("stripe webhook received", "type", event.Type, "id", event.ID)

	// Check event ID idempotency - reject duplicates
	alreadyProcessed, err := h.db.CheckAndRecordWebhookEvent(c.Context(), event.ID, string(event.Type))
	if err != nil {
		slog.Error("failed to check webhook event idempotency", "event_id", event.ID, "error", err)
		// Return 500 to trigger Stripe retry
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal error",
		})
	}
	if alreadyProcessed {
		slog.Info("duplicate stripe webhook event, skipping", "event_id", event.ID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"received":  true,
			"duplicate": true,
		})
	}

	// Route to event-specific handlers
	switch event.Type {
	case "crypto.onramp_session.updated":
		return h.handleOnrampSessionUpdated(c, event.Data.Object)
	default:
		// Return 200 for unhandled events to prevent Stripe retries
		slog.Debug("unhandled stripe webhook event", "type", event.Type)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"received": true,
		})
	}
}

// handleOnrampSessionUpdated processes crypto.onramp_session.updated events
func (h *StripeWebhookHandler) handleOnrampSessionUpdated(c fiber.Ctx, obj map[string]interface{}) error {
	// Extract fields from the parsed object map
	sessionID, _ := obj["id"].(string)
	status, _ := obj["status"].(string)

	var depositID string
	if metadata, ok := obj["metadata"].(map[string]interface{}); ok {
		depositID, _ = metadata["deposit_id"].(string)
	}

	slog.Info("processing onramp session update",
		"session_id", sessionID,
		"status", status,
		"deposit_id", depositID,
	)

	// Extract deposit ID from metadata
	if depositID == "" {
		slog.Warn("onramp session missing deposit_id in metadata", "session_id", sessionID)
		// Return 200 to prevent retries - this session wasn't created by us
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"received": true,
			"warning":  "missing deposit_id in metadata",
		})
	}

	parsedDepositID, err := uuid.Parse(depositID)
	if err != nil {
		slog.Error("invalid deposit_id in metadata", "deposit_id", depositID, "error", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid deposit_id format",
		})
	}

	ctx := c.Context()

	// Handle based on session status
	switch status {
	case "fulfillment_complete":
		// Get the deposit to check current status (idempotency)
		deposit, err := h.db.GetDepositByID(ctx, parsedDepositID)
		if err != nil {
			slog.Error("failed to get deposit", "deposit_id", parsedDepositID, "error", err)
			// Return 500 to trigger Stripe retry
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to get deposit",
			})
		}

		// Skip if already completed (idempotent)
		if deposit.Status == db.DepositStatusCompleted {
			slog.Info("deposit already completed, skipping", "deposit_id", parsedDepositID)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"received": true,
				"status":   "already_completed",
			})
		}

		// Complete the deposit and credit the account
		if err := h.db.CompleteDeposit(ctx, parsedDepositID); err != nil {
			slog.Error("failed to complete deposit", "deposit_id", parsedDepositID, "error", err)
			// Return 500 to trigger Stripe retry
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to complete deposit",
			})
		}

		slog.Info("deposit completed successfully", "deposit_id", parsedDepositID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"received": true,
			"status":   "completed",
		})

	case "rejected":
		// Mark deposit as failed
		if err := h.db.FailDeposit(ctx, parsedDepositID, "Stripe onramp session rejected"); err != nil {
			slog.Error("failed to mark deposit as failed", "deposit_id", parsedDepositID, "error", err)
			// Return 500 to trigger Stripe retry
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update deposit status",
			})
		}

		slog.Info("deposit marked as failed", "deposit_id", parsedDepositID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"received": true,
			"status":   "failed",
		})

	default:
		// Ignore intermediate states (requires_payment, fulfillment_processing, etc.)
		slog.Debug("ignoring intermediate onramp session status", "status", status, "deposit_id", parsedDepositID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"received": true,
			"status":   "ignored",
		})
	}
}
