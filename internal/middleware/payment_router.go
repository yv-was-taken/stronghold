package middleware

import (
	"errors"
	"log/slog"
	"strings"

	"stronghold/internal/billing"
	"stronghold/internal/db"
	"stronghold/internal/usdc"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// PaymentRouter routes payment handling between x402 (B2C) and API key (B2B) authentication.
// For scan endpoints, it checks:
// 1. X-PAYMENT header → delegate to x402 AtomicPayment
// 2. Authorization: Bearer sk_live_... → API key auth with credit deduction or metered billing
// 3. Neither → return 402 Payment Required
type PaymentRouter struct {
	x402   *X402Middleware
	apiKey *APIKeyMiddleware
	meter  *billing.MeterReporter
	db     *db.DB
}

// NewPaymentRouter creates a new payment router
func NewPaymentRouter(x402 *X402Middleware, apiKey *APIKeyMiddleware, meter *billing.MeterReporter, database *db.DB) *PaymentRouter {
	return &PaymentRouter{
		x402:   x402,
		apiKey: apiKey,
		meter:  meter,
		db:     database,
	}
}

// Route returns middleware that handles payment for the given price.
// It accepts either x402 crypto payment OR B2B API key authentication.
func (pr *PaymentRouter) Route(price usdc.MicroUSDC) fiber.Handler {
	// Pre-build the x402 handler for this price
	x402Handler := pr.x402.AtomicPayment(price)

	return func(c fiber.Ctx) error {
		// Path 1: x402 crypto payment (X-PAYMENT header present)
		if c.Get("X-Payment") != "" {
			return x402Handler(c)
		}

		// Path 2: API key authentication (Bearer sk_live_...)
		authHeader := string(c.Request().Header.Peek("Authorization"))
		if authHeader != "" {
			// Parse scheme case-insensitively (RFC 7235)
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && strings.HasPrefix(parts[1], "sk_live_") {
				return pr.handleAPIKeyPayment(c, price)
			}
		}

		// Path 3: Neither header present → delegate to x402 handler.
		// In dev mode (no payment networks configured), x402 passes through.
		// In production, x402 returns its own 402 with payment instructions.
		return x402Handler(c)
	}
}

// handleAPIKeyPayment authenticates via API key and handles billing (credits or metered).
// Billing is deferred until after the handler succeeds to avoid charging for failed requests.
func (pr *PaymentRouter) handleAPIKeyPayment(c fiber.Ctx, price usdc.MicroUSDC) error {
	// Authenticate API key
	account, _, err := pr.apiKey.Authenticate(c)
	if err != nil {
		return err
	}

	// Pre-check: verify the account has a way to pay before running the handler
	hasCredits := account.BalanceUSDC >= price
	hasMetered := pr.meter != nil && pr.meter.IsConfigured() && account.StripeCustomerID != nil && *account.StripeCustomerID != ""

	if !hasCredits && !hasMetered {
		return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
			"error":   "Insufficient credits",
			"message": "Your credit balance is insufficient. Purchase credits at /v1/billing/credits.",
		})
	}

	// Execute the handler BEFORE charging
	if err := c.Next(); err != nil {
		return err
	}

	// Only charge on successful responses (2xx)
	status := c.Response().StatusCode()
	if status < 200 || status >= 300 {
		return nil
	}

	// Try deducting from credit balance (atomic SQL)
	deducted, err := pr.db.DeductBalance(c.Context(), account.ID, price)
	if err != nil {
		slog.Error("failed to deduct balance", "account_id", account.ID, "error", err)
		c.Response().Reset()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Payment processing error",
		})
	}

	if deducted {
		pr.logUsage(c, account.ID, price, "credits")
		return nil
	}

	// Fall back to metered billing.
	// Each request gets a unique server-issued identifier. No deduplication is
	// intended — the scan runs on every request (including retries), so each
	// execution is a distinct billable event.
	if hasMetered {
		meterKey := uuid.New().String()
		if err := pr.meter.ReportUsage(c.Context(), account.ID, *account.StripeCustomerID, c.Path(), price, meterKey); err != nil {
			slog.Error("metered billing failed", "account_id", account.ID, "error", err)
			c.Response().Reset()
			if errors.Is(err, billing.ErrMeteringNotConfigured) {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Billing configuration error",
				})
			}
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Billing service temporarily unavailable. Please try again.",
			})
		}
		pr.logUsage(c, account.ID, price, "metered")
		return nil
	}

	// Neither credits nor metered billing succeeded (concurrent race on last credit)
	slog.Warn("billing race: scan succeeded but no payment method available",
		"account_id", account.ID)
	c.Response().Reset()
	return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
		"error":   "Payment failed",
		"message": "Unable to process payment. Please try again.",
	})
}

// logUsage creates a usage log entry for a B2B API request.
// CostUSDC is set to 0 in the DB row to prevent the deduct_account_balance_on_usage
// trigger from subtracting again — B2B balance changes are handled by DeductBalance
// (credits) or Stripe (metered). The actual cost is recorded in metadata for auditing.
func (pr *PaymentRouter) logUsage(c fiber.Ctx, accountID uuid.UUID, price usdc.MicroUSDC, paymentMethod string) {
	requestID := GetRequestID(c)
	usageLog := &db.UsageLog{
		AccountID: accountID,
		RequestID: requestID,
		Endpoint:  c.Path(),
		Method:    c.Method(),
		CostUSDC:  0, // trigger-safe: DeductBalance or Stripe already handled billing
		Status:    "success",
		Metadata: map[string]any{
			"payment_method": paymentMethod,
			"account_type":   "b2b",
			"actual_cost":    price,
		},
	}

	if err := pr.db.CreateUsageLog(c.Context(), usageLog); err != nil {
		slog.Error("failed to create usage log", "account_id", accountID, "error", err)
	}
}
