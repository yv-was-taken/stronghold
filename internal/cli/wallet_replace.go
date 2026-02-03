package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"stronghold/internal/wallet"

	"golang.org/x/term"
)

// WalletReplace replaces the current wallet with a new private key
// fileFlag: path to file containing private key
// yesFlag: skip confirmation warnings
func WalletReplace(fileFlag string, yesFlag bool) error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if !config.Auth.LoggedIn {
		return fmt.Errorf("not logged in. Run 'stronghold init' first")
	}

	// Read private key from various sources (in order of precedence)
	privateKey, err := readPrivateKey(fileFlag)
	if err != nil {
		return err
	}
	defer privateKey.Zero() // CRITICAL: Always zero the key when done

	// Validate the private key format
	cleanedKey, err := ValidatePrivateKeyHex(privateKey.String())
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	// Check BOTH config AND keyring for existing wallet
	walletExistsInConfig := config.Wallet.Address != ""
	walletExistsInKeyring := false

	if config.Auth.UserID != "" {
		w, err := wallet.New(wallet.Config{
			UserID:  config.Auth.UserID,
			Network: config.Wallet.Network,
		})
		if err == nil {
			walletExistsInKeyring = w.Exists()
		}
	}

	// Warn if wallet exists in EITHER location
	if (walletExistsInConfig || walletExistsInKeyring) && !yesFlag {
		address := config.Wallet.Address
		if address == "" {
			address = "(wallet found in system keyring)"
		}
		fmt.Printf("\n%s Existing wallet detected: %s\n", warningStyle.Render("WARNING:"), address)
		fmt.Println("Any funds not backed up will be lost.")
		fmt.Print("\nType 'yes' to continue or 'no' to cancel: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(strings.ToLower(response)) != "yes" {
			return fmt.Errorf("wallet replacement cancelled")
		}
	}

	// Import the new wallet
	userID := config.Auth.UserID
	if userID == "" {
		userID = generateUserID()
		config.Auth.UserID = userID
	}

	address, err := ImportWallet(userID, DefaultBlockchain, cleanedKey)
	if err != nil {
		return fmt.Errorf("failed to import wallet: %w", err)
	}

	// Update wallet on server if logged in
	apiClient := NewAPIClient(config.API.Endpoint)
	if err := apiClient.UpdateWallet(cleanedKey); err != nil {
		fmt.Printf("%s Could not update wallet on server: %v\n", warningStyle.Render("WARNING:"), err)
		fmt.Println("Wallet updated locally only. Server sync may be needed later.")
	} else {
		fmt.Println(successStyle.Render("✓ Wallet updated on server"))
	}

	// Update config
	config.Wallet.Address = address
	config.Wallet.Network = DefaultBlockchain
	if err := config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("%s Wallet updated: %s\n", successStyle.Render("✓"), address)
	return nil
}

// readPrivateKey reads the private key from various sources in order of precedence:
// 1. stdin (if piped)
// 2. STRONGHOLD_PRIVATE_KEY environment variable
// 3. --file flag
// 4. interactive prompt (if terminal)
//
// Returns a SecureBytes that must be zeroed when done (caller should use defer key.Zero()).
func readPrivateKey(fileFlag string) (*SecureBytes, error) {
	// 1. Check stdin (if piped)
	stdinInfo, _ := os.Stdin.Stat()
	if (stdinInfo.Mode() & os.ModeCharDevice) == 0 {
		// stdin has data piped to it
		reader := bufio.NewReader(os.Stdin)
		key, err := reader.ReadString('\n')
		if err != nil && key == "" {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		return NewSecureBytes([]byte(strings.TrimSpace(key))), nil
	}

	// 2. Check environment variable
	if envKey := os.Getenv("STRONGHOLD_PRIVATE_KEY"); envKey != "" {
		return NewSecureBytes([]byte(strings.TrimSpace(envKey))), nil
	}

	// 3. Check file flag
	if fileFlag != "" {
		info, err := os.Stat(fileFlag)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("key file not found: %s", fileFlag)
			}
			if os.IsPermission(err) {
				return nil, fmt.Errorf("permission denied reading key file: %s", fileFlag)
			}
			return nil, fmt.Errorf("failed to stat key file: %w", err)
		}
		if info.Size() == 0 {
			return nil, fmt.Errorf("key file is empty: %s", fileFlag)
		}
		if info.Size() > MaxKeyFileSize {
			return nil, fmt.Errorf("key file too large: %d bytes (max %d)", info.Size(), MaxKeyFileSize)
		}
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
		// Create SecureBytes from file data (trimmed)
		trimmed := strings.TrimSpace(string(data))
		// Zero the original data slice
		for i := range data {
			data[i] = 0
		}
		return NewSecureBytes([]byte(trimmed)), nil
	}

	// 4. Interactive prompt (only if terminal)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("Enter private key (hex): ")
		key, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // newline after password input
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}
		// Create SecureBytes and zero original
		trimmed := strings.TrimSpace(string(key))
		for i := range key {
			key[i] = 0
		}
		return NewSecureBytes([]byte(trimmed)), nil
	}

	return nil, fmt.Errorf("no private key provided. Use stdin, STRONGHOLD_PRIVATE_KEY env var, --file flag, or run interactively")
}
