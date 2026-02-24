package middleware

import (
	"errors"
	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
)

// APIKeyMiddleware handles API key authentication for B2B clients
type APIKeyMiddleware struct {
	db *db.DB
}

// NewAPIKeyMiddleware creates a new API key middleware instance
func NewAPIKeyMiddleware(database *db.DB) *APIKeyMiddleware {
	return &APIKeyMiddleware{db: database}
}

// Authenticate returns a Fiber handler that validates the X-API-Key header.
// On success it sets c.Locals("account_id") and c.Locals("auth_method", "api_key").
func (m *APIKeyMiddleware) Authenticate() fiber.Handler {
	return func(c fiber.Ctx) error {
		rawKey := c.Get("X-API-Key")
		if rawKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "API key required",
			})
		}

		keyHash := db.HashToken(rawKey)
		apiKey, err := m.db.GetAPIKeyByHash(c.Context(), keyHash)
		if err != nil {
			if errors.Is(err, db.ErrAPIKeyNotFound) {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error": "Invalid or revoked API key",
				})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Authentication service unavailable",
			})
		}

		c.Locals("account_id", apiKey.AccountID.String())
		c.Locals("auth_method", "api_key")

		return c.Next()
	}
}
