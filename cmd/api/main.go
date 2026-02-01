// @title Stronghold API
// @version 1.0
// @description Pay-per-request AI security scanning API with 4-layer threat detection.
// @description
// @description ## Authentication
// @description Most endpoints require JWT authentication via httpOnly cookies.
// @description Create an account at POST /v1/auth/account to get started.
// @description
// @description ## Payments
// @description Scan endpoints require x402 payment via the X-PAYMENT header.
// @description See GET /v1/pricing for current endpoint prices.

// @contact.name Stronghold Support
// @contact.url https://github.com/yv-was-taken/stronghold

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /
// @schemes http https

// @securityDefinitions.apikey CookieAuth
// @in cookie
// @name stronghold_access

// @tag.name health
// @tag.description Health check endpoints for monitoring
// @tag.name pricing
// @tag.description Endpoint pricing information
// @tag.name auth
// @tag.description Account creation and authentication
// @tag.name account
// @tag.description Account management and billing
// @tag.name scan
// @tag.description AI security scanning endpoints (payment required)

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/server"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Setup structured logging - JSON for production, text for development
	setupLogging(cfg)

	// Validate configuration - fails in production if critical values are missing
	if err := cfg.Validate(); err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	// Create server
	srv, err := server.New(cfg)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	// Create a context that will be cancelled on shutdown signal
	ctx, cancel := context.WithCancel(context.Background())

	// Start server in a goroutine (includes settlement worker)
	go func() {
		if err := srv.Start(ctx); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")

	// Cancel context to signal workers to stop
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server exited")
}

// setupLogging configures the global slog logger
func setupLogging(cfg *config.Config) {
	var handler slog.Handler

	if cfg.IsProduction() {
		// JSON output for production - easy to parse by log aggregators
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		// Text output for development - human readable
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	slog.SetDefault(slog.New(handler))
}
