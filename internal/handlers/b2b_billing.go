package handlers

import (
	"fmt"
	"log/slog"
	"math"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/usdc"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
)

// B2BBillingHandler handles B2B billing operations
type B2BBillingHandler struct {
	db           *db.DB
	stripeConfig *config.StripeConfig
	dashboardURL string
}

// NewB2BBillingHandler creates a new B2B billing handler
func NewB2BBillingHandler(database *db.DB, stripeConfig *config.StripeConfig, dashboardURL string) *B2BBillingHandler {
	return &B2BBillingHandler{
		db:           database,
		stripeConfig: stripeConfig,
		dashboardURL: dashboardURL,
	}
}

// RegisterRoutes registers B2B billing routes (all require JWT auth)
func (h *B2BBillingHandler) RegisterRoutes(app *fiber.App, authMiddleware fiber.Handler) {
	group := app.Group("/v1/billing", authMiddleware)
	group.Post("/credits", h.PurchaseCredits)
	group.Get("/info", h.GetBillingInfo)
	group.Post("/portal", h.CreateBillingPortalSession)
}

// PurchaseCreditsRequest represents a credit purchase request
type PurchaseCreditsRequest struct {
	AmountUSDC float64 `json:"amount_usdc"`
}

// PurchaseCredits creates a Stripe Checkout session for credit purchase
func (h *B2BBillingHandler) PurchaseCredits(c fiber.Ctx) error {
	accountID, err := h.getB2BAccountID(c)
	if err != nil {
		return err
	}

	var req PurchaseCreditsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate amount ($10 min, $10,000 max)
	if req.AmountUSDC < 10.0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Minimum credit purchase is $10.00",
		})
	}
	if req.AmountUSDC > 10000.0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Maximum credit purchase is $10,000.00",
		})
	}

	// Get account for Stripe customer ID
	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	if account.StripeCustomerID == nil || *account.StripeCustomerID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No Stripe customer linked to this account. Please contact support.",
		})
	}

	// Round to whole cents and derive both values from that to prevent
	// charging less in Stripe than credited in microUSDC (e.g. 10.009)
	amountCents := int64(math.Round(req.AmountUSDC * 100))
	microUSDCAmount := usdc.MicroUSDC(amountCents * 10000) // 1 cent = 10,000 microUSDC

	// Create Stripe Checkout session
	stripe.Key = h.stripeConfig.SecretKey

	// Create a pending deposit record
	deposit := &db.Deposit{
		AccountID:  accountID,
		Provider:   db.DepositProviderStripe,
		AmountUSDC: microUSDCAmount,
		FeeUSDC:    0, // No fee for credit purchases
		Metadata: map[string]any{
			"type": "b2b_credit_purchase",
		},
	}
	if err := h.db.CreateDeposit(c.Context(), deposit); err != nil {
		slog.Error("failed to create deposit record", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to initiate purchase",
		})
	}

	params := &stripe.CheckoutSessionParams{
		Mode:     stripe.String(string(stripe.CheckoutSessionModePayment)),
		Customer: account.StripeCustomerID,
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String("usd"),
					UnitAmount: stripe.Int64(amountCents),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String("Stronghold API Credits"),
						Description: stripe.String(fmt.Sprintf("$%.2f in API credits", req.AmountUSDC)),
					},
				},
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(h.dashboardURL + "/dashboard/main/billing?session_id={CHECKOUT_SESSION_ID}&status=success"),
		CancelURL:  stripe.String(h.dashboardURL + "/dashboard/main/billing?status=cancelled"),
	}
	params.AddMetadata("account_id", accountID.String())
	params.AddMetadata("deposit_id", deposit.ID.String())
	params.AddMetadata("amount_usdc_micro", fmt.Sprintf("%d", microUSDCAmount))

	sess, err := checkoutsession.New(params)
	if err != nil {
		slog.Error("failed to create Stripe checkout session", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create checkout session",
		})
	}

	// Store checkout session ID in deposit
	if err := h.db.UpdateDepositProviderTransaction(c.Context(), deposit.ID, sess.ID); err != nil {
		slog.Error("failed to update deposit with checkout session ID", "error", err)
	}

	return c.JSON(fiber.Map{
		"checkout_url": sess.URL,
		"session_id":   sess.ID,
		"deposit_id":   deposit.ID.String(),
	})
}

// GetBillingInfo returns billing overview for the authenticated B2B account
func (h *B2BBillingHandler) GetBillingInfo(c fiber.Ctx) error {
	accountID, err := h.getB2BAccountID(c)
	if err != nil {
		return err
	}

	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	// Get recent metered usage
	usageRecords, err := h.db.GetStripeUsageRecords(c.Context(), accountID, 10, 0)
	if err != nil {
		slog.Error("failed to get usage records", "error", err)
		usageRecords = nil
	}

	return c.JSON(fiber.Map{
		"credit_balance_usdc":  account.BalanceUSDC,
		"stripe_customer_id":   account.StripeCustomerID,
		"recent_metered_usage": usageRecords,
	})
}

// CreateBillingPortalSession creates a Stripe Billing Portal session
func (h *B2BBillingHandler) CreateBillingPortalSession(c fiber.Ctx) error {
	accountID, err := h.getB2BAccountID(c)
	if err != nil {
		return err
	}

	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}

	if account.StripeCustomerID == nil || *account.StripeCustomerID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "No Stripe customer linked to this account",
		})
	}

	stripe.Key = h.stripeConfig.SecretKey

	params := &stripe.BillingPortalSessionParams{
		Customer:  account.StripeCustomerID,
		ReturnURL: stripe.String(h.dashboardURL + "/dashboard/main/billing"),
	}

	sess, err := session.New(params)
	if err != nil {
		slog.Error("failed to create billing portal session", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create billing portal session",
		})
	}

	return c.JSON(fiber.Map{
		"portal_url": sess.URL,
	})
}

// getB2BAccountID extracts account ID and verifies it's a B2B account
func (h *B2BBillingHandler) getB2BAccountID(c fiber.Ctx) (uuid.UUID, error) {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return uuid.UUID{}, c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Authentication required",
		})
	}
	accountID, err := uuid.Parse(accountIDStr.(string))
	if err != nil {
		return uuid.UUID{}, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	// Verify B2B
	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		return uuid.UUID{}, c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Account not found",
		})
	}
	if account.AccountType != db.AccountTypeB2B {
		return uuid.UUID{}, c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Billing is only available for business accounts",
		})
	}

	return accountID, nil
}
