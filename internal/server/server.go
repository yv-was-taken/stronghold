package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"citadel-api/internal/citadel"
	"citadel-api/internal/config"
	"citadel-api/internal/handlers"
	"citadel-api/internal/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

// Server represents the HTTP server
type Server struct {
	app     *fiber.App
	config  *config.Config
	scanner *citadel.Scanner
}

// New creates a new server instance
func New(cfg *config.Config) (*Server, error) {
	// Initialize scanner
	scanner, err := citadel.NewScanner(&cfg.Citadel)
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %w", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Citadel API",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: errorHandler,
	})

	s := &Server{
		app:     app,
		config:  cfg,
		scanner: scanner,
	}

	// Setup middleware
	s.setupMiddleware()

	// Setup routes
	s.setupRoutes()

	return s, nil
}

// setupMiddleware configures all middleware
func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.app.Use(recover.New())

	// Logger middleware
	s.app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} ${latency}\n",
	}))

	// CORS middleware - configured for x402 headers
	s.app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-PAYMENT", "X-PAYMENT-RESPONSE", "Authorization"},
		ExposeHeaders:    []string{"X-PAYMENT-RESPONSE"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// x402 payment middleware (applied to all routes)
	x402 := middleware.NewX402Middleware(&s.config.X402, &s.config.Pricing)
	s.app.Use(x402.Middleware())
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Initialize x402 middleware for handlers
	x402 := middleware.NewX402Middleware(&s.config.X402, &s.config.Pricing)

	// Health handler (no payment required)
	healthHandler := handlers.NewHealthHandler()
	healthHandler.RegisterRoutes(s.app)

	// Pricing handler (no payment required)
	pricingHandler := handlers.NewPricingHandler(x402)
	pricingHandler.RegisterRoutes(s.app)

	// Scan handlers (payment required)
	scanHandler := handlers.NewScanHandler(s.scanner, x402)
	scanHandler.RegisterRoutes(s.app)

	// 404 handler
	s.app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "Not found",
			"message": "The requested endpoint does not exist",
			"path":    c.Path(),
		})
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%s", s.config.Server.Port)
	log.Printf("Starting Citadel API server on %s", addr)
	return s.app.Listen(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")

	// Close scanner
	if err := s.scanner.Close(); err != nil {
		log.Printf("Error closing scanner: %v", err)
	}

	// Shutdown Fiber
	return s.app.ShutdownWithContext(ctx)
}

// errorHandler handles errors globally
func errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	// Log the error
	log.Printf("Error: %v", err)

	// Return JSON response
	return c.Status(code).JSON(fiber.Map{
		"error":     message,
		"status":    code,
		"timestamp": time.Now().Unix(),
		"request_id": c.Locals("request_id"),
	})
}
