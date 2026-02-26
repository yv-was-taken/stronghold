package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const maxAPIKeysPerAccount = 10

// APIKeyHandler handles API key management endpoints
type APIKeyHandler struct {
	db *db.DB
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(database *db.DB) *APIKeyHandler {
	return &APIKeyHandler{db: database}
}

// RegisterRoutes registers API key routes (all require JWT auth)
func (h *APIKeyHandler) RegisterRoutes(app *fiber.App, authMiddleware fiber.Handler) {
	group := app.Group("/v1/api-keys", authMiddleware)
	group.Post("/", h.Create)
	group.Get("/", h.List)
	group.Delete("/:id", h.Revoke)
}

// CreateAPIKeyRequest represents a request to create an API key
type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

// CreateAPIKeyResponse includes the full key (only returned once)
type CreateAPIKeyResponse struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	KeyPrefix string `json:"key_prefix"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// Create generates a new API key for the authenticated B2B account
func (h *APIKeyHandler) Create(c fiber.Ctx) error {
	accountID, err := h.getB2BAccountID(c)
	if err != nil {
		return err
	}

	// Parse request
	var req CreateAPIKeyRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Key name is required",
		})
	}

	// Generate key: sk_live_ + 32 random hex chars
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate API key",
		})
	}
	fullKey := "sk_live_" + hex.EncodeToString(randomBytes)
	keyPrefix := fullKey[:12] // "sk_live_xxxx"

	// Hash for storage
	hash := sha256.Sum256([]byte(fullKey))
	keyHash := hex.EncodeToString(hash[:])

	// Atomically check key cap and insert (serialized via account row lock)
	apiKey, err := h.db.CreateAPIKey(c.Context(), accountID, keyPrefix, keyHash, req.Name, maxAPIKeysPerAccount)
	if err != nil {
		if errors.Is(err, db.ErrAPIKeyLimitReached) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("Maximum of %d active API keys per account", maxAPIKeysPerAccount),
			})
		}
		slog.Error("failed to create API key", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create API key",
		})
	}

	slog.Info("API key created",
		"account_id", accountID,
		"key_id", apiKey.ID,
		"key_prefix", keyPrefix,
	)

	return c.Status(fiber.StatusCreated).JSON(CreateAPIKeyResponse{
		ID:        apiKey.ID.String(),
		Key:       fullKey,
		KeyPrefix: keyPrefix,
		Name:      req.Name,
		CreatedAt: apiKey.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// APIKeyListItem represents a key in list responses (no full key)
type APIKeyListItem struct {
	ID         string  `json:"id"`
	KeyPrefix  string  `json:"key_prefix"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

// List returns all active API keys for the authenticated B2B account
func (h *APIKeyHandler) List(c fiber.Ctx) error {
	accountID, err := h.getB2BAccountID(c)
	if err != nil {
		return err
	}

	keys, err := h.db.ListAPIKeys(c.Context(), accountID)
	if err != nil {
		slog.Error("failed to list API keys", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to list API keys",
		})
	}

	items := make([]APIKeyListItem, len(keys))
	for i, k := range keys {
		items[i] = APIKeyListItem{
			ID:        k.ID.String(),
			KeyPrefix: k.KeyPrefix,
			Name:      k.Name,
			CreatedAt: k.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if k.LastUsedAt != nil {
			t := k.LastUsedAt.Format("2006-01-02T15:04:05Z")
			items[i].LastUsedAt = &t
		}
	}

	return c.JSON(fiber.Map{
		"api_keys": items,
	})
}

// Revoke revokes an API key by ID
func (h *APIKeyHandler) Revoke(c fiber.Ctx) error {
	accountID, err := h.getB2BAccountID(c)
	if err != nil {
		return err
	}

	keyIDStr := c.Params("id")
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid key ID",
		})
	}

	if err := h.db.RevokeAPIKey(c.Context(), keyID, accountID); err != nil {
		if errors.Is(err, db.ErrAPIKeyNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "API key not found or already revoked",
			})
		}
		slog.Error("failed to revoke API key", "account_id", accountID, "key_id", keyID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to revoke API key",
		})
	}

	slog.Info("API key revoked",
		"account_id", accountID,
		"key_id", keyID,
	)

	return c.JSON(fiber.Map{
		"message": "API key revoked",
	})
}

// getB2BAccountID extracts the account ID and verifies it belongs to a B2B
// account. All API key endpoints require B2B authorization.
func (h *APIKeyHandler) getB2BAccountID(c fiber.Ctx) (uuid.UUID, error) {
	accountID, err := h.getAccountID(c)
	if err != nil {
		return uuid.UUID{}, err
	}
	account, err := h.db.GetAccountByID(c.Context(), accountID)
	if err != nil {
		if errors.Is(err, db.ErrAccountNotFound) {
			return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "Account not found")
		}
		slog.Error("failed to look up account for API key management", "account_id", accountID, "error", err)
		return uuid.UUID{}, fiber.NewError(fiber.StatusInternalServerError, "Internal server error")
	}
	if account.AccountType != db.AccountTypeB2B {
		return uuid.UUID{}, fiber.NewError(fiber.StatusForbidden, "API keys are only available for business accounts")
	}
	return accountID, nil
}

// getAccountID extracts and parses the account_id from request context.
// Returns fiber.NewError so callers always get a non-nil error on failure
// (c.Status().JSON() returns nil, which would let callers continue with uuid.Nil).
func (h *APIKeyHandler) getAccountID(c fiber.Ctx) (uuid.UUID, error) {
	accountIDStr := c.Locals("account_id")
	if accountIDStr == nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusUnauthorized, "Authentication required")
	}
	str, ok := accountIDStr.(string)
	if !ok {
		return uuid.UUID{}, fiber.NewError(fiber.StatusInternalServerError, "Invalid account ID format")
	}
	accountID, err := uuid.Parse(str)
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusInternalServerError, "Invalid account ID")
	}
	return accountID, nil
}
