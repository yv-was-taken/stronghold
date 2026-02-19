package handlers

import (
	"log/slog"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/middleware"
	"stronghold/internal/stronghold"

	"github.com/gofiber/fiber/v3"
)

// ScanHandler handles scan-related endpoints
type ScanHandler struct {
	scanner *stronghold.Scanner
	x402    *middleware.X402Middleware
	db      *db.DB
	pricing *config.PricingConfig
}

// NewScanHandlerWithDB creates a new scan handler with database support
func NewScanHandlerWithDB(scanner *stronghold.Scanner, x402 *middleware.X402Middleware, database *db.DB, pricing *config.PricingConfig) *ScanHandler {
	return &ScanHandler{
		scanner: scanner,
		x402:    x402,
		db:      database,
		pricing: pricing,
	}
}

// ScanContentRequest represents a request to scan external content for prompt injection
type ScanContentRequest struct {
	Text        string `json:"text"`
	SourceURL   string `json:"source_url,omitempty"`   // Where content came from (e.g., https://github.com/...)
	SourceType  string `json:"source_type,omitempty"`  // "web_page", "file", "api_response", "code_repo"
	ContentType string `json:"content_type,omitempty"` // "html", "markdown", "json", "text", "code"
	FilePath    string `json:"file_path,omitempty"`    // For file reads, e.g., "README.md"
}

// ScanOutputRequest represents a request to scan LLM/agent output for credential leaks
type ScanOutputRequest struct {
	Text string `json:"text"`
}

// RegisterRoutes registers all scan routes
func (h *ScanHandler) RegisterRoutes(app *fiber.App) {
	if h.db == nil {
		panic("scan handler requires database for atomic payment")
	}
	if h.x402 == nil {
		panic("scan handler requires x402 middleware")
	}

	group := app.Group("/v1/scan")

	// Content scanning - detect prompt injection in external content
	group.Post("/content", h.x402.AtomicPayment(h.pricing.ScanContent), h.ScanContent)

	// Output scanning - detect credential leaks in LLM responses
	group.Post("/output", h.x402.AtomicPayment(h.pricing.ScanOutput), h.ScanOutput)
}

// ScanContent handles content scanning for prompt injection
// @Summary Scan external content for prompt injection
// @Description Scans content from external sources (websites, files, APIs) for prompt injection attacks before passing to LLM
// @Tags scan
// @Accept json
// @Produce json
// @Param request body ScanContentRequest true "Content scan request"
// @Success 200 {object} stronghold.ScanResult
// @Failure 400 {object} map[string]string
// @Failure 402 {object} map[string]interface{}
// @Router /v1/scan/content [post]
func (h *ScanHandler) ScanContent(c fiber.Ctx) error {
	requestID := middleware.GetRequestID(c)

	var req ScanContentRequest
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

	// Reject oversized payloads (500KB limit)
	if len(req.Text) > 500*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":      "Text too large, maximum size is 500KB",
			"request_id": requestID,
		})
	}

	result, err := h.scanner.ScanContent(c.Context(), req.Text, req.SourceURL, req.SourceType, req.ContentType)
	if err != nil {
		slog.Error("scan content failed", "request_id", requestID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":      "Scan failed",
			"request_id": requestID,
		})
	}

	// Add source metadata to result
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["source_url"] = req.SourceURL
	result.Metadata["source_type"] = req.SourceType
	result.Metadata["content_type"] = req.ContentType
	result.Metadata["file_path"] = req.FilePath

	result.RequestID = requestID

	// Record execution result in payment transaction for idempotent replay
	h.recordExecutionResult(c, result)

	return c.JSON(result)
}

// ScanOutput handles output scanning
// @Summary Scan LLM output for credential leaks
// @Description Scans LLM output text for credential leaks and sensitive data exposure
// @Tags scan
// @Accept json
// @Produce json
// @Param request body ScanOutputRequest true "Output scan request"
// @Success 200 {object} stronghold.ScanResult
// @Failure 400 {object} map[string]string
// @Failure 402 {object} map[string]interface{}
// @Router /v1/scan/output [post]
func (h *ScanHandler) ScanOutput(c fiber.Ctx) error {
	requestID := middleware.GetRequestID(c)

	var req ScanOutputRequest
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

	// Reject oversized payloads (500KB limit)
	if len(req.Text) > 500*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":      "Text too large, maximum size is 500KB",
			"request_id": requestID,
		})
	}

	result, err := h.scanner.ScanOutput(c.Context(), req.Text)
	if err != nil {
		slog.Error("scan output failed", "request_id", requestID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":      "Scan failed",
			"request_id": requestID,
		})
	}

	result.RequestID = requestID

	// Record execution result in payment transaction for idempotent replay
	h.recordExecutionResult(c, result)

	return c.JSON(result)
}

// recordExecutionResult stores the scan result in the payment transaction for idempotent replay
func (h *ScanHandler) recordExecutionResult(c fiber.Ctx, result *stronghold.ScanResult) {
	if h.db == nil {
		return
	}

	tx := middleware.GetPaymentTransaction(c)
	if tx == nil {
		return
	}

	// Convert result to map for storage
	resultMap := map[string]interface{}{
		"request_id":         result.RequestID,
		"decision":           result.Decision,
		"scores":             result.Scores,
		"reason":             result.Reason,
		"latency_ms":         result.LatencyMs,
		"metadata":           result.Metadata,
		"sanitized_text":     result.SanitizedText,
		"threats_found":      result.ThreatsFound,
		"recommended_action": result.RecommendedAction,
	}

	if err := h.db.RecordExecution(c.Context(), tx.ID, resultMap); err != nil {
		// Log but don't fail - the result was already computed
		// The middleware will still attempt settlement
		slog.Error("failed to record execution result",
			"payment_id", tx.ID,
			"request_id", result.RequestID,
			"error", err,
		)
	}
}
