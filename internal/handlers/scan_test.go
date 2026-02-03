package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"stronghold/internal/config"
	"stronghold/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockScanner implements a minimal scanner for testing
type mockScanner struct{}

func (m *mockScanner) ScanContent(text, sourceURL, sourceType, contentType string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"decision": "allow",
		"scores": map[string]float64{
			"heuristic": 0.1,
			"ml":        0.2,
		},
	}, nil
}

func TestScanContent_EmptyText(t *testing.T) {
	// Set up middleware with dev mode (no wallet = no payment required)
	x402cfg := &config.X402Config{
		WalletAddress:  "", // Dev mode
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}
	pricing := &config.PricingConfig{
		ScanContent: 0.001,
	}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	// Simple mock handler
	app.Post("/v1/scan/content", x402.AtomicPayment(0.001), func(c fiber.Ctx) error {
		requestID := middleware.GetRequestID(c)

		type ScanRequest struct {
			Text string `json:"text"`
		}

		var req ScanRequest
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":      "Invalid request body",
				"request_id": requestID,
			})
		}

		if req.Text == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":      "Text is required",
				"request_id": requestID,
			})
		}

		return c.JSON(fiber.Map{"decision": "allow"})
	})

	// Test empty text
	reqBody := map[string]string{"text": ""}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "Text is required", body["error"])
	assert.Contains(t, body, "request_id")
}

func TestScanContent_Success(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(0.001), func(c fiber.Ctx) error {
		requestID := middleware.GetRequestID(c)

		type ScanRequest struct {
			Text string `json:"text"`
		}

		var req ScanRequest
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if req.Text == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Text is required",
			})
		}

		return c.JSON(fiber.Map{
			"decision":   "allow",
			"request_id": requestID,
			"scores": map[string]float64{
				"heuristic": 0.1,
				"ml":        0.2,
			},
			"metadata": map[string]interface{}{},
		})
	})

	reqBody := map[string]string{"text": "Hello, this is safe text"}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "allow", body["decision"])
	assert.Contains(t, body, "request_id")
	assert.Contains(t, body, "scores")
	assert.Contains(t, body, "metadata")
}

func TestScanOutput_EmptyText(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/output", x402.AtomicPayment(0.001), func(c fiber.Ctx) error {
		requestID := middleware.GetRequestID(c)

		type ScanRequest struct {
			Text string `json:"text"`
		}

		var req ScanRequest
		c.Bind().Body(&req)

		if req.Text == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":      "Text is required",
				"request_id": requestID,
			})
		}

		return c.JSON(fiber.Map{"decision": "allow"})
	})

	reqBody := map[string]string{"text": ""}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/scan/output", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Equal(t, "Text is required", body["error"])
}

func TestScan_InvalidJSON(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(0.001), func(c fiber.Ctx) error {
		requestID := middleware.GetRequestID(c)

		type ScanRequest struct {
			Text string `json:"text"`
		}

		var req ScanRequest
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":      "Invalid request body",
				"request_id": requestID,
			})
		}

		return c.JSON(fiber.Map{"decision": "allow"})
	})

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
}

func TestScanContent_WithMetadata(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(0.001), func(c fiber.Ctx) error {
		requestID := middleware.GetRequestID(c)

		type ScanRequest struct {
			Text        string `json:"text"`
			SourceURL   string `json:"source_url"`
			SourceType  string `json:"source_type"`
			ContentType string `json:"content_type"`
		}

		var req ScanRequest
		c.Bind().Body(&req)

		if req.Text == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Text is required",
			})
		}

		return c.JSON(fiber.Map{
			"decision":   "allow",
			"request_id": requestID,
			"metadata": map[string]interface{}{
				"source_url":   req.SourceURL,
				"source_type":  req.SourceType,
				"content_type": req.ContentType,
			},
		})
	})

	reqBody := map[string]string{
		"text":         "Hello world",
		"source_url":   "https://example.com",
		"source_type":  "web_page",
		"content_type": "html",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	metadata := body["metadata"].(map[string]interface{})
	assert.Equal(t, "https://example.com", metadata["source_url"])
	assert.Equal(t, "web_page", metadata["source_type"])
	assert.Equal(t, "html", metadata["content_type"])
}

func TestScan_RequestIDInErrorResponse(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(0.001), func(c fiber.Ctx) error {
		requestID := middleware.GetRequestID(c)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":      "Scan failed: internal error",
			"request_id": requestID,
		})
	})

	reqBody := map[string]string{"text": "test"}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/scan/content", bytes.NewBuffer(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 500, resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	assert.Contains(t, body, "request_id")
	assert.NotEmpty(t, body["request_id"])
}
