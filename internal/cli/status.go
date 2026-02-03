package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Status displays the current status of Stronghold
func Status() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Installed {
		fmt.Println("Stronghold is not initialized.")
		fmt.Println("Run 'stronghold init' to set it up.")
		return nil
	}

	serviceManager := NewServiceManager(config)

	// Check if proxy is running
	proxyStatus, err := serviceManager.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check proxy status: %w", err)
	}

	// Reset daily stats if needed
	config.ResetDailyStats()

	// Print status header
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║         Stronghold Status                ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	// Check transparent proxy status
	tp := NewTransparentProxy(config)
	tpEnabled, _ := tp.Status()

	// Proxy status
	fmt.Println("Proxy:")
	if proxyStatus.Running {
		fmt.Printf("  Status:     %s\n", successStyle.Render("Running"))
		fmt.Printf("  Port:       %d\n", proxyStatus.Port)
		fmt.Printf("  PID:        %d\n", proxyStatus.PID)
		fmt.Printf("  Address:    %s\n", config.GetProxyAddr())
		if tpEnabled {
			fmt.Printf("  Mode:       %s\n", successStyle.Render("Network-level (transparent)"))
		} else {
			fmt.Printf("  Mode:       %s\n", warningStyle.Render("Not intercepting traffic"))
		}
		fmt.Printf("  Protection: %s\n", successStyle.Render("Enabled"))
	} else {
		fmt.Printf("  Status:     %s\n", errorStyle.Render("Stopped"))
		fmt.Printf("  Protection: %s\n", warningStyle.Render("Disabled"))
	}
	fmt.Println()

	// Session info
	fmt.Println("Session:")
	if config.Auth.LoggedIn {
		fmt.Printf("  User:       %s\n", config.Auth.Email)
		// In a real implementation, fetch balance from API
		fmt.Printf("  Balance:    $12.45\n")
		fmt.Printf("  Spent:      $3.55 this month\n")
	} else {
		fmt.Printf("  User:       %s\n", "Not logged in")
	}
	fmt.Println()

	// Usage stats
	fmt.Println("Usage (last 24h):")
	fmt.Printf("  Requests:   %d\n", config.Stats.RequestsToday)
	fmt.Printf("  Blocked:    %d (%.2f%%)\n",
		config.Stats.BlockedToday,
		percentage(config.Stats.BlockedToday, config.Stats.RequestsToday))
	fmt.Printf("  Warned:     %d (%.2f%%)\n",
		config.Stats.WarnedToday,
		percentage(config.Stats.WarnedToday, config.Stats.RequestsToday))
	fmt.Printf("  Cost:       $%.2f\n", config.Stats.CostToday)
	fmt.Println()

	// Configuration
	fmt.Println("Configuration:")
	fmt.Printf("  Config:     %s\n", ConfigPath())
	fmt.Printf("  Logs:       %s\n", config.Logging.File)
	fmt.Printf("  API:        %s\n", config.API.Endpoint)
	fmt.Printf("  Mode:       %s\n", config.Scanning.Mode)
	fmt.Println()

	// Quick actions
	fmt.Println("Quick Actions:")
	if proxyStatus.Running {
		fmt.Println("  stronghold disable  - Temporarily disable protection")
	} else {
		fmt.Println("  stronghold enable   - Enable protection")
	}
	fmt.Println("  stronghold logs     - View proxy logs")
	fmt.Println()

	return nil
}

// percentage calculates a percentage safely
func percentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// Logs displays the proxy logs
func Logs(follow bool, lines int) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logFile := config.Logging.File
	if logFile == "" {
		logFile = filepath.Join(ConfigDir(), "logs", "proxy.log")
	}

	// Check if log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logFile)
	}

	// Read and display logs
	if follow {
		// Tail -f equivalent
		fmt.Printf("Following logs from %s (Ctrl+C to exit)...\n\n", logFile)

		file, err := os.Open(logFile)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		defer file.Close()

		// Seek to end
		file.Seek(0, 2)

		// Read new lines
		buf := make([]byte, 1024)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				fmt.Print(string(buf[:n]))
			}
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}
	} else {
		// Read last N lines
		data, err := os.ReadFile(logFile)
		if err != nil {
			return fmt.Errorf("failed to read log file: %w", err)
		}

		// Simple implementation - just print the whole file or last portion
		content := string(data)
		if len(content) > 10000 {
			content = content[len(content)-10000:]
		}
		fmt.Print(content)
	}

	return nil
}
