package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all service configuration
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Auth       AuthConfig
	Cookie     CookieConfig
	Dashboard  DashboardConfig
	X402       X402Config
	Stronghold StrongholdConfig
	Pricing    PricingConfig
	RateLimit  RateLimitConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DatabaseConfig holds PostgreSQL database configuration
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// AuthConfig holds JWT authentication configuration
type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// CookieConfig holds httpOnly cookie configuration
type CookieConfig struct {
	Domain   string
	Secure   bool
	SameSite string
}

// DashboardConfig holds dashboard configuration
type DashboardConfig struct {
	URL            string
	AllowedOrigins []string
}

// X402Config holds x402 payment configuration
type X402Config struct {
	WalletAddress   string
	FacilitatorURL  string
	Network         string
}

// StrongholdConfig holds Stronghold scanner configuration
type StrongholdConfig struct {
	BlockThreshold   float64
	WarnThreshold    float64
	EnableHugot      bool
	EnableSemantics  bool
	HugotModelPath   string
	LLMProvider      string
	LLMAPIKey        string
}

// PricingConfig holds endpoint pricing in USD
type PricingConfig struct {
	ScanInput    float64
	ScanOutput   float64
	ScanUnified  float64
	ScanMultiturn float64
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled       bool
	WindowSeconds int
	MaxRequests   int
	LoginMax      int
	AccountMax    int
	RefreshMax    int
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  getDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "stronghold"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "stronghold"),
			SSLMode:  getEnv("DB_SSLMODE", "require"),
		},
		Auth: AuthConfig{
			JWTSecret:       getEnv("JWT_SECRET", ""),
			AccessTokenTTL:  getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL: getDuration("REFRESH_TOKEN_TTL", 90*24*time.Hour),
		},
		Cookie: CookieConfig{
			Domain:   getEnv("COOKIE_DOMAIN", ""),
			Secure:   getBool("COOKIE_SECURE", true),
			SameSite: getEnv("COOKIE_SAMESITE", "Strict"),
		},
		Dashboard: DashboardConfig{
			URL:            getEnv("DASHBOARD_URL", "http://localhost:3000"),
			AllowedOrigins: getEnvSlice("DASHBOARD_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		},
		X402: X402Config{
			WalletAddress:  getEnv("X402_WALLET_ADDRESS", ""),
			FacilitatorURL: getEnv("X402_FACILITATOR_URL", "https://x402.org/facilitator"),
			Network:        getEnv("X402_NETWORK", "base-sepolia"),
		},
		Stronghold: StrongholdConfig{
			BlockThreshold:  getFloat("STRONGHOLD_BLOCK_THRESHOLD", 0.55),
			WarnThreshold:   getFloat("STRONGHOLD_WARN_THRESHOLD", 0.35),
			EnableHugot:     getBool("STRONGHOLD_ENABLE_HUGOT", true),
			EnableSemantics: getBool("STRONGHOLD_ENABLE_SEMANTICS", true),
			HugotModelPath:  getEnv("HUGOT_MODEL_PATH", "./models"),
			LLMProvider:     getEnv("STRONGHOLD_LLM_PROVIDER", ""),
			LLMAPIKey:       getEnv("STRONGHOLD_LLM_API_KEY", ""),
		},
		Pricing: PricingConfig{
			ScanInput:     getFloat("PRICE_SCAN_INPUT", 0.001),
			ScanOutput:    getFloat("PRICE_SCAN_OUTPUT", 0.001),
			ScanUnified:   getFloat("PRICE_SCAN_UNIFIED", 0.002),
			ScanMultiturn: getFloat("PRICE_SCAN_MULTITURN", 0.005),
		},
		RateLimit: RateLimitConfig{
			Enabled:       getBool("RATE_LIMIT_ENABLED", true),
			WindowSeconds: getInt("RATE_LIMIT_WINDOW_SECONDS", 60),
			MaxRequests:   getInt("RATE_LIMIT_MAX_REQUESTS", 100),
			LoginMax:      getInt("RATE_LIMIT_LOGIN_MAX", 5),
			AccountMax:    getInt("RATE_LIMIT_ACCOUNT_MAX", 3),
			RefreshMax:    getInt("RATE_LIMIT_REFRESH_MAX", 10),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
