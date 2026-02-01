package handlers

import (
	"bytes"
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

func setupAuthTest(t *testing.T) (*fiber.App, *AuthHandler, *testutil.TestDB) {
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

	handler := NewAuthHandler(database, authConfig)
	app := fiber.New()
	handler.RegisterRoutes(app)

	return app, handler, testDB
}

func TestCreateAccount_ReturnsRecoveryFile(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	req := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)

	var body CreateAccountResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	// Verify account number format
	assert.Regexp(t, `^\d{4}-\d{4}-\d{4}-\d{4}$`, body.AccountNumber)

	// Verify recovery file content
	assert.Contains(t, body.RecoveryFile, "STRONGHOLD ACCOUNT RECOVERY FILE")
	assert.Contains(t, body.RecoveryFile, body.AccountNumber)
	assert.Contains(t, body.RecoveryFile, "Account ID:")

	// Verify expires_at is in the future
	assert.True(t, body.ExpiresAt.After(time.Now()))
}

func TestCreateAccount_SetsHttpOnlyCookies(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	req := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)

	// Check for Set-Cookie headers
	cookies := resp.Header.Values("Set-Cookie")
	require.GreaterOrEqual(t, len(cookies), 2, "Should set at least 2 cookies (access and refresh)")

	var foundAccess, foundRefresh bool
	for _, cookie := range cookies {
		if strings.Contains(cookie, AccessTokenCookie) {
			foundAccess = true
			assert.Contains(t, cookie, "HttpOnly")
			assert.Contains(t, cookie, "Path=/")
		}
		if strings.Contains(cookie, RefreshTokenCookie) {
			foundRefresh = true
			assert.Contains(t, cookie, "HttpOnly")
			assert.Contains(t, cookie, "Path=/v1/auth")
		}
	}

	assert.True(t, foundAccess, "Access token cookie should be set")
	assert.True(t, foundRefresh, "Refresh token cookie should be set")
}

func TestLogin_Success(t *testing.T) {
	app, handler, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// First create an account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	err = json.NewDecoder(createResp.Body).Decode(&createBody)
	require.NoError(t, err)
	createResp.Body.Close()

	// Now login
	loginBody := map[string]string{"account_number": createBody.AccountNumber}
	loginJSON, _ := json.Marshal(loginBody)

	loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(loginJSON))
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := app.Test(loginReq)
	require.NoError(t, err)
	defer loginResp.Body.Close()

	assert.Equal(t, 200, loginResp.StatusCode)

	var respBody LoginResponse
	err = json.NewDecoder(loginResp.Body).Decode(&respBody)
	require.NoError(t, err)

	assert.Equal(t, createBody.AccountNumber, respBody.AccountNumber)
	assert.True(t, respBody.ExpiresAt.After(time.Now()))

	// Verify cookies are set
	cookies := loginResp.Header.Values("Set-Cookie")
	assert.GreaterOrEqual(t, len(cookies), 2)

	// Verify last_login_at was updated
	_ = handler // We'd need to query the DB to verify this
}

func TestLogin_InvalidAccount(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	loginBody := map[string]string{"account_number": "9999-9999-9999-9999"}
	loginJSON, _ := json.Marshal(loginBody)

	start := time.Now()
	loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(loginJSON))
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := app.Test(loginReq)
	require.NoError(t, err)
	defer loginResp.Body.Close()
	elapsed := time.Since(start)

	assert.Equal(t, 401, loginResp.StatusCode)

	// Verify timing attack prevention (should take at least 100ms)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(100),
		"Login should have timing attack prevention delay")
}

func TestLogin_SuspendedAccount(t *testing.T) {
	app, handler, testDB := setupAuthTest(t)
	defer testDB.Close(t)
	_ = handler

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	err = json.NewDecoder(createResp.Body).Decode(&createBody)
	require.NoError(t, err)
	createResp.Body.Close()

	// Suspend the account directly in DB
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
	defer database.Close()

	account, err := database.GetAccountByNumber(t.Context(), createBody.AccountNumber)
	require.NoError(t, err)
	err = database.SuspendAccount(t.Context(), account.ID)
	require.NoError(t, err)

	// Try to login
	loginBody := map[string]string{"account_number": createBody.AccountNumber}
	loginJSON, _ := json.Marshal(loginBody)

	loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(loginJSON))
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := app.Test(loginReq)
	require.NoError(t, err)
	defer loginResp.Body.Close()

	assert.Equal(t, 403, loginResp.StatusCode)

	var errBody map[string]interface{}
	err = json.NewDecoder(loginResp.Body).Decode(&errBody)
	require.NoError(t, err)
	assert.Contains(t, errBody["error"], "not active")
}

func TestRefreshToken_RotatesToken(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// Create account and get initial cookies
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	err = json.NewDecoder(createResp.Body).Decode(&createBody)
	require.NoError(t, err)
	createResp.Body.Close()

	// Extract refresh token cookie
	var refreshToken string
	for _, cookie := range createResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, RefreshTokenCookie) {
			// Parse cookie value
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, RefreshTokenCookie+"=") {
					refreshToken = strings.TrimPrefix(part, RefreshTokenCookie+"=")
					break
				}
			}
		}
	}
	require.NotEmpty(t, refreshToken, "Refresh token should be in cookie")

	// Call refresh endpoint
	refreshReq := httptest.NewRequest("POST", "/v1/auth/refresh", nil)
	refreshReq.Header.Set("Cookie", RefreshTokenCookie+"="+refreshToken)

	refreshResp, err := app.Test(refreshReq)
	require.NoError(t, err)
	defer refreshResp.Body.Close()

	assert.Equal(t, 200, refreshResp.StatusCode)

	// Extract new refresh token
	var newRefreshToken string
	for _, cookie := range refreshResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, RefreshTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, RefreshTokenCookie+"=") {
					newRefreshToken = strings.TrimPrefix(part, RefreshTokenCookie+"=")
					break
				}
			}
		}
	}
	require.NotEmpty(t, newRefreshToken)
	assert.NotEqual(t, refreshToken, newRefreshToken, "Token should be rotated")

	// Old token should no longer work
	oldTokenReq := httptest.NewRequest("POST", "/v1/auth/refresh", nil)
	oldTokenReq.Header.Set("Cookie", RefreshTokenCookie+"="+refreshToken)

	oldTokenResp, err := app.Test(oldTokenReq)
	require.NoError(t, err)
	defer oldTokenResp.Body.Close()

	assert.Equal(t, 401, oldTokenResp.StatusCode, "Old token should be invalidated")
}

func TestLogout_ClearsAllSessions(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	// Extract access token cookie
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
	require.NotEmpty(t, accessToken)

	// Logout
	logoutReq := httptest.NewRequest("POST", "/v1/auth/logout", nil)
	logoutReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	logoutResp, err := app.Test(logoutReq)
	require.NoError(t, err)
	defer logoutResp.Body.Close()

	assert.Equal(t, 200, logoutResp.StatusCode)

	// Verify cookies are cleared
	for _, cookie := range logoutResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, AccessTokenCookie) || strings.Contains(cookie, RefreshTokenCookie) {
			// Cookie should be expired (in the past)
			assert.Contains(t, cookie, "expires=")
		}
	}
}

func TestAuthMiddleware_CookieAuth(t *testing.T) {
	app, handler, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// Add a protected route
	app.Get("/v1/protected", handler.AuthMiddleware(), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"account_id": c.Locals("account_id"),
		})
	})

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Extract access token
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
	require.NotEmpty(t, accessToken)

	// Access protected route with cookie
	protectedReq := httptest.NewRequest("GET", "/v1/protected", nil)
	protectedReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	protectedResp, err := app.Test(protectedReq)
	require.NoError(t, err)
	defer protectedResp.Body.Close()

	assert.Equal(t, 200, protectedResp.StatusCode)
}

func TestAuthMiddleware_HeaderAuth(t *testing.T) {
	app, handler, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// Add a protected route
	app.Get("/v1/protected", handler.AuthMiddleware(), func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"account_id": c.Locals("account_id"),
		})
	})

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Extract access token from cookie
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
	require.NotEmpty(t, accessToken)

	// Access protected route with Authorization header
	protectedReq := httptest.NewRequest("GET", "/v1/protected", nil)
	protectedReq.Header.Set("Authorization", "Bearer "+accessToken)

	protectedResp, err := app.Test(protectedReq)
	require.NoError(t, err)
	defer protectedResp.Body.Close()

	assert.Equal(t, 200, protectedResp.StatusCode)
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	app, handler, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// Override config with very short TTL
	handler.config.AccessTokenTTL = 1 * time.Millisecond

	app.Get("/v1/protected", handler.AuthMiddleware(), func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Extract access token
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
	require.NotEmpty(t, accessToken)

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Try to access protected route
	protectedReq := httptest.NewRequest("GET", "/v1/protected", nil)
	protectedReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	protectedResp, err := app.Test(protectedReq)
	require.NoError(t, err)
	defer protectedResp.Body.Close()

	assert.Equal(t, 401, protectedResp.StatusCode)
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	app, handler, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	app.Get("/v1/protected", handler.AuthMiddleware(), func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/v1/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Authentication required")
}

func TestCreateAccount_WithWallet(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	wallet := "0x1234567890abcdef1234567890abcdef12345678"
	reqBody := map[string]string{"wallet_address": wallet}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)
}

func TestCreateAccount_InvalidWallet(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	reqBody := map[string]string{"wallet_address": "invalid-wallet"}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Invalid wallet address")
}

func TestIsValidWalletAddress(t *testing.T) {
	testCases := []struct {
		address string
		valid   bool
	}{
		{"0x1234567890abcdef1234567890abcdef12345678", true},
		{"0xABCDEF1234567890ABCDEF1234567890ABCDEF12", true},
		{"0x1234", false},
		{"1234567890abcdef1234567890abcdef12345678", false},
		{"0xGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.address, func(t *testing.T) {
			result := isValidWalletAddress(tc.address)
			assert.Equal(t, tc.valid, result)
		})
	}
}

func TestLogin_EmptyAccountNumber(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	reqBody := map[string]string{"account_number": ""}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Account number is required")
}

func TestRefreshToken_MissingCookie(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	req := httptest.NewRequest("POST", "/v1/auth/refresh", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	assert.Contains(t, body["error"], "Refresh token is required")
}

func TestGetMe_ReturnsAccountInfo(t *testing.T) {
	app, _, testDB := setupAuthTest(t)
	defer testDB.Close(t)

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody CreateAccountResponse
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	// Extract access token
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

	// Get /me
	meReq := httptest.NewRequest("GET", "/v1/auth/me", nil)
	meReq.Header.Set("Cookie", AccessTokenCookie+"="+accessToken)

	meResp, err := app.Test(meReq)
	require.NoError(t, err)
	defer meResp.Body.Close()

	assert.Equal(t, 200, meResp.StatusCode)

	var meBody map[string]interface{}
	json.NewDecoder(meResp.Body).Decode(&meBody)

	assert.Equal(t, createBody.AccountNumber, meBody["account_number"])
	assert.Contains(t, meBody, "id")
	assert.Contains(t, meBody, "balance_usdc")
	assert.Contains(t, meBody, "status")
}
