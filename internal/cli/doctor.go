package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// CheckResult represents the result of a single check
type CheckResult struct {
	Name     string
	Status   CheckStatus
	Message  string
	Fix      string // Optional: how to fix if failed
}

// CheckStatus represents the status of a check
type CheckStatus int

const (
	CheckPass CheckStatus = iota
	CheckWarn
	CheckFail
)

// String returns a string representation of the check status
func (s CheckStatus) String() string {
	switch s {
	case CheckPass:
		return "PASS"
	case CheckWarn:
		return "WARN"
	case CheckFail:
		return "FAIL"
	default:
		return "UNKNOWN"
	}
}

// Doctor runs all prerequisite checks
func Doctor() error {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║       Stronghold Doctor                  ║")
	fmt.Println("║   System Prerequisites Check             ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	results := []CheckResult{}

	// Run all checks
	results = append(results, checkPlatform())
	results = append(results, checkRoot())
	results = append(results, checkFirewallTools())
	results = append(results, checkPortAvailable())
	results = append(results, checkConfig())
	results = append(results, checkProxyBinary())
	results = append(results, checkCLIBinary())

	if runtime.GOOS == "linux" {
		results = append(results, checkKernelModules())
		results = append(results, checkNFTables())
	}

	// Print results
	passCount := 0
	warnCount := 0
	failCount := 0

	for _, result := range results {
		switch result.Status {
		case CheckPass:
			fmt.Printf("%s %s\n", successStyle.Render("✓"), result.Name)
			passCount++
		case CheckWarn:
			fmt.Printf("%s %s: %s\n", warningStyle.Render("⚠"), result.Name, result.Message)
			warnCount++
		case CheckFail:
			fmt.Printf("%s %s: %s\n", errorStyle.Render("✗"), result.Name, result.Message)
			if result.Fix != "" {
				fmt.Printf("  → %s\n", infoStyle.Render(result.Fix))
			}
			failCount++
		}
	}

	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  %s %d checks passed\n", successStyle.Render("✓"), passCount)
	if warnCount > 0 {
		fmt.Printf("  %s %d warnings\n", warningStyle.Render("⚠"), warnCount)
	}
	if failCount > 0 {
		fmt.Printf("  %s %d checks failed\n", errorStyle.Render("✗"), failCount)
	}

	fmt.Println()
	if failCount > 0 {
		fmt.Println(errorStyle.Render("System is NOT ready for Stronghold."))
		fmt.Println("Please fix the failed checks above and run 'stronghold doctor' again.")
	} else if warnCount > 0 {
		fmt.Println(warningStyle.Render("System is ready but has warnings."))
		fmt.Println("You can proceed with initialization, but review the warnings above.")
	} else {
		fmt.Println(successStyle.Render("System is ready for Stronghold!"))
		fmt.Println("Run 'stronghold init' to get started.")
	}

	return nil
}

// checkPlatform verifies the OS is supported
func checkPlatform() CheckResult {
	result := CheckResult{Name: "Operating System"}

	switch runtime.GOOS {
	case "linux":
		result.Status = CheckPass
		result.Message = fmt.Sprintf("Linux (%s)", runtime.GOARCH)
	case "darwin":
		result.Status = CheckPass
		result.Message = fmt.Sprintf("macOS (%s)", runtime.GOARCH)
	default:
		result.Status = CheckFail
		result.Message = fmt.Sprintf("%s is not supported", runtime.GOOS)
		result.Fix = "Stronghold requires Linux or macOS"
	}

	return result
}

// checkRoot verifies we have root/admin privileges when needed
func checkRoot() CheckResult {
	result := CheckResult{Name: "Root/Admin Privileges"}

	if os.Getuid() == 0 {
		result.Status = CheckPass
		result.Message = "Running as root (required for install/enable)"
	} else {
		result.Status = CheckWarn
		result.Message = "Not running as root"
		result.Fix = "Use 'sudo stronghold init' and 'sudo stronghold enable'. Other commands work without root."
	}

	return result
}

// checkFirewallTools checks for iptables/nftables/pf
func checkFirewallTools() CheckResult {
	result := CheckResult{Name: "Firewall Tools"}

	tp := &TransparentProxy{config: DefaultConfig()}

	if tp.IsAvailable() {
		result.Status = CheckPass
		if runtime.GOOS == "linux" {
			if tp.hasNftables() {
				result.Message = "nftables available"
			} else if tp.hasIptables() {
				result.Message = "iptables available"
			}
		} else {
			result.Message = "pf (packet filter) available"
		}
	} else {
		result.Status = CheckFail
		result.Message = "No firewall tools found"

		if runtime.GOOS == "linux" {
			result.Fix = "Install iptables or nftables: sudo apt-get install iptables (Debian/Ubuntu) or sudo yum install iptables (RHEL/CentOS)"
		} else {
			result.Fix = "pf should be built into macOS - this is unexpected"
		}
	}

	return result
}

// checkPortAvailable checks if the proxy port is available
func checkPortAvailable() CheckResult {
	result := CheckResult{Name: "Proxy Port Available"}

	config := DefaultConfig()
	addr := config.GetProxyAddr()

	if IsPortAvailable(addr) {
		result.Status = CheckPass
		result.Message = fmt.Sprintf("Port %d is available", config.Proxy.Port)
	} else {
		result.Status = CheckWarn
		result.Message = fmt.Sprintf("Port %d is in use", config.Proxy.Port)
		result.Fix = fmt.Sprintf("Run 'stronghold init' to use an alternative port, or stop the process using port %d", config.Proxy.Port)
	}

	return result
}

// checkConfig checks if config can be loaded/created
func checkConfig() CheckResult {
	result := CheckResult{Name: "Configuration"}

	configDir := ConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		result.Status = CheckFail
		result.Message = fmt.Sprintf("Cannot create config directory: %v", err)
		result.Fix = fmt.Sprintf("Check permissions on %s", filepath.Dir(configDir))
		return result
	}

	// Try to load or create config
	config, err := LoadConfig()
	if err != nil {
		result.Status = CheckFail
		result.Message = fmt.Sprintf("Cannot load config: %v", err)
		result.Fix = "Check permissions on ~/.stronghold/"
		return result
	}

	// Try to save
	if err := config.Save(); err != nil {
		result.Status = CheckFail
		result.Message = fmt.Sprintf("Cannot write config: %v", err)
		result.Fix = "Check write permissions on ~/.stronghold/"
		return result
	}

	result.Status = CheckPass
	if config.Installed {
		result.Message = "Configuration exists (installed)"
	} else {
		result.Message = "Configuration directory ready"
	}

	return result
}

// checkProxyBinary checks if the proxy binary exists
func checkProxyBinary() CheckResult {
	result := CheckResult{Name: "Proxy Binary"}

	locations := []string{
		"/usr/local/bin/stronghold-proxy",
		"/usr/bin/stronghold-proxy",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "stronghold-proxy"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			result.Status = CheckPass
			result.Message = fmt.Sprintf("Found at %s", loc)
			return result
		}
	}

	// Check if we can build from source
	if _, err := os.Stat("./cmd/proxy/main.go"); err == nil {
		result.Status = CheckWarn
		result.Message = "Not installed, but source available"
		result.Fix = "Run 'go build ./cmd/proxy' to build, or run 'stronghold install'"
		return result
	}

	result.Status = CheckFail
	result.Message = "stronghold-proxy binary not found"
	result.Fix = "Run 'stronghold init' to install"

	return result
}

// checkCLIBinary checks if the CLI binary exists (less critical)
func checkCLIBinary() CheckResult {
	result := CheckResult{Name: "CLI Binary"}

	// If we're running, CLI obviously exists
	result.Status = CheckPass
	result.Message = "Running from current binary"

	return result
}

// checkKernelModules checks for required kernel modules on Linux
func checkKernelModules() CheckResult {
	result := CheckResult{Name: "Kernel Modules"}

	// Check if iptables modules are available
	modulesFile := "/proc/net/ip_tables_names"
	if _, err := os.Stat(modulesFile); err == nil {
		data, _ := os.ReadFile(modulesFile)
		if len(data) > 0 {
			result.Status = CheckPass
			result.Message = "IP tables modules loaded"
			return result
		}
	}

	// Check for nf_tables
	if _, err := os.Stat("/proc/net/nf_tables"); err == nil {
		result.Status = CheckPass
		result.Message = "NF tables available"
		return result
	}

	result.Status = CheckWarn
	result.Message = "Kernel networking modules not detected"
	result.Fix = "Modules may load on demand - try running 'sudo modprobe ip_tables'"

	return result
}

// checkNFTables checks specifically for nftables availability
func checkNFTables() CheckResult {
	result := CheckResult{Name: "NFTables Backend"}

	cmd := exec.Command("nft", "list", "tables")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// nftables might not be the backend, that's ok
		result.Status = CheckPass
		result.Message = "Using iptables backend"
		return result
	}

	result.Status = CheckPass
	if len(output) > 0 {
		result.Message = "nftables backend available with existing tables"
	} else {
		result.Message = "nftables backend available"
	}

	return result
}
