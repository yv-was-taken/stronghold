package config

import (
	"strings"
	"testing"
)

func TestValidateProductionRequiresAtLeastOneX402Wallet(t *testing.T) {
	cfg := validProductionConfig()
	cfg.X402 = X402Config{Networks: []string{"base", "solana"}}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error when no x402 wallet addresses are configured")
	}
	if !strings.Contains(err.Error(), "at least one X402 wallet address") {
		t.Fatalf("expected x402 wallet validation error, got: %v", err)
	}
}

func TestValidateProductionAllowsEVMWallet(t *testing.T) {
	cfg := validProductionConfig()
	cfg.X402 = X402Config{
		EVMWalletAddress: "0x1234567890123456789012345678901234567890",
		Networks:         []string{"base"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validation to pass with EVM wallet configured, got: %v", err)
	}
}

func TestValidateProductionAllowsSolanaWallet(t *testing.T) {
	cfg := validProductionConfig()
	cfg.X402 = X402Config{
		SolanaWalletAddress: "7xKXtg2CWYuV7i8UEz5B2oS6x9fPVkDz7M8f8f8f8f8f",
		Networks:            []string{"solana"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validation to pass with Solana wallet configured, got: %v", err)
	}
}

func TestLoadX402NetworksReturnsEmptyWhenNoWalletsConfigured(t *testing.T) {
	t.Setenv("X402_NETWORKS", "")
	t.Setenv("X402_NETWORK", "")
	t.Setenv("X402_EVM_WALLET_ADDRESS", "")
	t.Setenv("X402_WALLET_ADDRESS", "")
	t.Setenv("X402_SOLANA_WALLET_ADDRESS", "")

	networks := loadX402Networks()
	if len(networks) != 0 {
		t.Fatalf("expected no networks when no wallets are configured, got: %v", networks)
	}
}

func TestLoadX402NetworksAutoDetectsWallets(t *testing.T) {
	t.Setenv("X402_NETWORKS", "")
	t.Setenv("X402_NETWORK", "")
	t.Setenv("X402_EVM_WALLET_ADDRESS", "0x1234567890123456789012345678901234567890")
	t.Setenv("X402_WALLET_ADDRESS", "")
	t.Setenv("X402_SOLANA_WALLET_ADDRESS", "")

	networks := loadX402Networks()
	if len(networks) != 1 || networks[0] != "base" {
		t.Fatalf("expected auto-detected base network, got: %v", networks)
	}
}

func TestValidateDevelopmentPassesWithoutX402Wallets(t *testing.T) {
	cfg := &Config{
		Environment: EnvDevelopment,
		Database: DatabaseConfig{
			Password: "db-password",
		},
		Auth: AuthConfig{
			JWTSecret: strings.Repeat("a", 32),
		},
		Dashboard: DashboardConfig{
			AllowedOrigins: []string{"http://localhost:3000"},
		},
		Stripe: StripeConfig{
			SecretKey:      "sk_test_123",
			WebhookSecret:  "whsec_123",
			PublishableKey: "pk_test_123",
		},
		KMS: KMSConfig{
			Region: "us-east-1",
			KeyID:  "alias/stronghold-wallet-keys",
		},
		Stronghold: StrongholdConfig{
			BlockThreshold: 0.55,
			WarnThreshold:  0.35,
		},
		X402: X402Config{}, // no wallets configured
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validation to pass in development without x402 wallets, got: %v", err)
	}
}

func validProductionConfig() *Config {
	return &Config{
		Environment: EnvProduction,
		Database: DatabaseConfig{
			Password: "db-password",
		},
		Auth: AuthConfig{
			JWTSecret: strings.Repeat("a", 32),
		},
		Dashboard: DashboardConfig{
			AllowedOrigins: []string{"https://dashboard.example.com"},
		},
		Stripe: StripeConfig{
			SecretKey:      "sk_test_123",
			WebhookSecret:  "whsec_123",
			PublishableKey: "pk_test_123",
		},
		KMS: KMSConfig{
			Region: "us-east-1",
			KeyID:  "alias/stronghold-wallet-keys",
		},
		Stronghold: StrongholdConfig{
			BlockThreshold: 0.55,
			WarnThreshold:  0.35,
		},
	}
}
