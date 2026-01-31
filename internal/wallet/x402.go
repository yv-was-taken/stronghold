// Package wallet provides x402 payment functionality
package wallet

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// X402Payment represents a signed x402 payment
type X402Payment struct {
	Scheme  string `json:"scheme"`
	Payload string `json:"payload"` // base64 encoded
}

// X402Payload represents the actual x402 payment payload
type X402Payload struct {
	Network         string `json:"network"`
	Scheme          string `json:"scheme"`
	Payer           string `json:"payer"`
	Receiver        string `json:"receiver"`
	TokenAddress    string `json:"tokenAddress"`
	Amount          string `json:"amount"`
	Timestamp       int64  `json:"timestamp"`
	Nonce           string `json:"nonce"`
	Signature       string `json:"signature"` // hex encoded
}

// PaymentRequirements represents the 402 response from the server
type PaymentRequirements struct {
	Scheme          string `json:"scheme"`
	Network         string `json:"network"`
	Recipient       string `json:"recipient"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	FacilitatorURL  string `json:"facilitator_url"`
	Description     string `json:"description"`
}

// X402Config holds x402 configuration
type X402Config struct {
	Network        string
	TokenAddress   string
	FacilitatorURL string
	ChainID        int
}

// X402Configs for supported networks
var (
	X402BaseMainnet = X402Config{
		Network:        "base",
		TokenAddress:   "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", // USDC on Base
		FacilitatorURL: "https://x402.org/facilitator",
		ChainID:        8453,
	}
	X402BaseSepolia = X402Config{
		Network:        "base-sepolia",
		TokenAddress:   "0x036CbD53842c5426634e7929541eC2318f3dCF7e", // USDC on Base Sepolia
		FacilitatorURL: "https://x402.org/facilitator",
		ChainID:        84532,
	}
)

// CreateX402Payment creates a signed x402 payment for the given requirements
func (w *Wallet) CreateX402Payment(req *PaymentRequirements) (string, error) {
	if !w.Exists() {
		return "", fmt.Errorf("wallet not initialized")
	}

	// Get x402 config for network
	var x402Config X402Config
	switch req.Network {
	case "base":
		x402Config = X402BaseMainnet
	case "base-sepolia":
		x402Config = X402BaseSepolia
	default:
		return "", fmt.Errorf("unsupported network: %s", req.Network)
	}

	// Create payment payload
	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("failed to generate payment nonce: %w", err)
	}
	payload := X402Payload{
		Network:      x402Config.Network,
		Scheme:       "x402",
		Payer:        w.Address.Hex(),
		Receiver:     req.Recipient,
		TokenAddress: x402Config.TokenAddress,
		Amount:       req.Amount,
		Timestamp:    time.Now().Unix(),
		Nonce:        nonce,
	}

	// Create EIP-712 typed data for signing
	typedData := &TypedData{
		Types: map[string][]TypedDataField{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Payment": {
				{Name: "receiver", Type: "address"},
				{Name: "tokenAddress", Type: "address"},
				{Name: "amount", Type: "uint256"},
				{Name: "timestamp", Type: "uint256"},
				{Name: "nonce", Type: "string"},
			},
		},
		PrimaryType: "Payment",
		Domain: TypedDataDomain{
			Name:              "x402",
			Version:           "1",
			ChainID:           x402Config.ChainID,
			VerifyingContract: req.Recipient,
		},
		Message: map[string]interface{}{
			"receiver":     req.Recipient,
			"tokenAddress": x402Config.TokenAddress,
			"amount":       req.Amount,
			"timestamp":    payload.Timestamp,
			"nonce":        nonce,
		},
	}

	// Sign the typed data
	signature, err := w.SignTypedData(typedData)
	if err != nil {
		return "", fmt.Errorf("failed to sign payment: %w", err)
	}

	// Add signature to payload
	payload.Signature = fmt.Sprintf("0x%x", signature)

	// Encode payload to JSON
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create final payment header: "x402;base64payload"
	payment := fmt.Sprintf("x402;%s", base64.StdEncoding.EncodeToString(payloadJSON))

	return payment, nil
}

// VerifyPaymentSignature verifies that a payment signature is valid
// This can be used client-side to verify before sending
func VerifyPaymentSignature(payload *X402Payload, expectedPayer string) error {
	// Reconstruct the typed data hash
	typedData := &TypedData{
		Types: map[string][]TypedDataField{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Payment": {
				{Name: "receiver", Type: "address"},
				{Name: "tokenAddress", Type: "address"},
				{Name: "amount", Type: "uint256"},
				{Name: "timestamp", Type: "uint256"},
				{Name: "nonce", Type: "string"},
			},
		},
		PrimaryType: "Payment",
		Domain: TypedDataDomain{
			Name:              "x402",
			Version:           "1",
			ChainID:           getChainID(payload.Network),
			VerifyingContract: payload.Receiver,
		},
		Message: map[string]interface{}{
			"receiver":     payload.Receiver,
			"tokenAddress": payload.TokenAddress,
			"amount":       payload.Amount,
			"timestamp":    payload.Timestamp,
			"nonce":        payload.Nonce,
		},
	}

	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return fmt.Errorf("failed to hash domain: %w", err)
	}

	typedDataHash, err := typedData.HashStruct("Payment", typedData.Message)
	if err != nil {
		return fmt.Errorf("failed to hash message: %w", err)
	}

	rawData := []byte("\x19\x01")
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, typedDataHash...)
	hash := crypto.Keccak256(rawData)

	// Parse signature
	sigBytes := common.FromHex(payload.Signature)
	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length")
	}

	// Recover public key
	recoveredPubKey, err := crypto.SigToPub(hash, sigBytes)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %w", err)
	}

	recoveredAddr := crypto.PubkeyToAddress(*recoveredPubKey)
	if recoveredAddr.Hex() != expectedPayer {
		return fmt.Errorf("signature mismatch: recovered %s, expected %s", recoveredAddr.Hex(), expectedPayer)
	}

	return nil
}

// ParseX402Payment parses an x402 payment header
func ParseX402Payment(paymentHeader string) (*X402Payload, error) {
	// Parse header format: "x402;base64payload"
	parts := make([]string, 0)
	current := ""
	for i, c := range paymentHeader {
		if c == ';' && i > 0 && paymentHeader[i-1] != '\\' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)

	if len(parts) != 2 || parts[0] != "x402" {
		return nil, fmt.Errorf("invalid payment header format")
	}

	// Decode base64 payload
	payloadJSON, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %w", err)
	}

	var payload X402Payload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &payload, nil
}

// Helper functions

func generateNonce() (string, error) {
	// Generate a cryptographically secure random nonce
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

func getChainID(network string) int {
	switch network {
	case "base":
		return 8453
	case "base-sepolia":
		return 84532
	default:
		return 8453
	}
}

// GetTokenDecimals returns the decimals for a given token
func GetTokenDecimals(tokenAddress string) int {
	// USDC has 6 decimals on all networks
	if common.HexToAddress(tokenAddress) == common.HexToAddress(USDCBaseAddress) {
		return 6
	}
	return 18 // Default for ERC20
}

// AmountToWei converts a human-readable amount to wei-like units (based on token decimals)
func AmountToWei(amount float64, decimals int) *big.Int {
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	amountInt := big.NewInt(int64(amount * float64(multiplier.Int64())))
	return amountInt
}

// WeiToAmount converts wei-like units to human-readable amount
func WeiToAmount(amount *big.Int, decimals int) float64 {
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	amountFloat := new(big.Float).SetInt(amount)
	divisorFloat := new(big.Float).SetInt(divisor)
	result, _ := new(big.Float).Quo(amountFloat, divisorFloat).Float64()
	return result
}
