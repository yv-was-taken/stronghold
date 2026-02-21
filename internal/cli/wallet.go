package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
	"stronghold/internal/wallet"
)

var (
	accountTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#00D4AA"))

	accountAddressStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF"))

	accountBalanceStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#00D4AA"))

	accountInfoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))

	accountWarningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFA500"))

	accountErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF4444"))
)

// AccountBalance displays account balances (legacy alias for wallet balance).
func AccountBalance() error {
	return WalletBalance()
}

// WalletBalance displays wallet balances and status by chain.
func WalletBalance() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your account"))
		return nil
	}

	if config.Wallet.Address == "" && config.Wallet.SolanaAddress == "" {
		fmt.Println(accountWarningStyle.Render("⚠ Account not fully set up"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to complete account setup"))
		return nil
	}

	fmt.Println(accountTitleStyle.Render("💳 Wallet Balances"))
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Show EVM (Base) wallet balance
	if config.Wallet.Address != "" {
		fmt.Println("Base (EVM) Wallet:")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.Address))

		w, err := wallet.New(wallet.Config{
			UserID:  config.Auth.UserID,
			Network: config.Wallet.Network,
		})
		if err != nil {
			fmt.Println(accountWarningStyle.Render(fmt.Sprintf("  ⚠ Could not load wallet: %v", err)))
		} else {
			balance, err := w.GetBalanceHuman(ctx)
			if err != nil {
				fmt.Println(accountWarningStyle.Render("  ⚠ Could not fetch balance"))
			} else {
				fmt.Printf("  Balance: %s\n", accountBalanceStyle.Render(fmt.Sprintf("%.6f USDC", balance)))
			}
		}
		fmt.Println()
	}

	// Show Solana wallet balance
	if config.Wallet.SolanaAddress != "" {
		fmt.Println("Solana Wallet:")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.SolanaAddress))

		solanaNetwork := config.Wallet.SolanaNetwork
		if solanaNetwork == "" {
			solanaNetwork = DefaultSolanaNetwork
		}
		sw, err := wallet.NewSolana(wallet.SolanaConfig{
			UserID:  config.Auth.UserID,
			Network: solanaNetwork,
		})
		if err != nil {
			fmt.Println(accountWarningStyle.Render(fmt.Sprintf("  ⚠ Could not load wallet: %v", err)))
		} else {
			balance, err := sw.GetBalanceHuman(ctx)
			if err != nil {
				fmt.Println(accountWarningStyle.Render("  ⚠ Could not fetch balance"))
			} else {
				fmt.Printf("  Balance: %s\n", accountBalanceStyle.Render(fmt.Sprintf("%.6f USDC", balance)))
			}
		}
		fmt.Println()
	}

	return nil
}

// WalletList displays configured wallets by chain.
func WalletList() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your account"))
		return nil
	}

	fmt.Println(accountTitleStyle.Render("🔐 Wallets"))
	fmt.Println()

	if config.Wallet.Address != "" {
		fmt.Println("EVM (Base):")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.Address))
	} else {
		fmt.Println("EVM (Base):")
		fmt.Println(accountWarningStyle.Render("  Not configured"))
	}
	fmt.Println()

	if config.Wallet.SolanaAddress != "" {
		fmt.Println("Solana:")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.SolanaAddress))
	} else {
		fmt.Println("Solana:")
		fmt.Println(accountWarningStyle.Render("  Not configured"))
	}
	fmt.Println()

	if config.Wallet.Address == "" && config.Wallet.SolanaAddress == "" {
		fmt.Println(accountInfoStyle.Render("Use 'stronghold wallet replace evm' or 'stronghold wallet replace solana' to configure wallets."))
	}

	return nil
}

// AccountDeposit shows deposit address for direct USDC deposits
func AccountDeposit() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your account"))
		return nil
	}

	fmt.Println(accountTitleStyle.Render("💳 Add Funds"))
	fmt.Println()

	// Show account number for dashboard login
	if config.Auth.AccountNumber != "" {
		fmt.Println("Your Account Number:")
		fmt.Println(accountAddressStyle.Render("  " + config.Auth.AccountNumber))
		fmt.Println()
	}

	// Show Base (EVM) deposit address
	if config.Wallet.Address != "" {
		fmt.Println("Base (EVM) Deposit Address:")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.Address))
		fmt.Println(accountInfoStyle.Render("  Send USDC on Base network."))
		fmt.Println()
	}

	// Show Solana deposit address
	if config.Wallet.SolanaAddress != "" {
		fmt.Println("Solana Deposit Address:")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.SolanaAddress))
		fmt.Println(accountInfoStyle.Render("  Send USDC on Solana network."))
		fmt.Println()
	}

	fmt.Println(accountInfoStyle.Render("Or visit the dashboard:"))
	fmt.Println(accountInfoStyle.Render("  https://getstronghold.xyz/dashboard"))
	fmt.Println(accountInfoStyle.Render("  - Pay with card via Stripe"))
	fmt.Println()

	fmt.Println(accountWarningStyle.Render("Important:"))
	fmt.Println(accountInfoStyle.Render("  - Only send USDC on supported networks (Base, Solana)"))
	fmt.Println(accountInfoStyle.Render("  - Sending other tokens may result in permanent loss"))
	fmt.Println(accountInfoStyle.Render("  - Deposits typically arrive in 1-2 minutes"))

	return nil
}

// SetupWallet creates or loads a wallet for the user during install
// In production, this would call the backend API which creates the wallet
// and returns the address. For now, we create it locally.
func SetupWallet(userID string, network string) (string, error) {
	w, err := wallet.New(wallet.Config{
		UserID:  userID,
		Network: network,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initialize wallet: %w", err)
	}

	// Check if wallet already exists
	if w.Exists() {
		return w.AddressString(), nil
	}

	// Create new wallet
	if _, err := w.Create(); err != nil {
		return "", fmt.Errorf("failed to create wallet: %w", err)
	}

	return w.AddressString(), nil
}

// SetupSolanaWallet creates or loads a Solana wallet for the user during install
func SetupSolanaWallet(userID string, network string) (string, error) {
	w, err := wallet.NewSolana(wallet.SolanaConfig{
		UserID:  userID,
		Network: network,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initialize Solana wallet: %w", err)
	}

	if w.Exists() {
		return w.AddressString(), nil
	}

	if _, err := w.Create(); err != nil {
		return "", fmt.Errorf("failed to create Solana wallet: %w", err)
	}

	return w.AddressString(), nil
}

// ImportWallet imports an EVM wallet from a private key hex string
// This is used when logging in on a new device to restore the server-stored wallet
func ImportWallet(userID string, network string, privateKeyHex string) (string, error) {
	w, err := wallet.New(wallet.Config{
		UserID:  userID,
		Network: network,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initialize wallet: %w", err)
	}

	// Import the private key
	if _, err := w.Import(privateKeyHex); err != nil {
		return "", fmt.Errorf("failed to import wallet: %w", err)
	}

	return w.AddressString(), nil
}

// ImportSolanaWallet imports a Solana wallet from a base58-encoded private key
func ImportSolanaWallet(userID string, network string, privateKeyBase58 string) (string, error) {
	w, err := wallet.NewSolana(wallet.SolanaConfig{
		UserID:  userID,
		Network: network,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initialize Solana wallet: %w", err)
	}

	if _, err := w.Import(privateKeyBase58); err != nil {
		return "", fmt.Errorf("failed to import Solana wallet: %w", err)
	}

	return w.AddressString(), nil
}

// WalletLink registers locally-configured wallet public keys with the server.
// Requires login and TOTP (trusted device).
func WalletLink() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn || config.Auth.AccountNumber == "" {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your account"))
		return nil
	}

	evmAddr := config.Wallet.Address
	solanaAddr := config.Wallet.SolanaAddress

	if evmAddr == "" && solanaAddr == "" {
		fmt.Println(accountWarningStyle.Render("⚠ No wallets configured locally"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' or 'stronghold wallet replace' to set up wallets"))
		return nil
	}

	// Login to API
	apiClient := NewAPIClient(config.API.Endpoint, config.Auth.DeviceToken)
	loginResp, err := apiClient.Login(config.Auth.AccountNumber)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Ensure trusted device (TOTP required for this endpoint)
	if err := ensureTrustedDevice(apiClient, config, loginResp.TOTPRequired); err != nil {
		return fmt.Errorf("TOTP verification failed: %w", err)
	}

	// Build request with locally-configured addresses
	req := &UpdateWalletAddressesRequest{}
	if evmAddr != "" {
		req.EVMAddress = &evmAddr
	}
	if solanaAddr != "" {
		req.SolanaAddress = &solanaAddr
	}

	resp, err := apiClient.UpdateWalletAddresses(req)
	if err != nil {
		return fmt.Errorf("failed to register wallet addresses: %w", err)
	}

	// Save any updated device token
	if err := config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Report results
	fmt.Println(accountTitleStyle.Render("✓ Wallet addresses registered with server"))
	fmt.Println()
	if resp.EVMWalletAddress != nil {
		fmt.Printf("  EVM:    %s\n", *resp.EVMWalletAddress)
	}
	if resp.SolanaWalletAddress != nil {
		fmt.Printf("  Solana: %s\n", *resp.SolanaWalletAddress)
	}

	return nil
}

// ExportWallet exports the wallet private key to a file for backup
func ExportWallet(outputPath string) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your account"))
		return nil
	}

	// Load wallet
	w, err := wallet.New(wallet.Config{
		UserID:  config.Auth.UserID,
		Network: config.Wallet.Network,
	})
	if err != nil {
		return fmt.Errorf("failed to load wallet: %w", err)
	}

	if !w.Exists() {
		fmt.Println(accountErrorStyle.Render("✗ No wallet found"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your wallet"))
		return nil
	}

	// Determine output path
	if outputPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		outputPath = filepath.Join(homeDir, ".stronghold", "wallet-backup")
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		fmt.Println(accountWarningStyle.Render("⚠ Backup already exists at " + outputPath))
		return nil
	}

	// Display security warning
	fmt.Println(accountWarningStyle.Render("⚠ WARNING: This will export your private key"))
	fmt.Println()
	fmt.Println(accountInfoStyle.Render("Your private key grants full control over your wallet."))
	fmt.Println(accountInfoStyle.Render("Anyone with access to this key can spend your funds."))
	fmt.Println()
	fmt.Println(accountInfoStyle.Render("Keep this backup secure:"))
	fmt.Println(accountInfoStyle.Render("  - Store offline if possible"))
	fmt.Println(accountInfoStyle.Render("  - Never share it with anyone"))
	fmt.Println(accountInfoStyle.Render("  - Delete after importing to a secure wallet"))
	fmt.Println()

	// Prompt for confirmation
	if !Confirm("Export private key? [y/N]") {
		fmt.Println(accountInfoStyle.Render("Export cancelled"))
		return nil
	}

	// Export the EVM key
	privateKey, err := w.Export()
	if err != nil {
		return fmt.Errorf("failed to export wallet: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write EVM key to file with secure permissions
	if err := os.WriteFile(outputPath, []byte(privateKey), 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	fmt.Println()
	fmt.Println(accountTitleStyle.Render("✓ Base (EVM) wallet exported"))
	fmt.Println(accountInfoStyle.Render("  Backup saved to: " + outputPath))

	// Also export Solana wallet if it exists
	if config.Wallet.SolanaAddress != "" {
		solanaNetwork := config.Wallet.SolanaNetwork
		if solanaNetwork == "" {
			solanaNetwork = DefaultSolanaNetwork
		}
		sw, err := wallet.NewSolana(wallet.SolanaConfig{
			UserID:  config.Auth.UserID,
			Network: solanaNetwork,
		})
		if err == nil && sw.Exists() {
			solanaKey, err := sw.Export()
			if err == nil {
				solanaPath := outputPath + "-solana"
				if err := os.WriteFile(solanaPath, []byte(solanaKey), 0600); err == nil {
					fmt.Println()
					fmt.Println(accountTitleStyle.Render("✓ Solana wallet exported"))
					fmt.Println(accountInfoStyle.Render("  Backup saved to: " + solanaPath))
				}
			}
		}
	}

	return nil
}
