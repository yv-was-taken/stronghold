package middleware

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"stronghold/internal/config"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit_BlocksAfterMax(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   5,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.Middleware())
	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// First 5 requests should succeed
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode, "Request %d should succeed", i+1)
	}

	// 6th request should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 429, resp.StatusCode)
}

func TestRateLimit_HealthExempt(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   2, // Very low limit
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.Middleware())
	app.Get("/health", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})
	app.Get("/health/live", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})
	app.Get("/health/ready", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// Health endpoints should never be rate limited
	for i := 0; i < 100; i++ {
		paths := []string{"/health", "/health/live", "/health/ready"}
		for _, path := range paths {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("X-Forwarded-For", "192.168.1.1")

			resp, err := app.Test(req)
			require.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, 200, resp.StatusCode, "Health endpoint %s should not be rate limited", path)
		}
	}
}

func TestRateLimit_AuthLimits(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   100,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	testCases := []struct {
		name     string
		path     string
		limit    int
	}{
		{"login", "/v1/auth/login", 5},
		{"account", "/v1/auth/account", 3},
		{"refresh", "/v1/auth/refresh", 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(rlm.AuthLimiter())
			app.Post(tc.path, func(c fiber.Ctx) error {
				return c.SendStatus(200)
			})

			// Requests up to limit should succeed
			for i := 0; i < tc.limit; i++ {
				req := httptest.NewRequest("POST", tc.path, nil)
				req.Header.Set("X-Forwarded-For", "10.0.0.1")

				resp, err := app.Test(req)
				require.NoError(t, err)
				resp.Body.Close()
				assert.Equal(t, 200, resp.StatusCode, "Request %d should succeed for %s", i+1, tc.path)
			}

			// Next request should be rate limited
			req := httptest.NewRequest("POST", tc.path, nil)
			req.Header.Set("X-Forwarded-For", "10.0.0.1")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 429, resp.StatusCode, "%s should be rate limited after %d requests", tc.path, tc.limit)
		})
	}
}

func TestRateLimit_RetryAfterHeader(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   1,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.Middleware())
	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// First request
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()

	// Second request should be rate limited
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 429, resp.StatusCode)

	// Retry-After header should be present
	retryAfter := resp.Header.Get("Retry-After")
	assert.NotEmpty(t, retryAfter)
}

func TestRateLimit_PerIP(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   2,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	// Configure Fiber to use X-Forwarded-For header for IP detection
	app := fiber.New(fiber.Config{
		ProxyHeader: "X-Forwarded-For",
	})
	app.Use(rlm.Middleware())
	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// IP 1: 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode)
	}

	// IP 1: 3rd request should fail
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 429, resp.StatusCode)

	// IP 2: Should have its own limit (2 requests should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.2")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode, "Different IP should have independent limit")
	}
}

func TestRateLimit_Disabled(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       false,
		WindowSeconds: 60,
		MaxRequests:   1,
		LoginMax:      1,
		AccountMax:    1,
		RefreshMax:    1,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.Middleware())
	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// Should never rate limit when disabled
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode)
	}
}

func TestRateLimit_ResponseBody(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   1,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.Middleware())
	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// First request
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()

	// Second request (rate limited)
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 429, resp.StatusCode)

	// Verify response body format
	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Contains(t, body, "error")
	assert.Contains(t, body, "message")
	assert.Contains(t, body, "retry_after")
}

func TestRateLimit_AuthLimiterPerEndpoint(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 60,
		MaxRequests:   100,
		LoginMax:      2,
		AccountMax:    2,
		RefreshMax:    2,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.AuthLimiter())
	app.Post("/v1/auth/login", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})
	app.Post("/v1/auth/account", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// Use up login limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/v1/auth/login", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode)
	}

	// Login should now be rate limited
	req := httptest.NewRequest("POST", "/v1/auth/login", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 429, resp.StatusCode)

	// But account endpoint should still work (separate limit)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/v1/auth/account", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode, "Account endpoint should have separate limit")
	}
}

func TestIsHealthEndpoint(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/health", true},
		{"/health/", true},
		{"/health/live", true},
		{"/health/ready", true},
		{"/healthcheck", true}, // Prefix match
		{"/api/health", false},
		{"/v1/health", false},
		{"/", false},
		{"/api/test", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := isHealthEndpoint(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestRateLimit_WindowExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-based test in short mode")
	}

	cfg := &config.RateLimitConfig{
		Enabled:       true,
		WindowSeconds: 1, // 1 second window
		MaxRequests:   2,
		LoginMax:      5,
		AccountMax:    3,
		RefreshMax:    10,
	}

	rlm := NewRateLimitMiddleware(cfg)

	app := fiber.New()
	app.Use(rlm.Middleware())
	app.Get("/api/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// Use up the limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := app.Test(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode)
	}

	// Should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 429, resp.StatusCode)

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	resp, err = app.Test(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
}
