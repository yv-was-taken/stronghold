package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigVersion is the current config file version
const ConfigVersion = "1.0"

// ProxyConfig holds proxy-specific configuration
type ProxyConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

// APIConfig holds Stronghold API configuration
type APIConfig struct {
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Token         string `yaml:"token"`
	Email         string `yaml:"email"`
	UserID        string `yaml:"user_id"`
	AccountNumber string `yaml:"account_number"`
	LoggedIn      bool   `yaml:"logged_in"`
}

// WalletConfig holds wallet configuration
type WalletConfig struct {
	Address string `yaml:"address"`
	Network string `yaml:"network"`
}

// PaymentsConfig holds payment configuration
type PaymentsConfig struct {
	Method         string  `yaml:"method"`
	AutoTopup      bool    `yaml:"auto_topup"`
	TopupThreshold float64 `yaml:"topup_threshold"`
	TopupAmount    float64 `yaml:"topup_amount"`
	WalletAddress  string  `yaml:"wallet_address,omitempty"`
}

// ScanningConfig holds scanning behavior configuration
type ScanningConfig struct {
	Mode           string  `yaml:"mode"`
	BlockThreshold float64 `yaml:"block_threshold"`
	FailOpen       bool    `yaml:"fail_open"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// UsageStats holds usage statistics
type UsageStats struct {
	RequestsToday int64   `yaml:"requests_today"`
	BlockedToday  int64   `yaml:"blocked_today"`
	WarnedToday   int64   `yaml:"warned_today"`
	CostToday     float64 `yaml:"cost_today"`
	LastReset     string  `yaml:"last_reset"`
}

// CLIConfig holds the complete CLI configuration
type CLIConfig struct {
	Version     string         `yaml:"version"`
	Proxy       ProxyConfig    `yaml:"proxy"`
	API         APIConfig      `yaml:"api"`
	Auth        AuthConfig     `yaml:"auth"`
	Wallet      WalletConfig   `yaml:"wallet"`
	Payments    PaymentsConfig `yaml:"payments"`
	Scanning    ScanningConfig `yaml:"scanning"`
	Logging     LoggingConfig  `yaml:"logging"`
	Stats       UsageStats     `yaml:"stats"`
	Installed   bool           `yaml:"installed"`
	InstallDate string         `yaml:"install_date,omitempty"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *CLIConfig {
	homeDir, _ := os.UserHomeDir()
	return &CLIConfig{
		Version: ConfigVersion,
		Proxy: ProxyConfig{
			Port: 8080,
			Bind: "127.0.0.1",
		},
		API: APIConfig{
			Endpoint: "https://api.stronghold.security",
			Timeout:  30 * time.Second,
		},
		Auth: AuthConfig{
			LoggedIn: false,
		},
		Wallet: WalletConfig{
			Network: "base",
		},
		Payments: PaymentsConfig{
			Method:         "stripe",
			AutoTopup:      true,
			TopupThreshold: 5.00,
			TopupAmount:    20.00,
		},
		Scanning: ScanningConfig{
			Mode:           "smart",
			BlockThreshold: 0.55,
			FailOpen:       true,
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  filepath.Join(homeDir, ".stronghold", "logs", "proxy.log"),
		},
		Stats: UsageStats{
			LastReset: time.Now().Format(time.RFC3339),
		},
		Installed: false,
	}
}

// ConfigDir returns the configuration directory
func ConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".stronghold")
}

// ConfigPath returns the full path to the config file
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// LoadConfig loads the configuration from disk
func LoadConfig() (*CLIConfig, error) {
	configPath := ConfigPath()

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config CLIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Migrate if needed
	if config.Version == "" {
		config.Version = ConfigVersion
	}

	return &config, nil
}

// Save saves the configuration to disk
func (c *CLIConfig) Save() error {
	configDir := ConfigDir()

	// Create directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create logs directory
	logsDir := filepath.Join(configDir, "logs")
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := ConfigPath()
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetProxyAddr returns the proxy address
func (c *CLIConfig) GetProxyAddr() string {
	return fmt.Sprintf("%s:%d", c.Proxy.Bind, c.Proxy.Port)
}

// GetProxyURL returns the proxy URL for environment variables
func (c *CLIConfig) GetProxyURL() string {
	return fmt.Sprintf("http://%s", c.GetProxyAddr())
}

// IsPortAvailable checks if the configured port is available
func (c *CLIConfig) IsPortAvailable() bool {
	addr := fmt.Sprintf("%s:%d", c.Proxy.Bind, c.Proxy.Port)
	return IsPortAvailable(addr)
}

// ResetDailyStats resets daily statistics if needed
func (c *CLIConfig) ResetDailyStats() {
	lastReset, _ := time.Parse(time.RFC3339, c.Stats.LastReset)
	now := time.Now()

	if lastReset.Day() != now.Day() || lastReset.Month() != now.Month() || lastReset.Year() != now.Year() {
		c.Stats.RequestsToday = 0
		c.Stats.BlockedToday = 0
		c.Stats.WarnedToday = 0
		c.Stats.CostToday = 0
		c.Stats.LastReset = now.Format(time.RFC3339)
	}
}

// Platform returns the current platform
func Platform() string {
	return runtime.GOOS
}

// IsSupportedPlatform checks if the current platform is supported
func IsSupportedPlatform() bool {
	switch Platform() {
	case "linux", "darwin":
		return true
	default:
		return false
	}
}

