package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"

	"github.com/gofiber/fiber/v3"
)

// Version is the application version, set at build time via ldflags.
var Version = "dev"

// facilitatorCache caches the result of the x402 facilitator health check
// to avoid making an external HTTP call on every health/readiness request.
var facilitatorCache struct {
	mu     sync.Mutex
	status string
	expiry time.Time
}

const facilitatorCacheTTL = 30 * time.Second

// resetFacilitatorCache clears the cached facilitator status (used in tests)
func resetFacilitatorCache() {
	facilitatorCache.mu.Lock()
	facilitatorCache.status = ""
	facilitatorCache.expiry = time.Time{}
	facilitatorCache.mu.Unlock()
}

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
		Version:   Version,
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

	// In production, readiness requires x402 payment wallets to be configured.
	if h.config != nil && h.config.IsProduction() && !h.config.X402.HasPayments() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not_ready",
			"reason": "payment_not_configured",
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

// checkX402Facilitator verifies x402 facilitator is reachable.
// Results are cached for 30 seconds to avoid per-request external HTTP calls.
func (h *HealthHandler) checkX402Facilitator() string {
	if h.config == nil || h.config.X402.FacilitatorURL == "" {
		return "not_configured"
	}

	facilitatorCache.mu.Lock()
	if time.Now().Before(facilitatorCache.expiry) {
		status := facilitatorCache.status
		facilitatorCache.mu.Unlock()
		return status
	}
	facilitatorCache.mu.Unlock()

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	var status string
	resp, err := client.Head(h.config.X402.FacilitatorURL)
	if err != nil {
		status = "unreachable"
	} else {
		resp.Body.Close()
		// Accept 2xx, 3xx, or 405 (Method Not Allowed - means server is up but doesn't support HEAD)
		if resp.StatusCode < 500 {
			status = "up"
		} else {
			status = "error"
		}
	}

	facilitatorCache.mu.Lock()
	facilitatorCache.status = status
	facilitatorCache.expiry = time.Now().Add(facilitatorCacheTTL)
	facilitatorCache.mu.Unlock()

	return status
}
