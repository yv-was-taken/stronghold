package handlers

import (
	"context"
	"net/http"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	db     *db.DB
	config *config.Config
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(database *db.DB, cfg *config.Config) *HealthHandler {
	return &HealthHandler{
		db:     database,
		config: cfg,
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Services  map[string]string `json:"services"`
	Timestamp int64             `json:"timestamp"`
}

// RegisterRoutes registers health check routes
func (h *HealthHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/health", h.Health)
	app.Get("/health/live", h.Liveness)
	app.Get("/health/ready", h.Readiness)
}

// Health returns the full health status
// @Summary Health check
// @Description Returns the health status of the API and its dependencies
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (h *HealthHandler) Health(c fiber.Ctx) error {
	services := make(map[string]string)
	overallStatus := "healthy"

	// Check database
	dbStatus := h.checkDatabase()
	services["database"] = dbStatus
	if dbStatus != "up" {
		overallStatus = "degraded"
	}

	// Check x402 facilitator
	if x402Status := h.checkX402Facilitator(); x402Status != "up" {
		overallStatus = "degraded"
		services["x402"] = x402Status
	} else {
		services["x402"] = "up"
	}

	// API is always up if we're responding
	services["api"] = "up"

	return c.JSON(HealthResponse{
		Status:    overallStatus,
		Version:   "1.0.0",
		Services:  services,
		Timestamp: time.Now().Unix(),
	})
}

// Liveness returns liveness probe status
// @Summary Liveness probe
// @Description Kubernetes liveness probe endpoint
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health/live [get]
func (h *HealthHandler) Liveness(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "alive",
	})
}

// Readiness returns readiness probe status
// @Summary Readiness probe
// @Description Kubernetes readiness probe endpoint
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Success 503 {object} map[string]string
// @Router /health/ready [get]
func (h *HealthHandler) Readiness(c fiber.Ctx) error {
	// Check database connectivity
	if dbStatus := h.checkDatabase(); dbStatus != "up" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status":   "not_ready",
			"reason":   "database_unavailable",
			"database": dbStatus,
		})
	}

	// Check x402 facilitator reachability
	if x402Status := h.checkX402Facilitator(); x402Status != "up" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not_ready",
			"reason": "x402_unavailable",
			"x402":   x402Status,
		})
	}

	return c.JSON(fiber.Map{
		"status": "ready",
	})
}

// checkDatabase verifies database connectivity
func (h *HealthHandler) checkDatabase() string {
	if h.db == nil {
		return "not_configured"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		return "down"
	}
	return "up"
}

// checkX402Facilitator verifies x402 facilitator is reachable
func (h *HealthHandler) checkX402Facilitator() string {
	if h.config == nil || h.config.X402.FacilitatorURL == "" {
		return "not_configured"
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	resp, err := client.Head(h.config.X402.FacilitatorURL)
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()

	// Accept 2xx, 3xx, or 405 (Method Not Allowed - means server is up but doesn't support HEAD)
	if resp.StatusCode < 500 {
		return "up"
	}
	return "error"
}
