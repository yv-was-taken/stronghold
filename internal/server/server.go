package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/handlers"
	"stronghold/internal/middleware"
	"stronghold/internal/stronghold"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

// Server represents the HTTP server
type Server struct {
	app       *fiber.App
	config    *config.Config
	scanner   *stronghold.Scanner
	database  *db.DB
	authHandler *handlers.AuthHandler
}

// New creates a new server instance
func New(cfg *config.Config) (*Server, error) {
	// Initialize scanner
	scanner, err := stronghold.NewScanner(&cfg.Stronghold)
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %w", err)
	}

	// Initialize database
	database, err := db.New(&db.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		Name:     cfg.Database.Name,
		SSLMode:  cfg.Database.SSLMode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Initialize auth handler
	authConfig := &handlers.AuthConfig{
		JWTSecret:       cfg.Auth.JWTSecret,
		AccessTokenTTL:  cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL: cfg.Auth.RefreshTokenTTL,
		DashboardURL:    cfg.Dashboard.URL,
		AllowedOrigins:  cfg.Dashboard.AllowedOrigins,
	}
	authHandler := handlers.NewAuthHandler(database, authConfig)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Stronghold API",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: errorHandler,
	})

	s := &Server{
		app:         app,
		config:      cfg,
		scanner:     scanner,
		database:    database,
		authHandler: authHandler,
	}

	// Setup middleware
	s.setupMiddleware()

	// Setup routes
	s.setupRoutes()

	// Setup post-route middleware (for payment settlement)
	s.setupPostMiddleware()

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

	// CORS middleware - configured for dashboard and x402 headers
	s.app.Use(cors.New(cors.Config{
		AllowOrigins:     s.config.Dashboard.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-PAYMENT", "X-PAYMENT-RESPONSE", "Authorization"},
		ExposeHeaders:    []string{"X-PAYMENT-RESPONSE"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// x402 payment middleware (applied to all routes except auth)
	x402 := middleware.NewX402Middleware(&s.config.X402, &s.config.Pricing)
	s.app.Use(x402.Middleware())
}

// setupPostMiddleware configures middleware that runs after routes
func (s *Server) setupPostMiddleware() {
	// x402 settlement middleware (settles payments after successful responses)
	x402 := middleware.NewX402Middleware(&s.config.X402, &s.config.Pricing)
	s.app.Use(x402.SettleAfterHandler())
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Initialize x402 middleware for handlers
	x402 := middleware.NewX402Middleware(&s.config.X402, &s.config.Pricing)

	// Health handler (no payment required)
	healthHandler := handlers.NewHealthHandler(s.database, s.config)
	healthHandler.RegisterRoutes(s.app)

	// Pricing handler (no payment required)
	pricingHandler := handlers.NewPricingHandler(x402)
	pricingHandler.RegisterRoutes(s.app)

	// Auth handlers (no payment required)
	s.authHandler.RegisterRoutes(s.app)

	// Account handlers (no payment required for account management)
	accountHandler := handlers.NewAccountHandler(s.database, &handlers.AuthConfig{
		JWTSecret:       s.config.Auth.JWTSecret,
		AccessTokenTTL:  s.config.Auth.AccessTokenTTL,
		RefreshTokenTTL: s.config.Auth.RefreshTokenTTL,
		DashboardURL:    s.config.Dashboard.URL,
		AllowedOrigins:  s.config.Dashboard.AllowedOrigins,
	})
	accountHandler.RegisterRoutes(s.app, s.authHandler)

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
	log.Printf("Starting Stronghold API server on %s", addr)
	return s.app.Listen(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")

	// Close database connection
	if s.database != nil {
		s.database.Close()
	}

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
