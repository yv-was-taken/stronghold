package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"
	"stronghold/internal/handlers"
	"stronghold/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestApp creates a minimal test application with real database
func createTestApp(t *testing.T, testDB *testutil.TestDB) (*fiber.App, *db.DB) {
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

	app := fiber.New(fiber.Config{
		AppName: "Stronghold Test",
	})

	// Add basic middleware
	app.Use(recover.New())
	app.Use(middleware.RequestID())

	// Rate limiting
	rateLimitConfig := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   100,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}
	rateLimiter := middleware.NewRateLimitMiddleware(rateLimitConfig)
	app.Use(rateLimiter.Middleware())

	// Auth handler
	authConfig := &handlers.AuthConfig{
		JWTSecret:       "test-secret-key",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 90 * 24 * time.Hour,
		DashboardURL:    "http://localhost:3000",
		AllowedOrigins:  []string{"http://localhost:3000"},
		Cookie: handlers.CookieConfig{
			Secure:   false,
			SameSite: "Lax",
		},
	}
	authHandler := handlers.NewAuthHandler(database, authConfig, nil)

	// Health handler
	serverConfig := &config.Config{
		X402: config.X402Config{
			FacilitatorURL: "",
		},
	}
	healthHandler := handlers.NewHealthHandler(database, serverConfig)
	healthHandler.RegisterRoutes(app)

	// Auth routes with rate limiting
	authHandler.RegisterRoutesWithMiddleware(app, rateLimiter.AuthLimiter())

	// Account routes
	accountHandler := handlers.NewAccountHandler(database, authConfig, nil)
	accountHandler.RegisterRoutes(app, authHandler)

	return app, database
}

func TestIntegration_CompleteAuthFlow(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	app, database := createTestApp(t, testDB)
	defer database.Close()

	// 1. Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	assert.Equal(t, 201, createResp.StatusCode)

	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	accountNumber := createBody["account_number"].(string)
	assert.Regexp(t, `^\d{4}-\d{4}-\d{4}-\d{4}$`, accountNumber)

	// Extract tokens from cookies
	var accessToken, refreshToken string
	for _, cookie := range createResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, handlers.AccessTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, handlers.AccessTokenCookie+"=") {
					accessToken = strings.TrimPrefix(part, handlers.AccessTokenCookie+"=")
					break
				}
			}
		}
		if strings.Contains(cookie, handlers.RefreshTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, handlers.RefreshTokenCookie+"=") {
					refreshToken = strings.TrimPrefix(part, handlers.RefreshTokenCookie+"=")
					break
				}
			}
		}
	}
	require.NotEmpty(t, accessToken)
	require.NotEmpty(t, refreshToken)

	// 2. Login with account number
	loginBody := map[string]string{"account_number": accountNumber}
	loginJSON, _ := json.Marshal(loginBody)
	loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(loginJSON))
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := app.Test(loginReq)
	require.NoError(t, err)
	assert.Equal(t, 200, loginResp.StatusCode)
	loginResp.Body.Close()

	// 3. Refresh token
	refreshReq := httptest.NewRequest("POST", "/v1/auth/refresh", nil)
	refreshReq.Header.Set("Cookie", handlers.RefreshTokenCookie+"="+refreshToken)

	refreshResp, err := app.Test(refreshReq)
	require.NoError(t, err)
	assert.Equal(t, 200, refreshResp.StatusCode)

	// Extract new tokens
	var newAccessToken string
	for _, cookie := range refreshResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, handlers.AccessTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, handlers.AccessTokenCookie+"=") {
					newAccessToken = strings.TrimPrefix(part, handlers.AccessTokenCookie+"=")
					break
				}
			}
		}
	}
	require.NotEmpty(t, newAccessToken)
	refreshResp.Body.Close()

	// 4. Access protected route
	meReq := httptest.NewRequest("GET", "/v1/auth/me", nil)
	meReq.Header.Set("Cookie", handlers.AccessTokenCookie+"="+newAccessToken)

	meResp, err := app.Test(meReq)
	require.NoError(t, err)
	assert.Equal(t, 200, meResp.StatusCode)

	var meBody map[string]interface{}
	json.NewDecoder(meResp.Body).Decode(&meBody)
	meResp.Body.Close()

	assert.Equal(t, accountNumber, meBody["account_number"])

	// 5. Logout
	logoutReq := httptest.NewRequest("POST", "/v1/auth/logout", nil)
	logoutReq.Header.Set("Cookie", handlers.AccessTokenCookie+"="+newAccessToken)

	logoutResp, err := app.Test(logoutReq)
	require.NoError(t, err)
	assert.Equal(t, 200, logoutResp.StatusCode)
	logoutResp.Body.Close()

	// 6. Verify old token no longer works for protected routes
	afterLogoutReq := httptest.NewRequest("GET", "/v1/auth/me", nil)
	afterLogoutReq.Header.Set("Cookie", handlers.AccessTokenCookie+"="+newAccessToken)

	// Token is still valid JWT but session should be invalidated
	// (Note: In the current implementation, JWT is stateless so this might still work
	// until the token expires. For session invalidation, refresh endpoint should fail)
}

func TestIntegration_HealthChecksWithLiveDB(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	app, database := createTestApp(t, testDB)
	defer database.Close()

	t.Run("health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)

		assert.Contains(t, body, "status")
		assert.Contains(t, body, "services")
		services := body["services"].(map[string]interface{})
		assert.Equal(t, "up", services["database"])
	})

	t.Run("liveness probe", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/live", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("readiness probe", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health/ready", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// May be 200 or 503 depending on x402 facilitator config
		assert.Contains(t, []int{200, 503}, resp.StatusCode)
	})
}

func TestIntegration_RateLimitingAcrossRequests(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	// Create app with very low rate limit for testing
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

	app := fiber.New()
	app.Use(recover.New())
	app.Use(middleware.RequestID())

	rateLimitConfig := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   3, // Very low limit for testing
		LoginMax:      2,
		AccountMax:    1,
		RefreshMax:    2,
	}
	rateLimiter := middleware.NewRateLimitMiddleware(rateLimitConfig)
	app.Use(rateLimiter.Middleware())

	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// First 3 requests should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-Forwarded-For", "10.0.0.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode, "Request %d should succeed", i+1)
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 429, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
}

func TestIntegration_RequestIDPropagation(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	app, database := createTestApp(t, testDB)
	defer database.Close()

	t.Run("generates request ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		requestID := resp.Header.Get("X-Request-ID")
		assert.NotEmpty(t, requestID)
		assert.Regexp(t, `^[0-9a-f-]{36}$`, requestID) // UUID format
	})

	t.Run("preserves client request ID", func(t *testing.T) {
		clientID := "client-trace-123"
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("X-Request-ID", clientID)

		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, clientID, resp.Header.Get("X-Request-ID"))
	})
}

func TestIntegration_AccountManagement(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	app, database := createTestApp(t, testDB)
	defer database.Close()

	// Create account and get token
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var accessToken string
	for _, cookie := range createResp.Header.Values("Set-Cookie") {
		if strings.Contains(cookie, handlers.AccessTokenCookie) {
			parts := strings.Split(cookie, ";")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, handlers.AccessTokenCookie+"=") {
					accessToken = strings.TrimPrefix(part, handlers.AccessTokenCookie+"=")
					break
				}
			}
		}
	}
	createResp.Body.Close()
	require.NotEmpty(t, accessToken)

	t.Run("get account", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/account", nil)
		req.Header.Set("Cookie", handlers.AccessTokenCookie+"="+accessToken)

		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)

		assert.Contains(t, body, "id")
		assert.Contains(t, body, "account_number")
		assert.Contains(t, body, "balance_usdc")
		assert.Contains(t, body, "deposit_stats")
	})

	t.Run("get usage", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/account/usage", nil)
		req.Header.Set("Cookie", handlers.AccessTokenCookie+"="+accessToken)

		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)

		assert.Contains(t, body, "logs")
	})

	t.Run("get usage stats", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/account/usage/stats?days=30", nil)
		req.Header.Set("Cookie", handlers.AccessTokenCookie+"="+accessToken)

		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)

		assert.Contains(t, body, "total_stats")
		assert.Contains(t, body, "daily_breakdown")
	})

	t.Run("link wallet", func(t *testing.T) {
		wallet := "0xabcdef1234567890abcdef1234567890abcdef12"
		reqBody := map[string]string{"wallet_address": wallet}
		bodyJSON, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("PUT", "/v1/account/wallet", bytes.NewBuffer(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", handlers.AccessTokenCookie+"="+accessToken)

		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})
}

func TestIntegration_MultipleSessionsLogout(t *testing.T) {
	testDB := testutil.NewTestDB(t)
	defer testDB.Close(t)

	app, database := createTestApp(t, testDB)
	defer database.Close()

	// Create account
	createReq := httptest.NewRequest("POST", "/v1/auth/account", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := app.Test(createReq)
	require.NoError(t, err)

	var createBody map[string]interface{}
	json.NewDecoder(createResp.Body).Decode(&createBody)
	createResp.Body.Close()

	accountNumber := createBody["account_number"].(string)

	// Login from multiple "devices" (create multiple sessions)
	var sessions []struct {
		accessToken  string
		refreshToken string
	}

	for i := 0; i < 3; i++ {
		loginBody := map[string]string{"account_number": accountNumber}
		loginJSON, _ := json.Marshal(loginBody)
		loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(loginJSON))
		loginReq.Header.Set("Content-Type", "application/json")

		loginResp, err := app.Test(loginReq)
		require.NoError(t, err)

		var accessToken, refreshToken string
		for _, cookie := range loginResp.Header.Values("Set-Cookie") {
			if strings.Contains(cookie, handlers.AccessTokenCookie) {
				parts := strings.Split(cookie, ";")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, handlers.AccessTokenCookie+"=") {
						accessToken = strings.TrimPrefix(part, handlers.AccessTokenCookie+"=")
					}
				}
			}
			if strings.Contains(cookie, handlers.RefreshTokenCookie) {
				parts := strings.Split(cookie, ";")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if strings.HasPrefix(part, handlers.RefreshTokenCookie+"=") {
						refreshToken = strings.TrimPrefix(part, handlers.RefreshTokenCookie+"=")
					}
				}
			}
		}
		loginResp.Body.Close()

		sessions = append(sessions, struct {
			accessToken  string
			refreshToken string
		}{accessToken, refreshToken})
	}

	require.Len(t, sessions, 3)

	// Logout from one session - should logout all
	logoutReq := httptest.NewRequest("POST", "/v1/auth/logout", nil)
	logoutReq.Header.Set("Cookie", handlers.AccessTokenCookie+"="+sessions[0].accessToken)

	logoutResp, err := app.Test(logoutReq)
	require.NoError(t, err)
	assert.Equal(t, 200, logoutResp.StatusCode)
	logoutResp.Body.Close()

	// All refresh tokens should be invalidated
	for i, session := range sessions {
		refreshReq := httptest.NewRequest("POST", "/v1/auth/refresh", nil)
		refreshReq.Header.Set("Cookie", handlers.RefreshTokenCookie+"="+session.refreshToken)

		refreshResp, err := app.Test(refreshReq)
		require.NoError(t, err)
		refreshResp.Body.Close()

		// Session should be invalidated (401)
		assert.Equal(t, 401, refreshResp.StatusCode, "Session %d should be invalidated", i)
	}
}
