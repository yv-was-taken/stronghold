package middleware

import (
	"io"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_GeneratesUUID(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/test", func(c fiber.Ctx) error {
		requestID := GetRequestID(c)
		return c.SendString(requestID)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// Check response header
	headerID := resp.Header.Get(RequestIDHeader)
	assert.NotEmpty(t, headerID)

	// Verify UUID format (8-4-4-4-12)
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	assert.True(t, uuidRegex.MatchString(headerID), "Request ID should be valid UUID format, got: %s", headerID)

	// Verify body contains same ID
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, headerID, string(body))
}

func TestRequestID_PassthroughClientID(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/test", func(c fiber.Ctx) error {
		requestID := GetRequestID(c)
		return c.SendString(requestID)
	})

	clientProvidedID := "client-request-12345"
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(RequestIDHeader, clientProvidedID)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// Client-provided ID should be used
	headerID := resp.Header.Get(RequestIDHeader)
	assert.Equal(t, clientProvidedID, headerID)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, clientProvidedID, string(body))
}

func TestRequestID_ResponseHeader(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// X-Request-ID should always be in response
	headerID := resp.Header.Get(RequestIDHeader)
	assert.NotEmpty(t, headerID)
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/test", func(c fiber.Ctx) error {
		requestID := GetRequestID(c)
		return c.SendString(requestID)
	})

	var ids []string
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		resp.Body.Close()

		ids = append(ids, string(body))
	}

	// All IDs should be unique
	unique := make(map[string]bool)
	for _, id := range ids {
		assert.False(t, unique[id], "Request ID should be unique per request")
		unique[id] = true
	}
}

func TestRequestID_EmptyStringClientIDGeneratesNew(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/test", func(c fiber.Ctx) error {
		requestID := GetRequestID(c)
		return c.SendString(requestID)
	})

	// Empty string should trigger new ID generation
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(RequestIDHeader, "")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	headerID := resp.Header.Get(RequestIDHeader)
	assert.NotEmpty(t, headerID)

	// Should be a valid UUID (not empty string)
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	assert.True(t, uuidRegex.MatchString(headerID))
}

func TestGetRequestID_NoMiddleware(t *testing.T) {
	app := fiber.New()

	// No middleware - GetRequestID should return empty string
	app.Get("/test", func(c fiber.Ctx) error {
		requestID := GetRequestID(c)
		return c.SendString(requestID)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "", string(body))
}

func TestRequestID_WorksWithErrors(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/error", func(c fiber.Ctx) error {
		return c.Status(500).JSON(fiber.Map{
			"error":      "Internal server error",
			"request_id": GetRequestID(c),
		})
	})

	req := httptest.NewRequest("GET", "/error", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 500, resp.StatusCode)

	// Request ID should still be in header even on error
	headerID := resp.Header.Get(RequestIDHeader)
	assert.NotEmpty(t, headerID)
}

func TestRequestID_ClientCanTraceRequest(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())

	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	// Client sends their own tracking ID
	clientID := "trace-abc-123-xyz"
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(RequestIDHeader, clientID)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Response should echo back the client's ID for correlation
	assert.Equal(t, clientID, resp.Header.Get(RequestIDHeader))
}
