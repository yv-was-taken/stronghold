package handlers

import (
	"log/slog"
	"strings"

	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// B2BAuthHandler handles B2B onboarding (post-WorkOS-authentication)
type B2BAuthHandler struct {
	db *db.DB
}

// NewB2BAuthHandler creates a new B2B auth handler
func NewB2BAuthHandler(database *db.DB) *B2BAuthHandler {
	return &B2BAuthHandler{db: database}
}

// RegisterRoutes registers B2B auth routes.
// The onboard endpoint requires WorkOS JWT auth (account_id must be set in context).
func (h *B2BAuthHandler) RegisterRoutes(app *fiber.App, authMiddleware fiber.Handler) {
	group := app.Group("/v1/auth/b2b")
	group.Use(authMiddleware)
	group.Post("/onboard", h.Onboard)
}

// OnboardRequest represents a B2B onboarding request
type OnboardRequest struct {
	CompanyName string `json:"company_name"`
}

// Onboard sets the company name for a newly provisioned B2B account.
// Called after the user's first WorkOS sign-in when company_name is missing.
func (h *B2BAuthHandler) Onboard(c fiber.Ctx) error {
	var req OnboardRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	req.CompanyName = strings.TrimSpace(req.CompanyName)
	if req.CompanyName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Company name is required",
		})
	}

	if len(req.CompanyName) > 255 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Company name must be 255 characters or less",
		})
	}

	accountIDStr, ok := c.Locals("account_id").(string)
	if !ok || accountIDStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Authentication required",
		})
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid account",
		})
	}

	// Verify the account exists and is B2B
	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		slog.Error("failed to get account for onboarding", "account_id", accountID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update account",
		})
	}

	if account.AccountType != db.AccountTypeB2B {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Onboarding is only available for business accounts",
		})
	}

	if err := h.db.UpdateCompanyName(c.Context(), accountID, req.CompanyName); err != nil {
		slog.Error("failed to update company name", "account_id", accountID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update company name",
		})
	}

	slog.Info("B2B account onboarded",
		"account_id", accountID,
		"company_name", req.CompanyName,
	)

	return c.JSON(fiber.Map{
		"account_id":   accountID.String(),
		"company_name": req.CompanyName,
	})
}
