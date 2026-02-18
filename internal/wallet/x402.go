// Package wallet provides x402 payment functionality
package wallet

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"stronghold/internal/usdc"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// timeNow is a variable for testing (allows mocking time)
var timeNow = time.Now

// PaymentPayloadV2 represents the x402 v2 payment payload sent to facilitator
type PaymentPayloadV2 struct {
	X402Version int                      `json:"x402Version"`
	Payload     map[string]interface{}   `json:"payload"`
	Accepted    PaymentRequirementsV2    `json:"accepted"`
}

// PaymentRequirementsV2 represents the x402 v2 payment requirements
type PaymentRequirementsV2 struct {
	Scheme            string                 `json:"scheme"`
	Network           string                 `json:"network"`
	Asset             string                 `json:"asset"`
	Amount            string                 `json:"amount"`
	PayTo             string                 `json:"payTo"`
	MaxTimeoutSeconds int                    `json:"maxTimeoutSeconds"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
}

// EIP3009Authorization represents the authorization for EIP-3009 transferWithAuthorization
type EIP3009Authorization struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	ValidAfter  string `json:"validAfter"`
	ValidBefore string `json:"validBefore"`
	Nonce       string `json:"nonce"`
}

// X402Payload represents our internal payment payload
type X402Payload struct {
	Network      string `json:"network"`
	Scheme       string `json:"scheme"`
	Payer        string `json:"payer"`
	Receiver     string `json:"receiver"`
	TokenAddress string `json:"tokenAddress"`
	Amount       string `json:"amount"`
	Timestamp    int64  `json:"timestamp"`
	Nonce        string `json:"nonce"`
	Signature    string `json:"signature,omitempty"`   // EVM: hex encoded EIP-3009 signature
	Transaction  string `json:"transaction,omitempty"` // Solana: base64 encoded partially-signed transaction
}

// PaymentRequirements represents the 402 response from the server
type PaymentRequirements struct {
	Scheme         string `json:"scheme"`
	Network        string `json:"network"`
	Recipient      string `json:"recipient"`
	Amount         string `json:"amount"`
	Currency       string `json:"currency"`
	FacilitatorURL string `json:"facilitator_url"`
	Description    string `json:"description"`
	FeePayer       string `json:"fee_payer,omitempty"` // Solana only: facilitator's pubkey for fee payment
}

// X402Config holds x402 configuration
type X402Config struct {
	Network        string
	TokenAddress   string
	FacilitatorURL string
	ChainID        int
}

// x402NetworkConfigs maps network names to their configurations
var x402NetworkConfigs = map[string]X402Config{
	"base": {
		Network:        "base",
		TokenAddress:   "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", // USDC on Base
		FacilitatorURL: "https://x402.org/facilitator",
		ChainID:        8453,
	},
	"base-sepolia": {
		Network:        "base-sepolia",
		TokenAddress:   "0x036CbD53842c5426634e7929541eC2318f3dCF7e", // USDC on Base Sepolia
		FacilitatorURL: "https://x402.org/facilitator",
		ChainID:        84532,
	},
	"solana": {
		Network:        "solana",
		TokenAddress:   USDCSolanaMint,
		FacilitatorURL: "https://x402.org/facilitator",
	},
	"solana-devnet": {
		Network:        "solana-devnet",
		TokenAddress:   USDCSolanaDevnetMint,
		FacilitatorURL: "https://x402.org/facilitator",
	},
}

// networkToCAIP2 converts our network names to CAIP-2 format
func networkToCAIP2(network string) string {
	switch network {
	case "base":
		return "eip155:8453"
	case "base-sepolia":
		return "eip155:84532"
	case "solana":
		return "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp"
	case "solana-devnet":
		return "solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1"
	default:
		// If already in CAIP-2 format, return as-is
		if len(network) > 7 && network[:7] == "eip155:" {
			return network
		}
		if len(network) > 7 && network[:7] == "solana:" {
			return network
		}
		// Fallback to base-sepolia
		return "eip155:84532"
	}
}

// IsNetworkSupported returns true if the network has a known x402 configuration
func IsNetworkSupported(network string) bool {
	_, ok := x402NetworkConfigs[network]
	return ok
}

// IsSolanaNetwork returns true if the network is a Solana network
func IsSolanaNetwork(network string) bool {
	return network == "solana" || network == "solana-devnet" || strings.HasPrefix(network, "solana:")
}

// X402Configs for supported networks (exported for compatibility)
var (
	X402BaseMainnet  = x402NetworkConfigs["base"]
	X402BaseSepolia  = x402NetworkConfigs["base-sepolia"]
	X402Solana       = x402NetworkConfigs["solana"]
	X402SolanaDevnet = x402NetworkConfigs["solana-devnet"]
)

// CreateX402Payment creates a signed x402 payment for the given requirements
func (w *Wallet) CreateX402Payment(req *PaymentRequirements) (string, error) {
	if !w.Exists() {
		return "", fmt.Errorf("wallet not initialized")
	}

	// Get x402 config for network
	x402Config, ok := x402NetworkConfigs[req.Network]
	if !ok {
		return "", fmt.Errorf("unsupported network: %s", req.Network)
	}

	// Create payment payload
	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("failed to generate payment nonce: %w", err)
	}

	timestamp := timeNow().Unix()
	validAfter := int64(0)
	validBefore := timestamp + 300 // 5 minute validity window

	// Parse amount as big.Int
	amount := new(big.Int)
	if _, ok := amount.SetString(req.Amount, 10); !ok {
		return "", fmt.Errorf("invalid amount: %s", req.Amount)
	}

	// Generate nonce as bytes
	nonceBytes := common.FromHex(nonce)

	payload := X402Payload{
		Network:      x402Config.Network,
		Scheme:       "x402",
		Payer:        w.Address.Hex(),
		Receiver:     req.Recipient,
		TokenAddress: x402Config.TokenAddress,
		Amount:       req.Amount,
		Timestamp:    timestamp,
		Nonce:        nonce,
	}

	// Sign using proper EIP-3009 TransferWithAuthorization format
	signature, err := w.SignEIP3009(
		int64(x402Config.ChainID),
		x402Config.TokenAddress,
		w.Address.Hex(), // from
		req.Recipient,   // to
		amount,
		validAfter,
		validBefore,
		nonceBytes,
	)
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

// FacilitatorRequest represents the request body for facilitator /verify and /settle endpoints.
// x402-rs reads x402Version from the root JSON to route v1 vs v2 requests.
type FacilitatorRequest struct {
	X402Version         int                   `json:"x402Version"`
	PaymentPayload      PaymentPayloadV2      `json:"paymentPayload"`
	PaymentRequirements PaymentRequirementsV2 `json:"paymentRequirements"`
}

// BuildFacilitatorRequest creates the x402 v2 format request for the facilitator.
// Handles both EVM (EIP-3009) and Solana (serialized transaction) payloads.
func BuildFacilitatorRequest(payload *X402Payload, originalReq *PaymentRequirements) *FacilitatorRequest {
	caip2Network := networkToCAIP2(payload.Network)

	if IsSolanaNetwork(payload.Network) {
		return buildSolanaFacilitatorRequest(payload, caip2Network)
	}
	return buildEVMFacilitatorRequest(payload, caip2Network)
}

// buildEVMFacilitatorRequest builds the facilitator request for EVM (Base) payments
func buildEVMFacilitatorRequest(payload *X402Payload, caip2Network string) *FacilitatorRequest {
	// Calculate validity window (5 minutes from timestamp)
	validAfter := "0"
	validBefore := fmt.Sprintf("%d", payload.Timestamp+300)

	// Format nonce with 0x prefix for EIP-3009
	nonce := hexutil.Encode(common.FromHex(payload.Nonce))

	// Build EIP-3009 authorization
	authorization := EIP3009Authorization{
		From:        payload.Payer,
		To:          payload.Receiver,
		Value:       payload.Amount,
		ValidAfter:  validAfter,
		ValidBefore: validBefore,
		Nonce:       nonce,
	}

	paymentReqs := PaymentRequirementsV2{
		Scheme:            "exact",
		Network:           caip2Network,
		Asset:             payload.TokenAddress,
		Amount:            payload.Amount,
		PayTo:             payload.Receiver,
		MaxTimeoutSeconds: 300,
		Extra: map[string]interface{}{
			"assetTransferMethod": "eip3009",
			"name":                "USD Coin",
			"version":             "2",
		},
	}

	paymentPayload := PaymentPayloadV2{
		X402Version: 2,
		Payload: map[string]interface{}{
			"signature":     payload.Signature,
			"authorization": authorization,
		},
		Accepted: paymentReqs,
	}

	return &FacilitatorRequest{
		X402Version:         2,
		PaymentPayload:      paymentPayload,
		PaymentRequirements: paymentReqs,
	}
}

// buildSolanaFacilitatorRequest builds the facilitator request for Solana payments
func buildSolanaFacilitatorRequest(payload *X402Payload, caip2Network string) *FacilitatorRequest {
	paymentReqs := PaymentRequirementsV2{
		Scheme:            "exact",
		Network:           caip2Network,
		Asset:             payload.TokenAddress,
		Amount:            payload.Amount,
		PayTo:             payload.Receiver,
		MaxTimeoutSeconds: 300,
		Extra: map[string]interface{}{
			"assetTransferMethod": "solana-transfer",
		},
	}

	paymentPayload := PaymentPayloadV2{
		X402Version: 2,
		Payload: map[string]interface{}{
			"transaction": payload.Transaction,
		},
		Accepted: paymentReqs,
	}

	return &FacilitatorRequest{
		X402Version:         2,
		PaymentPayload:      paymentPayload,
		PaymentRequirements: paymentReqs,
	}
}

// BuildFacilitatorRequestFromHeader parses a payment header and builds the facilitator request
func BuildFacilitatorRequestFromHeader(paymentHeader string, originalReq *PaymentRequirements) (*FacilitatorRequest, error) {
	payload, err := ParseX402Payment(paymentHeader)
	if err != nil {
		return nil, err
	}
	return BuildFacilitatorRequest(payload, originalReq), nil
}

// VerifyPaymentSignature verifies that a payment signature is valid
// This can be used client-side to verify before sending
func VerifyPaymentSignature(payload *X402Payload, expectedPayer string) error {
	// Calculate validity window from timestamp
	validAfter := int64(0)
	validBefore := payload.Timestamp + 300

	// Parse the amount
	amount := new(big.Int)
	if _, ok := amount.SetString(payload.Amount, 10); !ok {
		return fmt.Errorf("invalid amount: %s", payload.Amount)
	}

	chainID, err := getChainID(payload.Network)
	if err != nil {
		return fmt.Errorf("cannot verify EVM signature: %w", err)
	}

	// Convert nonce to hex string with 0x prefix using hexutil
	// payload.Nonce comes without 0x prefix, so we decode and re-encode properly
	nonceBytes := common.FromHex(payload.Nonce)
	nonceHex := hexutil.Encode(nonceBytes)

	// Build the EIP-712 typed data using go-ethereum's proper implementation
	// This must match exactly what SignEIP3009 uses
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
			ChainId:           math.NewHexOrDecimal256(int64(chainID)),
			VerifyingContract: payload.TokenAddress,
		},
		Message: apitypes.TypedDataMessage{
			"from":        payload.Payer,
			"to":          payload.Receiver,
			"value":       (*math.HexOrDecimal256)(amount),
			"validAfter":  math.NewHexOrDecimal256(validAfter),
			"validBefore": math.NewHexOrDecimal256(validBefore),
			"nonce":       nonceHex,
		},
	}

	// Get the hash using go-ethereum's proper EIP-712 implementation
	hash, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return fmt.Errorf("failed to hash typed data: %w", err)
	}

	// Parse signature
	sigBytes := common.FromHex(payload.Signature)
	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length: got %d, want 65", len(sigBytes))
	}

	// Normalize V value for recovery: EIP-712 uses 27/28, go-ethereum expects 0/1
	sigForRecovery := make([]byte, 65)
	copy(sigForRecovery, sigBytes)
	if sigForRecovery[64] >= 27 {
		sigForRecovery[64] -= 27
	}

	// Recover public key
	recoveredPubKey, err := crypto.SigToPub(hash, sigForRecovery)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %w", err)
	}

	recoveredAddr := crypto.PubkeyToAddress(*recoveredPubKey)
	expectedAddr := common.HexToAddress(expectedPayer)
	if recoveredAddr != expectedAddr {
		return fmt.Errorf("signature mismatch: recovered %s, expected %s", recoveredAddr.Hex(), expectedAddr.Hex())
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

func generateNonce() (string, error) {
	// Generate a cryptographically secure random nonce
	// Use 32 bytes (256 bits) to reduce birthday collision risk at scale
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

func getChainID(network string) (int, error) {
	switch network {
	case "base":
		return 8453, nil
	case "base-sepolia":
		return 84532, nil
	default:
		return 0, fmt.Errorf("unsupported network for chain ID: %s (Solana networks use CAIP-2 identifiers, not numeric chain IDs)", network)
	}
}

// GetTokenDecimals returns the decimals for a given token
func GetTokenDecimals(tokenAddress string) int {
	// USDC has 6 decimals on all networks (both EVM and Solana)
	switch tokenAddress {
	case USDCBaseAddress, USDCSolanaMint, USDCSolanaDevnetMint:
		return 6
	}
	if common.HexToAddress(tokenAddress) == common.HexToAddress(USDCBaseAddress) {
		return 6
	}
	return 18 // Default for ERC20
}

// AmountToWei converts a MicroUSDC amount to on-chain atomic units (*big.Int) for the given chain.
// Since MicroUSDC is already in integer domain, this is an exact conversion with no precision loss.
func AmountToWei(amount usdc.MicroUSDC, chain string) *big.Int {
	return amount.ToBigInt(chain)
}

// WeiToAmount converts on-chain atomic units to MicroUSDC.
func WeiToAmount(amount *big.Int, chain string) usdc.MicroUSDC {
	return usdc.FromBigInt(amount, chain)
}
