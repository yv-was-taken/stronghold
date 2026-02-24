package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/db/testutil"
	"stronghold/internal/middleware"
	"stronghold/internal/stronghold"
	"stronghold/internal/usdc"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanContent_EmptyText(t *testing.T) {
	// Set up middleware with dev mode (no wallet = no payment required)
	x402cfg := &config.X402Config{
		EVMWalletAddress:  "", // Dev mode
		FacilitatorURL: "https://x402.org/facilitator",
		Networks:       []string{"base-sepolia"},
	}
	pricing := &config.PricingConfig{
		ScanContent: usdc.MicroUSDC(1000),
	}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	// Simple mock handler
	app.Post("/v1/scan/content", x402.AtomicPayment(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
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
		EVMWalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
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
		EVMWalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/output", x402.AtomicPayment(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
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
		EVMWalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
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
		EVMWalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
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
		EVMWalletAddress: "", // Dev mode
	}
	pricing := &config.PricingConfig{}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	app := fiber.New()
	app.Use(middleware.RequestID())

	app.Post("/v1/scan/content", x402.AtomicPayment(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
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

func TestScanHandler_RegisterRoutes_PanicsWithoutDB(t *testing.T) {
	x402cfg := &config.X402Config{
		EVMWalletAddress: "0x1234567890123456789012345678901234567890",
		FacilitatorURL:   "https://x402.org/facilitator",
		Networks:         []string{"base-sepolia"},
	}
	pricing := &config.PricingConfig{
		ScanContent: usdc.MicroUSDC(1000),
		ScanOutput:  usdc.MicroUSDC(1000),
	}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	handler := NewScanHandlerWithDB(nil, x402, nil, nil, pricing)
	app := fiber.New()

	assert.Panics(t, func() {
		handler.RegisterRoutes(app)
	})
}

func TestScanHandler_RegisterRoutes_PanicsWithoutX402(t *testing.T) {
	pricing := &config.PricingConfig{
		ScanContent: usdc.MicroUSDC(1000),
		ScanOutput:  usdc.MicroUSDC(1000),
	}

	handler := NewScanHandlerWithDB(nil, nil, nil, &db.DB{}, pricing)
	app := fiber.New()

	assert.Panics(t, func() {
		handler.RegisterRoutes(app)
	})
}

func TestScanHandler_RegisterRoutes_PanicsWithoutPricing(t *testing.T) {
	x402cfg := &config.X402Config{
		EVMWalletAddress: "0x1234567890123456789012345678901234567890",
		FacilitatorURL:   "https://x402.org/facilitator",
		Networks:         []string{"base-sepolia"},
	}
	x402 := middleware.NewX402Middleware(x402cfg, &config.PricingConfig{})

	handler := NewScanHandlerWithDB(nil, x402, nil, &db.DB{}, nil)
	app := fiber.New()

	assert.Panics(t, func() {
		handler.RegisterRoutes(app)
	})
}

func TestScanHandler_RegisterRoutes_PanicsWithoutScanner(t *testing.T) {
	x402cfg := &config.X402Config{
		EVMWalletAddress: "0x1234567890123456789012345678901234567890",
		FacilitatorURL:   "https://x402.org/facilitator",
		Networks:         []string{"base-sepolia"},
	}
	pricing := &config.PricingConfig{
		ScanContent: usdc.MicroUSDC(1000),
		ScanOutput:  usdc.MicroUSDC(1000),
	}
	x402 := middleware.NewX402Middleware(x402cfg, pricing)

	handler := NewScanHandlerWithDB(nil, x402, nil, &db.DB{}, pricing)
	app := fiber.New()

	assert.Panics(t, func() {
		handler.RegisterRoutes(app)
	})
}

// --- Jailbreak filtering tests ---

// makeScanResult builds a ScanResult with the given threats and decision.
func makeScanResult(decision stronghold.Decision, threats []stronghold.Threat) *stronghold.ScanResult {
	action := "allow"
	if decision == stronghold.DecisionBlock {
		action = "block"
	} else if decision == stronghold.DecisionWarn {
		action = "warn"
	}
	return &stronghold.ScanResult{
		Decision:          decision,
		ThreatsFound:      threats,
		RecommendedAction: action,
		Reason:            "test",
		Scores:            map[string]float64{"heuristic": 0.5},
	}
}

func TestFilterJailbreakThreats_B2C_AlwaysFilters(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionBlock, []stronghold.Threat{
		{Category: "jailbreak", Pattern: "ignore previous", Severity: "high"},
		{Category: "prompt_injection", Pattern: "evil prompt", Severity: "high"},
	})

	// Create a Fiber app and test route to get a real fiber.Ctx
	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		// Simulate B2C path (no auth_method set, which is the x402 path)
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// Jailbreak threats should be filtered, prompt_injection should remain
	assert.Len(t, result.ThreatsFound, 1)
	assert.Equal(t, "prompt_injection", result.ThreatsFound[0].Category)
	// Decision should remain BLOCK because there's still a non-jailbreak threat
	assert.Equal(t, stronghold.DecisionBlock, result.Decision)
}

func TestFilterJailbreakThreats_B2C_AllJailbreak_ResetsToAllow(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionBlock, []stronghold.Threat{
		{Category: "jailbreak", Pattern: "ignore previous", Severity: "high"},
		{Category: "jailbreak", Pattern: "DAN mode", Severity: "medium"},
	})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		// B2C path (no auth_method)
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// All threats were jailbreak, so they should all be removed
	assert.Empty(t, result.ThreatsFound)
	// Decision should reset to ALLOW
	assert.Equal(t, stronghold.DecisionAllow, result.Decision)
	assert.Equal(t, "allow", result.RecommendedAction)
	assert.Equal(t, "No actionable threats detected", result.Reason)
}

func TestFilterJailbreakThreats_B2B_DefaultEnabled_KeepsJailbreak(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)
	ctx := context.Background()

	// Create account (jailbreak detection is enabled by default for B2B)
	account, err := database.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionBlock, []stronghold.Threat{
		{Category: "jailbreak", Pattern: "ignore previous", Severity: "high"},
		{Category: "prompt_injection", Pattern: "evil prompt", Severity: "high"},
	})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		// Simulate B2B path
		c.Locals("auth_method", "api_key")
		c.Locals("account_id", account.ID.String())
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// B2B with default enabled: jailbreak threats should NOT be filtered
	assert.Len(t, result.ThreatsFound, 2)
	assert.Equal(t, stronghold.DecisionBlock, result.Decision)
}

func TestFilterJailbreakThreats_B2B_DetectionDisabled_FiltersJailbreak(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)
	ctx := context.Background()

	// Create account and disable jailbreak detection
	account, err := database.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)
	err = database.SetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionBlock, []stronghold.Threat{
		{Category: "jailbreak", Pattern: "ignore previous", Severity: "high"},
		{Category: "prompt_injection", Pattern: "evil prompt", Severity: "high"},
	})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		c.Locals("auth_method", "api_key")
		c.Locals("account_id", account.ID.String())
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// B2B with detection disabled: jailbreak threats should be filtered
	assert.Len(t, result.ThreatsFound, 1)
	assert.Equal(t, "prompt_injection", result.ThreatsFound[0].Category)
}

func TestFilterJailbreakThreats_B2B_DetectionDisabled_AllJailbreak_ResetsToAllow(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)
	ctx := context.Background()

	account, err := database.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)
	err = database.SetJailbreakDetectionEnabled(ctx, account.ID, false)
	require.NoError(t, err)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionBlock, []stronghold.Threat{
		{Category: "jailbreak", Pattern: "ignore previous", Severity: "high"},
	})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		c.Locals("auth_method", "api_key")
		c.Locals("account_id", account.ID.String())
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// All threats were jailbreak and detection is disabled: decision should reset to ALLOW
	assert.Empty(t, result.ThreatsFound)
	assert.Equal(t, stronghold.DecisionAllow, result.Decision)
	assert.Equal(t, "allow", result.RecommendedAction)
}

func TestFilterJailbreakThreats_B2B_ExplicitlyEnabled_KeepsJailbreak(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)
	ctx := context.Background()

	account, err := database.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)
	// Explicitly enable (should be same as default, but tests the explicit set path)
	err = database.SetJailbreakDetectionEnabled(ctx, account.ID, true)
	require.NoError(t, err)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionBlock, []stronghold.Threat{
		{Category: "jailbreak", Pattern: "ignore previous", Severity: "high"},
	})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		c.Locals("auth_method", "api_key")
		c.Locals("account_id", account.ID.String())
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// Jailbreak detection is enabled, so jailbreak threats should remain
	assert.Len(t, result.ThreatsFound, 1)
	assert.Equal(t, "jailbreak", result.ThreatsFound[0].Category)
	assert.Equal(t, stronghold.DecisionBlock, result.Decision)
}

func TestFilterJailbreakThreats_NoThreats_NoChange(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)

	handler := &ScanHandler{db: database}

	result := makeScanResult(stronghold.DecisionAllow, []stronghold.Threat{})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		// B2C path
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, result.ThreatsFound)
	assert.Equal(t, stronghold.DecisionAllow, result.Decision)
}

func TestFilterJailbreakThreats_B2C_WarnNoThreats_PreservesDecision(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)

	handler := &ScanHandler{db: database}

	// Simulate heuristic path: WARN decision with no specific threats listed
	result := makeScanResult(stronghold.DecisionWarn, []stronghold.Threat{})

	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		// B2C path
		handler.filterJailbreakThreats(c, result)
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Decision should stay WARN — no jailbreak threats were removed, so no reason to downgrade
	assert.Empty(t, result.ThreatsFound)
	assert.Equal(t, stronghold.DecisionWarn, result.Decision)
}

func TestDualAuth_APIKeyHeaderUsesAPIKeyAuth(t *testing.T) {
	tDB := testutil.NewTestDB(t)
	defer tDB.Close(t)

	database := db.NewFromPool(tDB.Pool)
	ctx := context.Background()

	account, err := database.CreateAccount(ctx, nil, nil)
	require.NoError(t, err)

	_, rawKey, err := database.CreateAPIKey(ctx, account.ID, "dual auth test")
	require.NoError(t, err)

	x402cfg := &config.X402Config{
		EVMWalletAddress: "0x1234567890123456789012345678901234567890",
		FacilitatorURL:   "https://x402.org/facilitator",
		Networks:         []string{"base-sepolia"},
	}
	pricing := &config.PricingConfig{
		ScanContent: usdc.MicroUSDC(1000),
	}
	x402mw := middleware.NewX402Middleware(x402cfg, pricing)
	apiKeyMw := middleware.NewAPIKeyMiddleware(database)

	handler := &ScanHandler{
		x402:       x402mw,
		apiKeyAuth: apiKeyMw,
		db:         database,
		pricing:    pricing,
	}

	var capturedAuthMethod string
	var capturedAccountID string

	app := fiber.New()
	app.Post("/test", handler.dualAuth(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
		capturedAuthMethod, _ = c.Locals("auth_method").(string)
		capturedAccountID, _ = c.Locals("account_id").(string)
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", rawKey)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "api_key", capturedAuthMethod)
	assert.Equal(t, account.ID.String(), capturedAccountID)
}

func TestDualAuth_NoAPIKeyHeaderUsesX402(t *testing.T) {
	// In dev mode (empty wallet), x402 bypasses payment
	x402cfg := &config.X402Config{
		EVMWalletAddress: "", // Dev mode
		FacilitatorURL:   "https://x402.org/facilitator",
		Networks:         []string{"base-sepolia"},
	}
	pricing := &config.PricingConfig{
		ScanContent: usdc.MicroUSDC(1000),
	}
	x402mw := middleware.NewX402Middleware(x402cfg, pricing)

	handler := &ScanHandler{
		x402:    x402mw,
		pricing: pricing,
	}

	var capturedAuthMethod string

	app := fiber.New()
	app.Post("/test", handler.dualAuth(usdc.MicroUSDC(1000)), func(c fiber.Ctx) error {
		capturedAuthMethod, _ = c.Locals("auth_method").(string)
		return c.JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	// No X-API-Key header

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	// In x402 dev mode bypass, auth_method is not set (empty string)
	assert.Equal(t, "", capturedAuthMethod)
}
