package wallet

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
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
	signature, err := w.signTypedData(typedData)
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

// signTypedData signs EIP-712 typed data
func (w *TestWallet) signTypedData(typedData *TypedData) ([]byte, error) {
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("failed to hash domain: %w", err)
	}

	typedDataHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to hash message: %w", err)
	}

	// Construct the final hash: keccak256("\x19\x01" || domainSeparator || structHash)
	rawData := []byte("\x19\x01")
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, typedDataHash...)

	sig, err := crypto.Sign(crypto.Keccak256(rawData), w.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign typed data: %w", err)
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
