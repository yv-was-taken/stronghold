package handlers

import (
	"citadel-api/internal/citadel"
	"citadel-api/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// ScanHandler handles scan-related endpoints
type ScanHandler struct {
	scanner *citadel.Scanner
	x402    *middleware.X402Middleware
}

// NewScanHandler creates a new scan handler
func NewScanHandler(scanner *citadel.Scanner, x402 *middleware.X402Middleware) *ScanHandler {
	return &ScanHandler{
		scanner: scanner,
		x402:    x402,
	}
}

// ScanInputRequest represents a request to scan input
type ScanInputRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id,omitempty"`
}

// ScanOutputRequest represents a request to scan output
type ScanOutputRequest struct {
	Text string `json:"text"`
}

// ScanUnifiedRequest represents a unified scan request
type ScanUnifiedRequest struct {
	Text string `json:"text"`
	Mode string `json:"mode"` // "input", "output", or "both"
}

// ScanMultiturnRequest represents a multi-turn scan request
type ScanMultiturnRequest struct {
	SessionID string         `json:"session_id"`
	Turns     []citadel.Turn `json:"turns"`
}

// RegisterRoutes registers all scan routes
func (h *ScanHandler) RegisterRoutes(app *fiber.App) {
	group := app.Group("/v1/scan")

	// Input scanning - $0.001
	group.Post("/input", h.x402.RequirePayment(0.001), h.ScanInput)

	// Output scanning - $0.001
	group.Post("/output", h.x402.RequirePayment(0.001), h.ScanOutput)

	// Unified scanning - $0.002
	group.Post("/", h.x402.RequirePayment(0.002), h.ScanUnified)

	// Multi-turn scanning - $0.005
	group.Post("/multiturn", h.x402.RequirePayment(0.005), h.ScanMultiturn)
}

// ScanInput handles input scanning
// @Summary Scan user input for prompt injection
// @Description Scans user input text for prompt injection attacks and other security threats
// @Tags scan
// @Accept json
// @Produce json
// @Param request body ScanInputRequest true "Input scan request"
// @Success 200 {object} citadel.ScanResult
// @Failure 400 {object} map[string]string
// @Failure 402 {object} map[string]interface{}
// @Router /v1/scan/input [post]
func (h *ScanHandler) ScanInput(c fiber.Ctx) error {
	var req ScanInputRequest
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

	result, err := h.scanner.ScanInput(c.Context(), req.Text)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Scan failed: " + err.Error(),
		})
	}

	result.RequestID = uuid.New().String()
	h.x402.PaymentResponse(c, result.RequestID)

	return c.JSON(result)
}

// ScanOutput handles output scanning
// @Summary Scan LLM output for credential leaks
// @Description Scans LLM output text for credential leaks and sensitive data exposure
// @Tags scan
// @Accept json
// @Produce json
// @Param request body ScanOutputRequest true "Output scan request"
// @Success 200 {object} citadel.ScanResult
// @Failure 400 {object} map[string]string
// @Failure 402 {object} map[string]interface{}
// @Router /v1/scan/output [post]
func (h *ScanHandler) ScanOutput(c fiber.Ctx) error {
	var req ScanOutputRequest
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

	result, err := h.scanner.ScanOutput(c.Context(), req.Text)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Scan failed: " + err.Error(),
		})
	}

	result.RequestID = uuid.New().String()
	h.x402.PaymentResponse(c, result.RequestID)

	return c.JSON(result)
}

// ScanUnified handles unified scanning
// @Summary Unified content scanning
// @Description Scans content for both input and output threats based on mode
// @Tags scan
// @Accept json
// @Produce json
// @Param request body ScanUnifiedRequest true "Unified scan request"
// @Success 200 {object} citadel.ScanResult
// @Failure 400 {object} map[string]string
// @Failure 402 {object} map[string]interface{}
// @Router /v1/scan [post]
func (h *ScanHandler) ScanUnified(c fiber.Ctx) error {
	var req ScanUnifiedRequest
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

	mode := req.Mode
	if mode == "" {
		mode = "both"
	}

	if mode != "input" && mode != "output" && mode != "both" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid mode. Must be 'input', 'output', or 'both'",
		})
	}

	result, err := h.scanner.ScanUnified(c.Context(), req.Text, mode)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Scan failed: " + err.Error(),
		})
	}

	result.RequestID = uuid.New().String()
	h.x402.PaymentResponse(c, result.RequestID)

	return c.JSON(result)
}

// ScanMultiturn handles multi-turn conversation scanning
// @Summary Scan multi-turn conversations
// @Description Scans conversation history for context-aware attacks
// @Tags scan
// @Accept json
// @Produce json
// @Param request body ScanMultiturnRequest true "Multi-turn scan request"
// @Success 200 {object} citadel.ScanResult
// @Failure 400 {object} map[string]string
// @Failure 402 {object} map[string]interface{}
// @Router /v1/scan/multiturn [post]
func (h *ScanHandler) ScanMultiturn(c fiber.Ctx) error {
	var req ScanMultiturnRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.SessionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Session ID is required",
		})
	}

	if len(req.Turns) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "At least one turn is required",
		})
	}

	result, err := h.scanner.ScanMultiturn(c.Context(), req.SessionID, req.Turns)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Scan failed: " + err.Error(),
		})
	}

	result.RequestID = uuid.New().String()
	h.x402.PaymentResponse(c, result.RequestID)

	return c.JSON(result)
}
