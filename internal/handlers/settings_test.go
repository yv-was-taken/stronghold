package handlers

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stronghold/internal/db"
	"stronghold/internal/db/testutil"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSettingsTest(t *testing.T) (*fiber.App, *AuthHandler, *SettingsHandler, *testutil.TestDB, *db.DB) {
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
	settingsHandler := NewSettingsHandler(database)

	app := fiber.New()
	authHandler.RegisterRoutes(app)
	settingsHandler.RegisterRoutes(app, authHandler)

	return app, authHandler, settingsHandler, testDB, database
}

func createAuthenticatedAccountForSettings(t *testing.T, app *fiber.App) (string, string) {
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

	return createBody.AccountNumber, accessToken
}

func TestGetSettings_DefaultForNewAccount(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForSettings(t, app)

	req := httptest.NewRequest("GET", "/v1/account/settings/", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// New account without API keys: default should be false (no API keys = B2C default)
	assert.Equal(t, false, body["jailbreak_detection_enabled"])
	assert.Equal(t, false, body["has_api_keys"])
}

func TestGetSettings_DefaultTrueWhenHasAPIKeys(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	accountNumber, accessToken := createAuthenticatedAccountForSettings(t, app)

	// Create an API key directly in DB
	account, err := database.GetAccountByNumber(t.Context(), accountNumber)
	require.NoError(t, err)
	randomBytes := make([]byte, 16)
	_, err = rand.Read(randomBytes)
	require.NoError(t, err)
	rawKey := "sk_live_" + hex.EncodeToString(randomBytes)
	keyHash := sha256.Sum256([]byte(rawKey))
	_, err = database.CreateAPIKey(t.Context(), account.ID, rawKey[:12], hex.EncodeToString(keyHash[:]), "test key", 10)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/v1/account/settings/", nil)
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Account with API keys: default should be true (B2B default)
	assert.Equal(t, true, body["jailbreak_detection_enabled"])
	assert.Equal(t, true, body["has_api_keys"])
}

func TestUpdateSettings_JailbreakDetectionEnabled(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForSettings(t, app)

	// Update to true
	updateBody := map[string]bool{"jailbreak_detection_enabled": true}
	bodyJSON, _ := json.Marshal(updateBody)

	req := httptest.NewRequest("PUT", "/v1/account/settings/", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, true, body["jailbreak_detection_enabled"])
}

func TestGetSettings_ReflectsUpdatedValue(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForSettings(t, app)

	// Set to true
	updateBody := map[string]bool{"jailbreak_detection_enabled": true}
	bodyJSON, _ := json.Marshal(updateBody)

	updateReq := httptest.NewRequest("PUT", "/v1/account/settings/", bytes.NewBuffer(bodyJSON))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	updateResp, err := app.Test(updateReq)
	require.NoError(t, err)
	updateResp.Body.Close()

	// GET should reflect the updated value
	getReq := httptest.NewRequest("GET", "/v1/account/settings/", nil)
	getReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, 200, getResp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(getResp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, true, body["jailbreak_detection_enabled"])
}

func TestUpdateSettings_Toggle(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForSettings(t, app)

	// Set to true
	bodyJSON, _ := json.Marshal(map[string]bool{"jailbreak_detection_enabled": true})
	req := httptest.NewRequest("PUT", "/v1/account/settings/", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)
	resp, _ := app.Test(req)
	resp.Body.Close()

	// Set to false
	bodyJSON, _ = json.Marshal(map[string]bool{"jailbreak_detection_enabled": false})
	req = httptest.NewRequest("PUT", "/v1/account/settings/", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)
	resp, _ = app.Test(req)
	resp.Body.Close()

	// Verify it's false
	getReq := httptest.NewRequest("GET", "/v1/account/settings/", nil)
	getReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	getResp, err := app.Test(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(getResp.Body).Decode(&body)
	assert.Equal(t, false, body["jailbreak_detection_enabled"])
}

func TestSettingsEndpoints_RequireAuth(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/account/settings/"},
		{"PUT", "/v1/account/settings/"},
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

func TestUpdateSettings_EmptyBodyIsNoop(t *testing.T) {
	app, _, _, testDB, database := setupSettingsTest(t)
	defer testDB.Close(t)
	defer database.Close()

	_, accessToken := createAuthenticatedAccountForSettings(t, app)

	// Send empty body (no jailbreak_detection_enabled field)
	req := httptest.NewRequest("PUT", "/v1/account/settings/", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	// Should still return current settings (default false for no API keys)
	assert.Equal(t, false, body["jailbreak_detection_enabled"])
}
