package main

import (
	"context"
	"log"
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

	// Create server
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Create a context that will be cancelled on shutdown signal
	ctx, cancel := context.WithCancel(context.Background())

	// Start server in a goroutine (includes settlement worker)
	go func() {
		if err := srv.Start(ctx); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Cancel context to signal workers to stop
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
