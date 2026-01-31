package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all service configuration
type Config struct {
	Server   ServerConfig
	X402     X402Config
	Citadel  CitadelConfig
	Pricing  PricingConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// X402Config holds x402 payment configuration
type X402Config struct {
	WalletAddress   string
	FacilitatorURL  string
	Network         string
}

// CitadelConfig holds Citadel scanner configuration
type CitadelConfig struct {
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

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  getDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
		},
		X402: X402Config{
			WalletAddress:  getEnv("X402_WALLET_ADDRESS", ""),
			FacilitatorURL: getEnv("X402_FACILITATOR_URL", "https://x402.org/facilitator"),
			Network:        getEnv("X402_NETWORK", "base-sepolia"),
		},
		Citadel: CitadelConfig{
			BlockThreshold:  getFloat("CITADEL_BLOCK_THRESHOLD", 0.55),
			WarnThreshold:   getFloat("CITADEL_WARN_THRESHOLD", 0.35),
			EnableHugot:     getBool("CITADEL_ENABLE_HUGOT", true),
			EnableSemantics: getBool("CITADEL_ENABLE_SEMANTICS", true),
			HugotModelPath:  getEnv("HUGOT_MODEL_PATH", "./models"),
			LLMProvider:     getEnv("CITADEL_LLM_PROVIDER", ""),
			LLMAPIKey:       getEnv("CITADEL_LLM_API_KEY", ""),
		},
		Pricing: PricingConfig{
			ScanInput:     getFloat("PRICE_SCAN_INPUT", 0.001),
			ScanOutput:    getFloat("PRICE_SCAN_OUTPUT", 0.001),
			ScanUnified:   getFloat("PRICE_SCAN_UNIFIED", 0.002),
			ScanMultiturn: getFloat("PRICE_SCAN_MULTITURN", 0.005),
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

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
