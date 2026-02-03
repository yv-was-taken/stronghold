package cli

import (
	"fmt"
	"time"
)

// Enable enables the Stronghold proxy with transparent proxying
func Enable() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Installed {
		return fmt.Errorf("stronghold is not initialized. Run 'stronghold init' first")
	}

	// Check if transparent proxy is available
	tp := NewTransparentProxy(config)
	if !tp.IsAvailable() {
		return fmt.Errorf("transparent proxy not available on this system. Requires iptables/nftables (Linux) or pf (macOS)")
	}

	serviceManager := NewServiceManager(config)

	// Check current status
	status, err := serviceManager.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	if status.Running {
		fmt.Printf("Stronghold proxy is already running on port %d (PID: %d)\n", status.Port, status.PID)
		return nil
	}

	fmt.Println("Starting Stronghold proxy...")

	// Start the proxy
	if err := serviceManager.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	// Wait a moment for the proxy to start
	time.Sleep(500 * time.Millisecond)

	// Verify it's running
	status, err = serviceManager.IsRunning()
	if err != nil || !status.Running {
		return fmt.Errorf("proxy failed to start properly")
	}

	// Enable transparent proxying
	if err := tp.Enable(); err != nil {
		serviceManager.Stop()
		return fmt.Errorf("failed to enable transparent proxy: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Stronghold proxy started successfully")
	fmt.Printf("  Address: %s\n", config.GetProxyAddr())
	fmt.Printf("  PID:     %d\n", status.PID)
	fmt.Println()
	fmt.Println("All HTTP/HTTPS traffic is now being intercepted at the network level.")
	fmt.Println("This cannot be bypassed by applications.")

	return nil
}
