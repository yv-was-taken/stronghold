package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/handlers"
	"stronghold/internal/kms"
	"stronghold/internal/middleware"
	"stronghold/internal/settlement"
	"stronghold/internal/stronghold"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

// Server represents the HTTP server
type Server struct {
	app              *fiber.App
	config           *config.Config
	scanner          *stronghold.Scanner
	database         *db.DB
	authHandler      *handlers.AuthHandler
	settlementWorker *settlement.Worker
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
		MaxConns: cfg.Database.MaxConns,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Run database migrations
	if err := database.Migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	// Initialize KMS client for wallet key encryption (optional in dev, required in prod)
	var kmsClient *kms.Client
	if cfg.KMS.Region != "" && cfg.KMS.KeyID != "" {
		kmsClient, err = kms.New(context.Background(), &kms.Config{
			Region: cfg.KMS.Region,
			KeyID:  cfg.KMS.KeyID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize KMS client: %w", err)
		}
		slog.Info("KMS client initialized", "region", cfg.KMS.Region, "key_id", cfg.KMS.KeyID)
	} else {
		slog.Warn("KMS not configured - wallet keys will not be stored server-side")
	}

	// Initialize auth handler
	authConfig := &handlers.AuthConfig{
		JWTSecret:       cfg.Auth.JWTSecret,
		AccessTokenTTL:  cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL: cfg.Auth.RefreshTokenTTL,
		DashboardURL:    cfg.Dashboard.URL,
		AllowedOrigins:  cfg.Dashboard.AllowedOrigins,
		Cookie: handlers.CookieConfig{
			Domain:   cfg.Cookie.Domain,
			Secure:   cfg.Cookie.Secure,
			SameSite: cfg.Cookie.SameSite,
		},
	}
	authHandler := handlers.NewAuthHandler(database, authConfig, kmsClient)

	// Create Fiber app
	fiberConfig := fiber.Config{
		AppName:      "Stronghold API",
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		ErrorHandler: errorHandler,
	}

	// Configure proxy header for correct client IP behind reverse proxy
	if cfg.Server.ProxyHeader != "" {
		fiberConfig.ProxyHeader = cfg.Server.ProxyHeader
	}
	if len(cfg.Server.TrustedProxies) > 0 {
		fiberConfig.TrustProxy = true
		fiberConfig.TrustProxyConfig = fiber.TrustProxyConfig{
			Proxies: cfg.Server.TrustedProxies,
		}
	}

	app := fiber.New(fiberConfig)

	// Create settlement worker for background retry of failed settlements
	settlementWorker := settlement.NewWorker(database, &cfg.X402, nil)

	s := &Server{
		app:              app,
		config:           cfg,
		scanner:          scanner,
		database:         database,
		authHandler:      authHandler,
		settlementWorker: settlementWorker,
	}

	// Setup middleware
	s.setupMiddleware()

	// Setup routes (now uses atomic payment middleware)
	s.setupRoutes()

	return s, nil
}

// setupMiddleware configures all middleware
func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.app.Use(recover.New())

	// Request ID middleware - must be early to ensure ID is available for logging
	s.app.Use(middleware.RequestID())

	// Security headers middleware - sets CSP, X-Frame-Options, etc.
	s.app.Use(middleware.SecurityHeaders())

	// Logger middleware - includes request ID
	// Use JSON format in production for log aggregators, text format for development
	if s.config.IsProduction() {
		s.app.Use(logger.New(logger.Config{
			Format: `{"time":"${time}","status":${status},"method":"${method}","path":"${path}","latency":"${latency}","ip":"${ip}","request_id":"${locals:request_id}"}` + "\n",
		}))
	} else {
		s.app.Use(logger.New(logger.Config{
			Format: "[${time}] ${status} - ${method} ${path} ${latency} [${locals:request_id}]\n",
		}))
	}

	// Rate limiting middleware (general limits)
	rateLimiter := middleware.NewRateLimitMiddleware(&s.config.RateLimit)
	s.app.Use(rateLimiter.Middleware())

	// CORS middleware - configured for dashboard, x402 headers, and request tracking
	s.app.Use(cors.New(cors.Config{
		AllowOrigins:     s.config.Dashboard.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-PAYMENT", "X-PAYMENT-RESPONSE", "Authorization", "X-Stronghold-Device", middleware.RequestIDHeader},
		ExposeHeaders:    []string{"X-PAYMENT-RESPONSE", middleware.RequestIDHeader},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Note: x402 payment middleware is now applied per-route via AtomicPayment
	// for atomic settlement. This removes the global Middleware() and SettleAfterHandler()
	// which had the race condition where settlement could fail after service delivery.
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Initialize x402 middleware with database for atomic payments
	x402 := middleware.NewX402MiddlewareWithDB(&s.config.X402, &s.config.Pricing, s.database)

	if !s.config.X402.HasPayments() {
		slog.Warn("x402 payments DISABLED - no wallet addresses configured",
			"environment", s.config.Environment,
		)
	}

	// Initialize rate limiter for auth routes
	rateLimiter := middleware.NewRateLimitMiddleware(&s.config.RateLimit)

	// Health handler (no payment required)
	healthHandler := handlers.NewHealthHandler(s.database, s.config)
	healthHandler.RegisterRoutes(s.app)

	// Pricing handler (no payment required)
	pricingHandler := handlers.NewPricingHandler(x402)
	pricingHandler.RegisterRoutes(s.app)

	// Auth handlers with stricter rate limiting
	s.authHandler.RegisterRoutesWithMiddleware(s.app, rateLimiter.AuthLimiter())

	// Account handlers (no payment required for account management)
	// Reuse authConfig from authHandler initialization
	accountHandler := handlers.NewAccountHandler(s.database, s.authHandler.Config(), &s.config.Stripe)
	accountHandler.RegisterRoutes(s.app, s.authHandler)

	// Stripe webhook handler (no auth required - verified via signature)
	stripeWebhookHandler := handlers.NewStripeWebhookHandler(s.database, &s.config.Stripe)
	s.app.Post("/webhooks/stripe", stripeWebhookHandler.HandleWebhook)

	// Scan handlers (payment required - now uses AtomicPayment for atomic settlement)
	scanHandler := handlers.NewScanHandlerWithDB(s.scanner, x402, s.database, &s.config.Pricing)
	scanHandler.RegisterRoutes(s.app)

	// API documentation
	docsHandler := handlers.NewDocsHandler()
	docsHandler.RegisterRoutes(s.app)

	// 404 handler
	s.app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":      "Not found",
			"message":    "The requested endpoint does not exist",
			"path":       c.Path(),
			"request_id": middleware.GetRequestID(c),
		})
	})
}

// Start starts the HTTP server and background workers
func (s *Server) Start(ctx context.Context) error {
	// Start settlement worker for background retry of failed settlements
	if s.settlementWorker != nil {
		s.settlementWorker.Start(ctx)
	}

	addr := fmt.Sprintf(":%s", s.config.Server.Port)
	slog.Info("starting Stronghold API server", "addr", addr)
	return s.app.Listen(addr)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down server")

	// Stop settlement worker first to prevent new retries
	if s.settlementWorker != nil {
		s.settlementWorker.Stop()
	}

	// Close database connection
	if s.database != nil {
		s.database.Close()
	}

	// Close scanner
	if err := s.scanner.Close(); err != nil {
		slog.Error("error closing scanner", "error", err)
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

	requestID := middleware.GetRequestID(c)

	// Log the error with request ID
	slog.Error("request error", "error", err, "request_id", requestID, "status", code)

	// Return JSON response
	return c.Status(code).JSON(fiber.Map{
		"error":      message,
		"status":     code,
		"timestamp":  time.Now().Unix(),
		"request_id": requestID,
	})
}
