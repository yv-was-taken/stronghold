package cli

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ServiceManager handles system service operations
type ServiceManager struct {
	config *CLIConfig
}

// NewServiceManager creates a new service manager
func NewServiceManager(config *CLIConfig) *ServiceManager {
	return &ServiceManager{config: config}
}

// ServiceStatus represents the status of the proxy service
type ServiceStatus struct {
	Running bool
	PID     int
	Port    int
	Error   error
}

// IsRunning checks if the proxy is running
func (s *ServiceManager) IsRunning() (*ServiceStatus, error) {
	// Check if process is listening on the configured port
	addr := net.JoinHostPort(s.config.Proxy.Bind, fmt.Sprintf("%d", s.config.Proxy.Port))

	// Try to connect
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return &ServiceStatus{Running: false}, nil
	}
	conn.Close()

	// Find the PID
	pid := s.findPIDByPort(s.config.Proxy.Port)

	return &ServiceStatus{
		Running: true,
		PID:     pid,
		Port:    s.config.Proxy.Port,
	}, nil
}

// findPIDByPort finds the process ID listening on a given port
func (s *ServiceManager) findPIDByPort(port int) int {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("sh", "-c", fmt.Sprintf("ss -tlnp | grep ':%d' | grep -oP 'pid=\\K[0-9]+' | head -1", port))
	case "darwin":
		cmd = exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d", port))
	default:
		return 0
	}

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	pid, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return pid
}

// Start starts the proxy service
func (s *ServiceManager) Start() error {
	// Check if already running
	status, _ := s.IsRunning()
	if status.Running {
		return fmt.Errorf("proxy is already running on port %d (PID: %d)", status.Port, status.PID)
	}

	// Build the proxy binary path
	proxyBinary := s.getProxyBinaryPath()

	// Check if proxy binary exists
	if _, err := os.Stat(proxyBinary); os.IsNotExist(err) {
		return fmt.Errorf("proxy binary not found at %s", proxyBinary)
	}

	// Start the proxy
	cmd := exec.Command(proxyBinary)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("STRONGHOLD_CONFIG=%s", ConfigPath()))
	cmd.Env = append(cmd.Env, fmt.Sprintf("STRONGHOLD_PROXY_PORT=%d", s.config.Proxy.Port))
	cmd.Env = append(cmd.Env, fmt.Sprintf("STRONGHOLD_PROXY_BIND=%s", s.config.Proxy.Bind))
	cmd.Env = append(cmd.Env, fmt.Sprintf("STRONGHOLD_API_ENDPOINT=%s", s.config.API.Endpoint))

	// Set up logging
	logFile := s.config.Logging.File
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	// Wait a moment and verify it's running
	time.Sleep(500 * time.Millisecond)
	status, _ = s.IsRunning()
	if !status.Running {
		return fmt.Errorf("proxy failed to start")
	}

	return nil
}

// Stop stops the proxy service
func (s *ServiceManager) Stop() error {
	status, err := s.IsRunning()
	if err != nil {
		return err
	}

	if !status.Running {
		return nil // Already stopped
	}

	// Kill the process
	if status.PID > 0 {
		process, err := os.FindProcess(status.PID)
		if err == nil {
			if err := process.Kill(); err != nil {
				return fmt.Errorf("failed to stop proxy (PID: %d): %w", status.PID, err)
			}
		}
	}

	// Wait for it to stop
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		status, _ = s.IsRunning()
		if !status.Running {
			return nil
		}
	}

	return fmt.Errorf("proxy did not stop in time")
}

// Restart restarts the proxy service
func (s *ServiceManager) Restart() error {
	if err := s.Stop(); err != nil {
		return err
	}
	return s.Start()
}

// InstallService installs the proxy as a system service
func (s *ServiceManager) InstallService() error {
	switch runtime.GOOS {
	case "linux":
		return s.installLinuxService()
	case "darwin":
		return s.installDarwinService()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// UninstallService removes the proxy system service
func (s *ServiceManager) UninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return s.uninstallLinuxService()
	case "darwin":
		return s.uninstallDarwinService()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// installLinuxService installs systemd service on Linux
func (s *ServiceManager) installLinuxService() error {
	proxyBinary := s.getProxyBinaryPath()
	username := StrongholdUsername()

	// Check if we can use system systemd or user systemd
	serviceDir := "/etc/systemd/system"
	useSystemd := false
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		// Check if we have permission
		if _, err := os.Stat(serviceDir); err == nil {
			useSystemd = true
		}
	}

	if useSystemd {
		// System service runs as stronghold user for transparent proxy filtering
		serviceContent := fmt.Sprintf(`[Unit]
Description=Stronghold Proxy Service
After=network.target

[Service]
Type=simple
User=%s
Group=%s
EnvironmentFile=-/etc/systemd/system/stronghold-proxy.env
ExecStart=%s
Restart=always
RestartSec=5
# Allow binding to privileged ports if needed
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`, username, username, proxyBinary)

		servicePath := filepath.Join(serviceDir, "stronghold-proxy.service")
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return fmt.Errorf("failed to write service file: %w", err)
		}

		// Reload systemd
		exec.Command("systemctl", "daemon-reload").Run()
	} else {
		// User-mode systemd - still runs as current user but firewall rules handle filtering
		userServiceDir := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
		os.MkdirAll(userServiceDir, 0755)

		serviceContent := fmt.Sprintf(`[Unit]
Description=Stronghold Proxy Service
After=network.target

[Service]
Type=simple
EnvironmentFile=-%%h/.config/systemd/user/stronghold-proxy.env
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, proxyBinary)

		servicePath := filepath.Join(userServiceDir, "stronghold-proxy.service")
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
			return fmt.Errorf("failed to write user service file: %w", err)
		}

		// Reload user systemd
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	return nil
}

// uninstallLinuxService removes systemd service on Linux
func (s *ServiceManager) uninstallLinuxService() error {
	// Try system service first
	servicePath := "/etc/systemd/system/stronghold-proxy.service"
	if _, err := os.Stat(servicePath); err == nil {
		exec.Command("systemctl", "stop", "stronghold-proxy").Run()
		exec.Command("systemctl", "disable", "stronghold-proxy").Run()
		os.Remove(servicePath)
		exec.Command("systemctl", "daemon-reload").Run()
		return nil
	}

	// Try user service
	userServicePath := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "stronghold-proxy.service")
	if _, err := os.Stat(userServicePath); err == nil {
		exec.Command("systemctl", "--user", "stop", "stronghold-proxy").Run()
		exec.Command("systemctl", "--user", "disable", "stronghold-proxy").Run()
		os.Remove(userServicePath)
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	return nil
}

// installDarwinService installs launchd service on macOS
func (s *ServiceManager) installDarwinService() error {
	proxyBinary := s.getProxyBinaryPath()
	configDir := ConfigDir()
	username := StrongholdUsername() // "_stronghold"

	// System-level daemon that runs as _stronghold user
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.stronghold.proxy</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>UserName</key>
    <string>%s</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>STRONGHOLD_CONFIG</key>
        <string>%s</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/stronghold-proxy.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/stronghold-proxy.log</string>
</dict>
</plist>
`, proxyBinary, username, ConfigPath())

	// Install as system daemon in /Library/LaunchDaemons (requires root)
	launchDaemonsDir := "/Library/LaunchDaemons"
	plistPath := filepath.Join(launchDaemonsDir, "com.stronghold.proxy.plist")

	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		// Fall back to user LaunchAgent if we can't write to system location
		launchAgentsDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
		os.MkdirAll(launchAgentsDir, 0755)

		// User agent doesn't have UserName key
		userPlistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.stronghold.proxy</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>STRONGHOLD_CONFIG</key>
        <string>%s</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/logs/proxy.log</string>
    <key>StandardErrorPath</key>
    <string>%s/logs/proxy.log</string>
</dict>
</plist>
`, proxyBinary, ConfigPath(), configDir, configDir)

		plistPath = filepath.Join(launchAgentsDir, "com.stronghold.proxy.plist")
		if err := os.WriteFile(plistPath, []byte(userPlistContent), 0644); err != nil {
			return fmt.Errorf("failed to write plist file: %w", err)
		}
	}

	return nil
}

// uninstallDarwinService removes launchd service on macOS
func (s *ServiceManager) uninstallDarwinService() error {
	// Try system daemon first
	systemPlistPath := "/Library/LaunchDaemons/com.stronghold.proxy.plist"
	if _, err := os.Stat(systemPlistPath); err == nil {
		exec.Command("launchctl", "unload", systemPlistPath).Run()
		os.Remove(systemPlistPath)
	}

	// Also try user agent
	userPlistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.stronghold.proxy.plist")
	if _, err := os.Stat(userPlistPath); err == nil {
		exec.Command("launchctl", "unload", userPlistPath).Run()
		os.Remove(userPlistPath)
	}

	return nil
}

// getProxyBinaryPath returns the path to the proxy binary
func (s *ServiceManager) getProxyBinaryPath() string {
	// Check if running from source (development) AND binary exists
	if _, err := os.Stat("./cmd/proxy"); err == nil {
		if _, err := os.Stat("./stronghold-proxy"); err == nil {
			return "./stronghold-proxy"
		}
	}

	// Check standard locations
	locations := []string{
		"/usr/local/bin/stronghold-proxy",
		"/usr/bin/stronghold-proxy",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "stronghold-proxy"),
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	// Default to /usr/local/bin
	return "/usr/local/bin/stronghold-proxy"
}

// IsPortAvailable checks if a port is available
func IsPortAvailable(addr string) bool {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// FindAvailablePort finds an available port starting from the given port
func FindAvailablePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		if IsPortAvailable(addr) {
			return port
		}
	}
	return 0
}
