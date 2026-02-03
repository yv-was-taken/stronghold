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

// AccountBalance displays the account balance and status
func AccountBalance() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to set up your account"))
		return nil
	}

	if config.Wallet.Address == "" {
		fmt.Println(accountWarningStyle.Render("⚠ Account not fully set up"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold init' to complete account setup"))
		return nil
	}

	// Load wallet to check balance
	w, err := wallet.New(wallet.Config{
		UserID:  config.Auth.UserID,
		Network: config.Wallet.Network,
	})
	if err != nil {
		return fmt.Errorf("failed to load account: %w", err)
	}

	fmt.Println(accountTitleStyle.Render("💳 Account"))
	fmt.Println()

	// Display address
	fmt.Println("Account ID:")
	fmt.Println(accountAddressStyle.Render("  " + config.Wallet.Address))
	fmt.Println()

	// Check balance
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	balance, err := w.GetBalanceHuman(ctx)
	if err != nil {
		fmt.Println(accountWarningStyle.Render("⚠ Could not fetch balance"))
		fmt.Println(accountInfoStyle.Render(fmt.Sprintf("  Error: %v", err)))
	} else {
		fmt.Printf("Balance: %s\n", accountBalanceStyle.Render(fmt.Sprintf("%.6f USDC", balance)))
		if balance < 1.0 {
			fmt.Println()
			fmt.Println(accountWarningStyle.Render("⚠ Low balance"))
			fmt.Println(accountInfoStyle.Render("  Run 'stronghold account deposit' to add funds"))
		}
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

	// Show wallet address for direct deposits
	if config.Wallet.Address != "" {
		fmt.Println("USDC Deposit Address (Base network):")
		fmt.Println(accountAddressStyle.Render("  " + config.Wallet.Address))
		fmt.Println()

		fmt.Println(accountInfoStyle.Render("Send USDC on Base network to the address above."))
		fmt.Println(accountInfoStyle.Render("Deposits are credited automatically after confirmation."))
		fmt.Println()
	}

	fmt.Println(accountInfoStyle.Render("Or visit the dashboard:"))
	fmt.Println(accountInfoStyle.Render("  https://dashboard.stronghold.security"))
	fmt.Println(accountInfoStyle.Render("  - Pay with card via Stripe"))
	fmt.Println(accountInfoStyle.Render("  - Coinbase Pay integration"))
	fmt.Println()

	fmt.Println(accountWarningStyle.Render("Important:"))
	fmt.Println(accountInfoStyle.Render("  - Only send USDC on Base network"))
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

// ImportWallet imports a wallet from a private key hex string
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

// ExportWallet exports the wallet private key to a file for backup
func ExportWallet(outputPath string) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		fmt.Println(accountErrorStyle.Render("✗ Not logged in"))
		fmt.Println(accountInfoStyle.Render("Run 'stronghold install' to set up your account"))
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
		fmt.Println(accountInfoStyle.Render("Run 'stronghold install' to set up your wallet"))
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

	// Export the key
	privateKey, err := w.Export()
	if err != nil {
		return fmt.Errorf("failed to export wallet: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to file with secure permissions (owner read/write only)
	if err := os.WriteFile(outputPath, []byte(privateKey), 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	fmt.Println()
	fmt.Println(accountTitleStyle.Render("✓ Wallet exported successfully"))
	fmt.Println(accountInfoStyle.Render("  Backup saved to: " + outputPath))

	return nil
}
