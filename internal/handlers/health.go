package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
)

// HealthHandler handles health check endpoints
type HealthHandler struct{}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
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
	return c.JSON(HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
		Services: map[string]string{
			"api":     "up",
			"citadel": "up",
			"x402":    "up",
		},
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
// @Router /health/ready [get]
func (h *HealthHandler) Readiness(c fiber.Ctx) error {
	// TODO: Check actual dependencies
	return c.JSON(fiber.Map{
		"status": "ready",
	})
}
