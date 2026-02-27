package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperCreateAPIKey generates a raw API key, hashes it, stores via the DB layer,
// and returns both the record and the raw key (needed for Authorization header).
func helperCreateAPIKey(t *testing.T, database *db.DB, accountID uuid.UUID, name string) (*db.APIKey, string) {
	t.Helper()

	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	require.NoError(t, err)

	rawKey := "sk_live_" + hex.EncodeToString(randomBytes)
	keyPrefix := rawKey[:12]
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	apiKey, err := database.CreateAPIKey(context.Background(), accountID, keyPrefix, keyHash, name, 10)
	require.NoError(t, err)

	return apiKey, rawKey
}

// helperCreateB2BAccount creates a B2B account for testing.
func helperCreateB2BAccount(t *testing.T, database *db.DB) *db.Account {
	t.Helper()
	account, err := database.CreateB2BAccount(context.Background(), "workos_test_"+uuid.NewString(), "test@example.com", "Test Corp")
	require.NoError(t, err)
	return account
}

// authMiddleware wraps APIKeyMiddleware.Authenticate as a fiber.Handler for test routes.
func authMiddleware(m *APIKeyMiddleware) fiber.Handler {
	return func(c fiber.Ctx) error {
		_, _, err := m.Authenticate(c)
		if err != nil {
			return err
		}
		return c.Next()
	}
}

func TestAPIKeyMiddleware_ValidKey(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := db.NewFromPool(testDB.Pool)

	account := helperCreateB2BAccount(t, database)
	_, rawKey := helperCreateAPIKey(t, database, account.ID, "test key")

	m := NewAPIKeyMiddleware(database)

	var capturedAccountID string

	app := fiber.New()
	app.Post("/test", authMiddleware(m), func(c fiber.Ctx) error {
		capturedAccountID, _ = c.Locals("account_id").(string)
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, account.ID.String(), capturedAccountID)
}

func TestAPIKeyMiddleware_MissingHeader(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := db.NewFromPool(testDB.Pool)
	m := NewAPIKeyMiddleware(database)

	app := fiber.New()
	app.Post("/test", authMiddleware(m), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

func TestAPIKeyMiddleware_InvalidKey(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := db.NewFromPool(testDB.Pool)
	m := NewAPIKeyMiddleware(database)

	app := fiber.New()
	app.Post("/test", authMiddleware(m), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Authorization", "Bearer sk_live_invalid_key_that_does_not_exist")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

func TestAPIKeyMiddleware_RevokedKey(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := db.NewFromPool(testDB.Pool)

	account := helperCreateB2BAccount(t, database)
	apiKey, rawKey := helperCreateAPIKey(t, database, account.ID, "revoked key")

	err := database.RevokeAPIKey(context.Background(), apiKey.ID, account.ID)
	require.NoError(t, err)

	m := NewAPIKeyMiddleware(database)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			message := "Internal server error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				message = e.Message
			}
			return c.Status(code).JSON(fiber.Map{"error": message})
		},
	})
	app.Post("/test", authMiddleware(m), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Contains(t, body["error"], "Invalid API key")
}

func TestAPIKeyMiddleware_EmptyBearer(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	database := db.NewFromPool(testDB.Pool)
	m := NewAPIKeyMiddleware(database)

	app := fiber.New()
	app.Post("/test", authMiddleware(m), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Authorization", "Bearer ")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}
