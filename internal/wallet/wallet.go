// Package wallet provides secure wallet management for Stronghold CLI
// Wallet keys are stored in the OS keychain and linked to user accounts
package wallet

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/99designs/keyring"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	// USDC contract address on Base
	USDCBaseAddress = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	// Base mainnet RPC (public fallback)
	BaseMainnetRPC = "https://mainnet.base.org"
	// Base sepolia RPC (for testing)
	BaseSepoliaRPC = "https://sepolia.base.org"
)

// USDC ABI for balanceOf (just the function selector)
var usdcBalanceOfSelector = crypto.Keccak256([]byte("balanceOf(address)"))[:4]

// Wallet represents a user's Ethereum wallet linked to their account
type Wallet struct {
	Address    common.Address
	userID     string
	keyring    keyring.Keyring
	network    string
	rpcURL     string
}

// Config holds wallet configuration
type Config struct {
	UserID  string
	Network string // "base" or "base-sepolia"
}

// New creates or loads a wallet for the given user
func New(cfg Config) (*Wallet, error) {
	// Determine RPC URL
	rpcURL := BaseMainnetRPC
	if cfg.Network == "base-sepolia" {
		rpcURL = BaseSepoliaRPC
	}

	// Open keyring with platform-specific configuration
	ring, err := openKeyring()
	if err != nil {
		return nil, fmt.Errorf("failed to open keyring: %w", err)
	}

	w := &Wallet{
		userID:  cfg.UserID,
		keyring: ring,
		network: cfg.Network,
		rpcURL:  rpcURL,
	}

	// Try to load existing wallet
	if err := w.load(); err != nil {
		// No existing wallet, will need to be created
		return w, nil
	}

	return w, nil
}

// openKeyring opens the OS keyring with appropriate configuration
func openKeyring() (keyring.Keyring, error) {
	// On Linux, check what's available and provide explicit errors
	if runtime.GOOS == "linux" {
		return openLinuxKeyring()
	}

	// macOS and Windows use their native keyrings
	config := keyring.Config{
		ServiceName:              "stronghold",
		KeychainName:             "stronghold",
		KeychainTrustApplication: true,
	}

	ring, err := keyring.Open(config)
	if err != nil {
		return nil, fmt.Errorf("failed to open system keyring: %w", err)
	}

	return ring, nil
}

// openLinuxKeyring tries Linux-specific backends with explicit error messages
func openLinuxKeyring() (keyring.Keyring, error) {
	var errors []string

	// Try Secret Service first (most common)
	if hasSecretService() {
		ring, err := keyring.Open(keyring.Config{
			ServiceName:              "stronghold",
			KeychainName:             "stronghold",
			KeychainTrustApplication: true,
			AllowedBackends:          []keyring.BackendType{keyring.SecretServiceBackend},
		})
		if err == nil {
			return ring, nil
		}
		errors = append(errors, fmt.Sprintf("Secret Service: %v", err))
	} else {
		errors = append(errors, "Secret Service: DBUS_SESSION_BUS_ADDRESS not set (is a desktop session running?)")
	}

	// Try KWallet
	if hasKWallet() {
		ring, err := keyring.Open(keyring.Config{
			ServiceName:              "stronghold",
			KeychainName:             "stronghold",
			KeychainTrustApplication: true,
			AllowedBackends:          []keyring.BackendType{keyring.KWalletBackend},
		})
		if err == nil {
			return ring, nil
		}
		errors = append(errors, fmt.Sprintf("KWallet: %v", err))
	} else {
		errors = append(errors, "KWallet: KDE_SESSION_VERSION not set (not running KDE?)")
	}

	// Try pass
	if hasPass() {
		ring, err := keyring.Open(keyring.Config{
			ServiceName:              "stronghold",
			KeychainName:             "stronghold",
			KeychainTrustApplication: true,
			AllowedBackends:          []keyring.BackendType{keyring.PassBackend},
		})
		if err == nil {
			return ring, nil
		}
		errors = append(errors, fmt.Sprintf("pass: %v", err))
	} else {
		errors = append(errors, "pass: 'pass' command not found in PATH (install: sudo apt install pass)")
	}

	// None worked - return detailed error
	return nil, fmt.Errorf("no secure keyring available:\n  - %s\n\nInstall one of the above and try again. See: stronghold doctor", strings.Join(errors, "\n  - "))
}

// Create generates a new wallet for the user
func (w *Wallet) Create() (*Wallet, error) {
	// Generate new key pair
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Get address
	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	w.Address = crypto.PubkeyToAddress(*publicKey)

	// Store private key in keyring
	privateKeyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))
	if err := w.keyring.Set(keyring.Item{
		Key:  w.keyID(),
		Data: []byte(privateKeyHex),
	}); err != nil {
		return nil, fmt.Errorf("failed to store key: %w", err)
	}

	return w, nil
}

// Import creates a wallet from an existing private key
func (w *Wallet) Import(privateKeyHex string) (*Wallet, error) {
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	w.Address = crypto.PubkeyToAddress(*publicKey)

	if err := w.keyring.Set(keyring.Item{
		Key:  w.keyID(),
		Data: []byte(hex.EncodeToString(crypto.FromECDSA(privateKey))),
	}); err != nil {
		return nil, fmt.Errorf("failed to store key: %w", err)
	}

	return w, nil
}

// AddressString returns the wallet address as a hex string
func (w *Wallet) AddressString() string {
	return w.Address.Hex()
}

// Exists returns true if a wallet exists for this user
func (w *Wallet) Exists() bool {
	_, err := w.keyring.Get(w.keyID())
	return err == nil
}

// Export returns the private key as a hex string
// WARNING: Handle with extreme care - this exposes sensitive key material
func (w *Wallet) Export() (string, error) {
	item, err := w.keyring.Get(w.keyID())
	if err != nil {
		return "", fmt.Errorf("wallet not found: %w", err)
	}
	return string(item.Data), nil
}

// GetBalance returns the USDC balance for this wallet
func (w *Wallet) GetBalance(ctx context.Context) (*big.Int, error) {
	if w.Address == (common.Address{}) {
		return nil, fmt.Errorf("wallet not initialized")
	}

	client, err := ethclient.DialContext(ctx, w.rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to network: %w", err)
	}
	defer client.Close()

	// Build balanceOf call data
	data := append(usdcBalanceOfSelector, common.LeftPadBytes(w.Address.Bytes(), 32)...)

	// Call the contract
	msg := map[string]interface{}{
		"to":   USDCBaseAddress,
		"data": hex.EncodeToString(data),
	}
	
	var result string
	if err := client.Client().CallContext(ctx, &result, "eth_call", msg, "latest"); err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Parse result
	balance := new(big.Int)
	balance.SetString(strings.TrimPrefix(result, "0x"), 16)
	
	return balance, nil
}

// GetBalanceHuman returns the USDC balance as a human-readable float
func (w *Wallet) GetBalanceHuman(ctx context.Context) (float64, error) {
	balance, err := w.GetBalance(ctx)
	if err != nil {
		return 0, err
	}

	// USDC has 6 decimals
	balanceFloat := new(big.Float).SetInt(balance)
	divisor := big.NewFloat(1_000_000)
	result, _ := new(big.Float).Quo(balanceFloat, divisor).Float64()
	
	return result, nil
}

// Sign signs data with the wallet's private key
func (w *Wallet) Sign(data []byte) ([]byte, error) {
	privateKey, err := w.getPrivateKey()
	if err != nil {
		return nil, err
	}
	defer w.zeroKey(privateKey)

	sig, err := crypto.Sign(crypto.Keccak256(data), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return sig, nil
}

// SignEIP3009 signs an EIP-3009 TransferWithAuthorization using proper EIP-712 encoding
// Reference implementation: https://github.com/brtvcl/eip-3009-transferWithAuthorization-example
func (w *Wallet) SignEIP3009(chainID int64, tokenAddress, from, to string, value *big.Int, validAfter, validBefore int64, nonce []byte) ([]byte, error) {
	privateKey, err := w.getPrivateKey()
	if err != nil {
		return nil, err
	}
	defer w.zeroKey(privateKey)

	// Convert nonce to hex string with 0x prefix using go-ethereum's hexutil
	// This is equivalent to ethers.hexlify() which produces "0x..." format
	nonceHex := hexutil.Encode(nonce)

	// Build the EIP-712 typed data using go-ethereum's proper implementation
	// Reference format from working ethers.js implementation:
	// - value: BigInt (we use *math.HexOrDecimal256)
	// - validAfter/validBefore: numbers (we use *math.HexOrDecimal256)
	// - nonce: hex string with 0x prefix
	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"TransferWithAuthorization": []apitypes.Type{
				{Name: "from", Type: "address"},
				{Name: "to", Type: "address"},
				{Name: "value", Type: "uint256"},
				{Name: "validAfter", Type: "uint256"},
				{Name: "validBefore", Type: "uint256"},
				{Name: "nonce", Type: "bytes32"},
			},
		},
		PrimaryType: "TransferWithAuthorization",
		Domain: apitypes.TypedDataDomain{
			Name:              "USD Coin",
			Version:           "2",
			ChainId:           math.NewHexOrDecimal256(chainID),
			VerifyingContract: tokenAddress,
		},
		Message: apitypes.TypedDataMessage{
			"from":        from,
			"to":          to,
			"value":       (*math.HexOrDecimal256)(value),
			"validAfter":  math.NewHexOrDecimal256(validAfter),
			"validBefore": math.NewHexOrDecimal256(validBefore),
			"nonce":       nonceHex, // Hex string with 0x prefix per ethers.js convention
		},
	}

	// Get the hash using go-ethereum's proper EIP-712 implementation
	hash, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return nil, fmt.Errorf("failed to hash typed data: %w", err)
	}

	// Sign the hash
	sig, err := crypto.Sign(hash, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Adjust V value for EIP-712: go-ethereum returns 0/1, but EIP-712 expects 27/28
	// Reference: viem uses v = recovery ? 28n : 27n
	if sig[64] < 27 {
		sig[64] += 27
	}

	return sig, nil
}

// Private helper methods

func (w *Wallet) keyID() string {
	return fmt.Sprintf("wallet-%s", w.userID)
}

func (w *Wallet) load() error {
	item, err := w.keyring.Get(w.keyID())
	if err != nil {
		return err
	}

	privateKey, err := crypto.HexToECDSA(string(item.Data))
	if err != nil {
		return fmt.Errorf("failed to parse stored key: %w", err)
	}

	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	w.Address = crypto.PubkeyToAddress(*publicKey)

	return nil
}

func (w *Wallet) getPrivateKey() (*ecdsa.PrivateKey, error) {
	item, err := w.keyring.Get(w.keyID())
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	privateKey, err := crypto.HexToECDSA(string(item.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}

	return privateKey, nil
}

func (w *Wallet) zeroKey(key *ecdsa.PrivateKey) {
	// Zero out the key data for security
	if key != nil && key.D != nil {
		key.D.SetUint64(0)
	}
}

// CheckKeyringAvailability checks if a secure keyring is available
func CheckKeyringAvailability() (available bool, backend string, err error) {
	ring, err := openKeyring()
	if err != nil {
		return false, "", err
	}

	// Try to get the backend type
	backend = "unknown"

	// The keyring package doesn't expose the backend directly, but we can infer
	// by trying to store and retrieve a test item
	testItem := keyring.Item{
		Key:  "__test__",
		Data: []byte("test"),
	}

	if err := ring.Set(testItem); err != nil {
		return false, "", fmt.Errorf("keyring test failed: %w", err)
	}

	_, err = ring.Get("__test__")
	if err != nil {
		return false, "", fmt.Errorf("keyring read test failed: %w", err)
	}

	// Clean up test item
	ring.Remove("__test__")

	// Try to detect which backend is being used based on platform
	switch runtime.GOOS {
	case "darwin":
		backend = "keychain"
	case "linux":
		// Check which backend was successfully opened
		// We only reach here if openKeyring() succeeded
		if hasSecretService() {
			backend = "secret-service"
		} else if hasKWallet() {
			backend = "kwallet"
		} else {
			backend = "pass"
		}
	case "windows":
		backend = "wincred"
	}

	return true, backend, nil
}

// GetKeyringHelp returns help text for setting up keyring on the current OS
func GetKeyringHelp() string {
	if runtime.GOOS == "darwin" {
		return `macOS Keychain:

Stronghold uses the macOS Keychain to securely store wallet private keys.
No setup required - it works automatically.
`
	}
	return GetLinuxKeyringHelp()
}

// GetLinuxKeyringHelp returns help text for setting up keyring on Linux
func GetLinuxKeyringHelp() string {
	return fmt.Sprintf(`Linux Keyring Setup:

Stronghold uses your system's keyring to securely store wallet private keys.

Supported backends: Secret Service (GNOME Keyring), KWallet, pass

Recommended: Install gnome-keyring and libsecret using your package manager.

%s
`, getDistroInstallHint())
}

func getDistroInstallHint() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Example: sudo apt install gnome-keyring libsecret-1-0"
	}
	content := strings.ToLower(string(data))

	switch {
	case strings.Contains(content, "arch") || strings.Contains(content, "manjaro") || strings.Contains(content, "endeavour"):
		return "sudo pacman -S gnome-keyring libsecret"
	case strings.Contains(content, "fedora") || strings.Contains(content, "rhel") || strings.Contains(content, "centos") || strings.Contains(content, "rocky"):
		return "sudo dnf install gnome-keyring libsecret"
	case strings.Contains(content, "opensuse") || strings.Contains(content, "suse"):
		return "sudo zypper install gnome-keyring libsecret"
	case strings.Contains(content, "void"):
		return "sudo xbps-install gnome-keyring libsecret"
	case strings.Contains(content, "alpine"):
		return "sudo apk add gnome-keyring libsecret"
	default:
		return "sudo apt install gnome-keyring libsecret-1-0"
	}
}

// hasSecretService checks if D-Bus Secret Service is available
func hasSecretService() bool {
	// Check if we can connect to D-Bus session
	return os.Getenv("DBUS_SESSION_BUS_ADDRESS") != ""
}

// hasKWallet checks if KWallet is available
func hasKWallet() bool {
	// KWallet is usually available on KDE
	return os.Getenv("KDE_SESSION_VERSION") != ""
}

// hasPass checks if pass is installed
func hasPass() bool {
	_, err := execLookPath("pass")
	return err == nil
}

// execLookPath is a wrapper for exec.LookPath
func execLookPath(file string) (string, error) {
	paths := strings.Split(os.Getenv("PATH"), string(filepath.ListSeparator))
	for _, dir := range paths {
		path := filepath.Join(dir, file)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("not found")
}

// Delete removes the wallet from the keyring
func (w *Wallet) Delete() error {
	return w.keyring.Remove(w.keyID())
}

// GetNetwork returns the network this wallet is configured for
func (w *Wallet) GetNetwork() string {
	return w.network
}

// IsMainnet returns true if this wallet is on Base mainnet
func (w *Wallet) IsMainnet() bool {
	return w.network == "base"
}
