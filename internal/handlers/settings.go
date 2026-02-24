package handlers

import (
	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// SettingsHandler handles account settings endpoints
type SettingsHandler struct {
	db *db.DB
}

// NewSettingsHandler creates a new settings handler
func NewSettingsHandler(database *db.DB) *SettingsHandler {
	return &SettingsHandler{db: database}
}

// RegisterRoutes registers settings routes
func (h *SettingsHandler) RegisterRoutes(app *fiber.App, authHandler *AuthHandler) {
	group := app.Group("/v1/account/settings")
	group.Get("/", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.GetSettings)
	group.Put("/", authHandler.AuthMiddleware(), authHandler.RequireTrustedDevice(), h.UpdateSettings)
}

// GetSettings returns the current account settings
func (h *SettingsHandler) GetSettings(c fiber.Ctx) error {
	accountIDStr, _ := c.Locals("account_id").(string)
	if accountIDStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	// Determine default: true for B2B accounts (have API keys), false otherwise
	hasKeys, err := h.db.HasActiveAPIKeys(c.Context(), accountID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to check account status",
		})
	}

	defaultEnabled := hasKeys

	enabled, err := h.db.GetJailbreakDetectionEnabled(c.Context(), accountID, defaultEnabled)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get settings",
		})
	}

	return c.JSON(fiber.Map{
		"jailbreak_detection_enabled": enabled,
		"has_api_keys":                hasKeys,
	})
}

// UpdateSettingsRequest represents a request to update account settings
type UpdateSettingsRequest struct {
	JailbreakDetectionEnabled *bool `json:"jailbreak_detection_enabled"`
}

// UpdateSettings updates the account settings
func (h *SettingsHandler) UpdateSettings(c fiber.Ctx) error {
	accountIDStr, _ := c.Locals("account_id").(string)
	if accountIDStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Invalid account ID",
		})
	}

	var req UpdateSettingsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.JailbreakDetectionEnabled != nil {
		if err := h.db.SetJailbreakDetectionEnabled(c.Context(), accountID, *req.JailbreakDetectionEnabled); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update settings",
			})
		}
	}

	// Return updated settings
	hasKeys, err := h.db.HasActiveAPIKeys(c.Context(), accountID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read updated settings",
		})
	}
	enabled, err := h.db.GetJailbreakDetectionEnabled(c.Context(), accountID, hasKeys)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read updated settings",
		})
	}

	return c.JSON(fiber.Map{
		"jailbreak_detection_enabled": enabled,
		"has_api_keys":                hasKeys,
	})
}
