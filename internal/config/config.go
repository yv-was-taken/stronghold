package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"stronghold/internal/usdc"
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
	Port           string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	ProxyHeader    string
	TrustedProxies []string
}

// DatabaseConfig holds PostgreSQL database configuration
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
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
	EVMWalletAddress    string   // EVM wallet address (Base)
	SolanaWalletAddress string   // Solana wallet address
	FacilitatorURL      string   // x402 facilitator URL
	Networks            []string // Supported payment networks (e.g. ["base", "solana"])
	SolanaFeePayer      string   // Facilitator's Solana pubkey for paying tx fees
}

// WalletForNetwork returns the wallet address for the given network.
// Returns empty string if no wallet is configured for that network
// or the network is unknown.
func (c *X402Config) WalletForNetwork(network string) string {
	switch network {
	case "solana", "solana-devnet":
		return c.SolanaWalletAddress
	case "base", "base-sepolia":
		return c.EVMWalletAddress
	default:
		return "" // unknown network
	}
}

// HasPayments returns true if at least one network has a configured wallet
func (c *X402Config) HasPayments() bool {
	for _, network := range c.Networks {
		if c.WalletForNetwork(network) != "" {
			return true
		}
	}
	return false
}

// StripeConfig holds Stripe payment configuration
type StripeConfig struct {
	SecretKey      string
	WebhookSecret  string
	PublishableKey string
}

// StrongholdConfig holds Stronghold scanner configuration
type StrongholdConfig struct {
	BlockThreshold  float64
	WarnThreshold   float64
	EnableHugot     bool
	EnableSemantics bool
	HugotModelPath  string
	LLMProvider     string
	LLMAPIKey       string
}

// PricingConfig holds endpoint pricing in microUSDC
type PricingConfig struct {
	ScanContent usdc.MicroUSDC
	ScanOutput  usdc.MicroUSDC
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
			Port:           getEnv("PORT", "8080"),
			ReadTimeout:    getDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:   getDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			ProxyHeader:    getEnv("PROXY_HEADER", "X-Forwarded-For"),
			TrustedProxies: getEnvSlice("TRUSTED_PROXIES", nil),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "stronghold"),
			Password: getEnv("DB_PASSWORD", ""),
			Name:     getEnv("DB_NAME", "stronghold"),
			SSLMode:  getEnv("DB_SSLMODE", "require"),
			MaxConns: int32(getInt("DB_MAX_CONNS", 0)),
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
			EVMWalletAddress:    getEnvWithFallback("X402_EVM_WALLET_ADDRESS", "X402_WALLET_ADDRESS", ""),
			SolanaWalletAddress: getEnv("X402_SOLANA_WALLET_ADDRESS", ""),
			FacilitatorURL:      getEnv("X402_FACILITATOR_URL", "https://x402.org/facilitator"),
			Networks:            loadX402Networks(),
			SolanaFeePayer:      getEnv("X402_SOLANA_FEE_PAYER", ""),
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
			ScanContent: getMicroUSDC("PRICE_SCAN_CONTENT", 0.001),
			ScanOutput:  getMicroUSDC("PRICE_SCAN_OUTPUT", 0.001),
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

// getEnvWithFallback tries the primary key first, then falls back to a legacy key
func getEnvWithFallback(primary, fallback, defaultValue string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	if value := os.Getenv(fallback); value != "" {
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

// getMicroUSDC parses a human-readable float env var (e.g. "0.001") into MicroUSDC.
func getMicroUSDC(key string, defaultFloat float64) usdc.MicroUSDC {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return usdc.FromFloat(f)
		}
		slog.Warn("invalid microUSDC env value, using default", "key", key, "value", value, "default_usdc", defaultFloat)
	}
	return usdc.FromFloat(defaultFloat)
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

// loadX402Networks loads the list of supported payment networks.
// Reads from X402_NETWORKS (comma-separated) first, falls back to
// the legacy X402_NETWORK (singular) env var, then auto-detects
// from configured wallet addresses.
func loadX402Networks() []string {
	if value := os.Getenv("X402_NETWORKS"); value != "" {
		var networks []string
		for _, n := range strings.Split(value, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				networks = append(networks, n)
			}
		}
		return networks
	}
	// Backward compatibility: fall back to singular X402_NETWORK
	if value := os.Getenv("X402_NETWORK"); value != "" {
		return []string{value}
	}
	// Auto-detect networks from configured wallet addresses
	var networks []string
	if os.Getenv("X402_EVM_WALLET_ADDRESS") != "" || os.Getenv("X402_WALLET_ADDRESS") != "" {
		networks = append(networks, "base")
	}
	if os.Getenv("X402_SOLANA_WALLET_ADDRESS") != "" {
		networks = append(networks, "solana")
	}
	// When no wallets are configured, return an empty list so config state is explicit.
	return networks
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

	// x402 wallet addresses are required in production so paid endpoints cannot silently bypass payment.
	if c.Environment == EnvProduction && !c.X402.HasPayments() {
		errs = append(errs, "at least one X402 wallet address (X402_EVM_WALLET_ADDRESS or X402_SOLANA_WALLET_ADDRESS) is required in production")
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

	// Validate scanner thresholds are within valid range
	if c.Stronghold.BlockThreshold < 0.0 || c.Stronghold.BlockThreshold > 1.0 {
		errs = append(errs, "STRONGHOLD_BLOCK_THRESHOLD must be between 0.0 and 1.0")
	}
	if c.Stronghold.WarnThreshold < 0.0 || c.Stronghold.WarnThreshold > 1.0 {
		errs = append(errs, "STRONGHOLD_WARN_THRESHOLD must be between 0.0 and 1.0")
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
