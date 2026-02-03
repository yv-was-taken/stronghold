package handlers

import (
	"stronghold/internal/middleware"

	"github.com/gofiber/fiber/v3"
)

// PricingHandler handles pricing-related endpoints
type PricingHandler struct {
	x402 *middleware.X402Middleware
}

// PricingResponse represents the pricing information response
type PricingResponse struct {
	Currency string       `json:"currency"`
	Network  string       `json:"network"`
	Routes   []RoutePrice `json:"routes"`
}

// RoutePrice represents a single route's pricing
type RoutePrice struct {
	Path        string  `json:"path"`
	Method      string  `json:"method"`
	Price       float64 `json:"price_usd"`
	Description string  `json:"description"`
}

// NewPricingHandler creates a new pricing handler
func NewPricingHandler(x402 *middleware.X402Middleware) *PricingHandler {
	return &PricingHandler{
		x402: x402,
	}
}

// RegisterRoutes registers pricing routes
func (h *PricingHandler) RegisterRoutes(app *fiber.App) {
	app.Get("/v1/pricing", h.GetPricing)
}

// GetPricing returns pricing information for all endpoints
// @Summary Get pricing information
// @Description Returns the pricing for all protected endpoints
// @Tags pricing
// @Produce json
// @Success 200 {object} PricingResponse
// @Router /v1/pricing [get]
func (h *PricingHandler) GetPricing(c fiber.Ctx) error {
	routes := h.x402.GetRoutes()

	routePrices := make([]RoutePrice, 0, len(routes))
	for _, route := range routes {
		description := ""
		switch route.Path {
		case "/v1/scan/content":
			description = "Content scanning for prompt injection detection"
		case "/v1/scan/output":
			description = "Output scanning for credential leak detection"
		}

		routePrices = append(routePrices, RoutePrice{
			Path:        route.Path,
			Method:      route.Method,
			Price:       route.Price,
			Description: description,
		})
	}

	return c.JSON(PricingResponse{
		Currency: "USDC",
		Network:  h.x402.GetNetwork(),
		Routes:   routePrices,
	})
}
