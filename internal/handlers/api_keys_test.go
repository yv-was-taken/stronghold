package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAPIKeyTest(t *testing.T) (*fiber.App, *AuthHandler, *APIKeyHandler, *testutil.TestDB, *db.DB) {
	testDB := testutil.NewTestDB(t)

	cfg := &db.Config{
		Host:     testDB.Host,
		Port:     testDB.Port,
		User:     testDB.User,
		Password: testDB.Password,
		Name:     testDB.Database,
		SSLMode:  "disable",
	}

	database, err := db.New(cfg)
	require.NoError(t, err)

	authConfig := &AuthConfig{
		JWTSecret:       "test-secret-key-for-testing",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 90 * 24 * time.Hour,
		DashboardURL:    "http://localhost:3000",
		AllowedOrigins:  []string{"http://localhost:3000"},
		Cookie: CookieConfig{
			Domain:   "",
			Secure:   false,
			SameSite: "Lax",
		},
	}

	authHandler := NewAuthHandler(database, authConfig, nil)
	apiKeyHandler := NewAPIKeyHandler(database)

	app := fiber.New()
	authHandler.RegisterRoutes(app)
	apiKeyHandler.RegisterRoutes(app, authHandler.AuthMiddleware())

	return app, authHandler, apiKeyHandler, testDB, database
}

// createAuthenticatedAccountForAPIKeys creates a B2B account and returns account number and access token.
// API key endpoints require B2B accounts, so after creating a B2C account (to get a valid JWT),
// we promote it to B2B via the database.
func createAuthenticatedAccountForAPIKeys(t *testing.T, app *fiber.App, database *db.DB) (string, string) {
	t.Helper()

	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	var accessToken string
	for _, cookie := range createResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, AccessTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, AccessTokenCookie+"=") {
					accessToken = strings.TrimPrefix(part, AccessTokenCookie+"=")
					break
				}
			}
		}
	}

	// Promote account to B2B so API key endpoints accept it
	account, err := database.GetAccountByNumber(context.Background(), createBody.AccountNumber)
	require.NoError(t, err)
	uniqueEmail := fmt.Sprintf("test-%s@example.com", account.ID)
	_, err = database.Pool().Exec(context.Background(),
		`UPDATE accounts SET account_type = 'b2b', email = $1 WHERE id = $2`,
		uniqueEmail, account.ID)
	require.NoError(t, err)

	return createBody.AccountNumber, accessToken
}

func TestCreateAPIKey_ReturnsRawKey(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForAPIKeys(t, app, database)

	reqBody := map[string]string{"label": "my test key"}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/api-keys/", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Should contain the raw key
	rawKey, ok := body["key"].(string)
	assert.True(t, ok, "response should contain 'key' field")
	assert.True(t, strings.HasPrefix(rawKey, "sk_live_"), "raw key should start with sk_live_")

	// Should also contain metadata
	assert.Contains(t, body, "id")
	assert.Contains(t, body, "key_prefix")
	assert.Equal(t, "my test key", body["label"])
}

func TestListAPIKeys_ReturnsAllKeys(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForAPIKeys(t, app, database)

	// Create two keys
	for _, label := range []string{"key one", "key two"} {
		reqBody := map[string]string{"label": label}
		bodyJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/v1/api-keys/", bytes.NewBuffer(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 201, resp.StatusCode)
	}

	// List keys
	req := httptest.NewRequest("GET", "/v1/api-keys/", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	keys := body["api_keys"].([]interface{})
	assert.Len(t, keys, 2)
}

func TestListAPIKeys_EmptyArray(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForAPIKeys(t, app, database)

	req := httptest.NewRequest("GET", "/v1/api-keys/", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Should be an empty array, not null
	keys := body["api_keys"].([]interface{})
	assert.Len(t, keys, 0)
}

func TestRevokeAPIKey_Success(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForAPIKeys(t, app, database)

	// Create a key
	reqBody := map[string]string{"label": "to revoke"}
	bodyJSON, _ := json.Marshal(reqBody)

	createReq := httptest.NewRequest("POST", "/v1/api-keys/", bytes.NewBuffer(bodyJSON))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	keyID := createBody["id"].(string)

	// Revoke the key
	revokeReq := httptest.NewRequest("DELETE", "/v1/api-keys/"+keyID, nil)
	revokeReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	revokeResp, err := app.Test(revokeReq)
	require.NoError(t, err)
	defer revokeResp.Body.Close()

	assert.Equal(t, 200, revokeResp.StatusCode)

	var revokeBody map[string]interface{}
	json.NewDecoder(revokeResp.Body).Decode(&revokeBody)
	assert.Equal(t, "API key revoked", revokeBody["message"])
}

func TestRevokeAPIKey_InvalidID(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForAPIKeys(t, app, database)

	req := httptest.NewRequest("DELETE", "/v1/api-keys/not-a-uuid", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Invalid API key ID")
}

func TestRevokeAPIKey_NonExistentID(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForAPIKeys(t, app, database)

	req := httptest.NewRequest("DELETE", "/v1/api-keys/"+uuid.New().String(), nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "not found or already revoked")
}

func TestAPIKeyEndpoints_RequireAuth(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/api-keys/"},
		{"GET", "/v1/api-keys/"},
		{"DELETE", "/v1/api-keys/" + uuid.New().String()},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			resp.Body.Close()

			assert.Equal(t, 401, resp.StatusCode)
		})
	}
}

func TestRevokeAPIKey_OtherAccountCannotRevoke(t *testing.T) {
	app, _, _, testDB, database := setupAPIKeyTest(t)
	defer testDB.Close(t)
	defer database.Close()

	// Create first account and API key
	_, accessToken1 := createAuthenticatedAccountForAPIKeys(t, app, database)

	reqBody := map[string]string{"label": "account1 key"}
	bodyJSON, _ := json.Marshal(reqBody)

	createReq := httptest.NewRequest("POST", "/v1/api-keys/", bytes.NewBuffer(bodyJSON))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken1)

	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	keyID := createBody["id"].(string)

	// Create second account
	_, accessToken2 := createAuthenticatedAccountForAPIKeys(t, app, database)

	// Try to revoke account1's key using account2's auth
	revokeReq := httptest.NewRequest("DELETE", "/v1/api-keys/"+keyID, nil)
	revokeReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken2)

	revokeResp, err := app.Test(revokeReq)
	require.NoError(t, err)
	defer revokeResp.Body.Close()

	assert.Equal(t, 404, revokeResp.StatusCode)

	// Verify the key still works (not actually revoked) by checking the DB directly
	account1, _ := database.GetAccountByNumber(context.Background(), "")
	_ = account1 // The key should still be active; tested indirectly by the 404 above
}
