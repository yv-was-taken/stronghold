package wallet

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// TestWallet is a wallet that doesn't require OS keyring access.
// Use this for unit and integration tests.
type TestWallet struct {
	privateKey *ecdsa.PrivateKey
	Address    common.Address
	network    string
}

// NewTestWallet creates a new test wallet with a random private key
func NewTestWallet() (*TestWallet, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	address := crypto.PubkeyToAddress(*publicKey)

	return &TestWallet{
		privateKey: privateKey,
		Address:    address,
		network:    "base-sepolia",
	}, nil
}

// NewTestWalletFromKey creates a test wallet from a hex-encoded private key
func NewTestWalletFromKey(privateKeyHex string) (*TestWallet, error) {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	address := crypto.PubkeyToAddress(*publicKey)

	return &TestWallet{
		privateKey: privateKey,
		Address:    address,
		network:    "base-sepolia",
	}, nil
}

// AddressString returns the wallet address as a hex string
func (w *TestWallet) AddressString() string {
	return w.Address.Hex()
}

// Exists returns true since a test wallet always exists once created
func (w *TestWallet) Exists() bool {
	return w.privateKey != nil
}

// SetNetwork sets the network for this wallet
func (w *TestWallet) SetNetwork(network string) {
	w.network = network
}

// CreateX402Payment creates a signed x402 payment for testing
// Uses proper EIP-3009 TransferWithAuthorization format
func (w *TestWallet) CreateX402Payment(req *PaymentRequirements) (string, error) {
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

	timestamp := time.Now().Unix()
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
	signature, err := w.signEIP3009(
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

// signEIP3009 signs an EIP-3009 TransferWithAuthorization using proper EIP-712 encoding
// Reference implementation: https://github.com/brtvcl/eip-3009-transferWithAuthorization-example
// V value format: viem uses 27/28, per https://github.com/wevm/viem/blob/main/src/accounts/utils/sign.ts
func (w *TestWallet) signEIP3009(chainID int64, tokenAddress, from, to string, value *big.Int, validAfter, validBefore int64, nonce []byte) ([]byte, error) {
	// Convert nonce to hex string with 0x prefix using go-ethereum's hexutil
	// This is equivalent to ethers.hexlify() which produces "0x..." format
	nonceHex := hexutil.Encode(nonce)

	// Build the EIP-712 typed data using go-ethereum's proper implementation
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
	sig, err := crypto.Sign(hash, w.privateKey)
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

// CreateTestPaymentHeader creates a payment header for testing with the given parameters
func (w *TestWallet) CreateTestPaymentHeader(recipient string, amountWei string, network string) (string, error) {
	req := &PaymentRequirements{
		Scheme:    "x402",
		Network:   network,
		Recipient: recipient,
		Amount:    amountWei,
		Currency:  "USDC",
	}
	return w.CreateX402Payment(req)
}
