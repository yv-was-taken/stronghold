package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment represents the runtime environment
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
	EnvTest        Environment = "test"
)

// Config holds all service configuration
type Config struct {
	Environment Environment
	Server      ServerConfig
	Database    DatabaseConfig
	Auth        AuthConfig
	Cookie      CookieConfig
	Dashboard   DashboardConfig
	X402        X402Config
	Stripe      StripeConfig
	Stronghold  StrongholdConfig
	Pricing     PricingConfig
	RateLimit   RateLimitConfig
	KMS         KMSConfig
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

// StripeConfig holds Stripe payment configuration
type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
	PublishableKey string
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
	ScanContent float64
	ScanOutput  float64
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

// KMSConfig holds AWS KMS configuration for wallet key encryption
type KMSConfig struct {
	Region string // AWS region (e.g., "us-east-1")
	KeyID  string // KMS key ARN or alias (e.g., "alias/stronghold-wallet-keys")
}

// Load loads configuration from environment variables
func Load() *Config {
	// Default to production for security - explicit opt-in to development mode
	env := Environment(getEnv("ENV", "production"))
	if env != EnvDevelopment && env != EnvProduction && env != EnvTest {
		env = EnvProduction
	}

	return &Config{
		Environment: env,
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
		Stripe: StripeConfig{
			SecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
			WebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
			PublishableKey: getEnv("STRIPE_PUBLISHABLE_KEY", ""),
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
			ScanContent: getFloat("PRICE_SCAN_CONTENT", 0.001),
			ScanOutput:  getFloat("PRICE_SCAN_OUTPUT", 0.001),
		},
		RateLimit: RateLimitConfig{
			Enabled:       getBool("RATE_LIMIT_ENABLED", true),
			WindowSeconds: getInt("RATE_LIMIT_WINDOW_SECONDS", 60),
			MaxRequests:   getInt("RATE_LIMIT_MAX_REQUESTS", 100),
			LoginMax:      getInt("RATE_LIMIT_LOGIN_MAX", 5),
			AccountMax:    getInt("RATE_LIMIT_ACCOUNT_MAX", 3),
			RefreshMax:    getInt("RATE_LIMIT_REFRESH_MAX", 10),
		},
		KMS: KMSConfig{
			Region: getEnv("KMS_REGION", ""),
			KeyID:  getEnv("KMS_KEY_ID", ""),
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

// Validate checks that all required configuration is present.
// In production, missing critical values will return an error.
// In development, it will use insecure defaults and log warnings.
func (c *Config) Validate() error {
	var errs []string

	// JWT_SECRET is critical for authentication security
	if c.Auth.JWTSecret == "" {
		if c.Environment == EnvProduction {
			errs = append(errs, "JWT_SECRET is required in production")
		}
	} else if c.Environment == EnvProduction && len(c.Auth.JWTSecret) < 32 {
		errs = append(errs, "JWT_SECRET must be at least 32 characters in production")
	}

	// Database password should be set in production
	if c.Database.Password == "" {
		if c.Environment == EnvProduction {
			errs = append(errs, "DB_PASSWORD is required in production")
		}
	}

	// Stripe keys are required in production
	if c.Environment == EnvProduction {
		if c.Stripe.SecretKey == "" {
			errs = append(errs, "STRIPE_SECRET_KEY is required in production")
		}
		if c.Stripe.WebhookSecret == "" {
			errs = append(errs, "STRIPE_WEBHOOK_SECRET is required in production")
		}
		if c.Stripe.PublishableKey == "" {
			errs = append(errs, "STRIPE_PUBLISHABLE_KEY is required in production")
		}
	}

	// CORS validation: wildcard origins are insecure when credentials are allowed
	// The server uses AllowCredentials: true, so wildcards must be rejected
	for _, origin := range c.Dashboard.AllowedOrigins {
		if origin == "*" {
			errs = append(errs, "DASHBOARD_ALLOWED_ORIGINS cannot contain wildcard '*' (credentials are enabled)")
			break
		}
	}

	// KMS is required in production for wallet key encryption
	if c.Environment == EnvProduction {
		if c.KMS.Region == "" {
			errs = append(errs, "KMS_REGION is required in production")
		}
		if c.KMS.KeyID == "" {
			errs = append(errs, "KMS_KEY_ID is required in production")
		}
	}

	if len(errs) > 0 {
		return errors.New("configuration errors: " + strings.Join(errs, "; "))
	}

	return nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Environment == EnvDevelopment
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Environment == EnvProduction
}
