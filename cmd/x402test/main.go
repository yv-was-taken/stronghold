//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"stronghold/internal/wallet"
)

func main() {
	privateKey := os.Getenv("TEST_PRIVATE_KEY")
	if len(privateKey) > 2 && privateKey[:2] == "0x" {
		privateKey = privateKey[2:]
	}

	testWallet, _ := wallet.NewTestWalletFromKey(privateKey)
	fmt.Printf("Wallet: %s\n", testWallet.AddressString())

	apiURL := "https://api.getstronghold.xyz"

	// Get payment requirements from API
	reqBody := map[string]string{"text": "Hello test"}
	reqJSON, _ := json.Marshal(reqBody)
	
	resp, _ := http.Post(apiURL+"/v1/scan/content", "application/json", bytes.NewReader(reqJSON))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	
	var payResp struct {
		PaymentRequirements *wallet.PaymentRequirements `json:"payment_requirements"`
	}
	json.Unmarshal(body, &payResp)
	
	// Create payment and print debug info
	fmt.Println("\n=== Creating payment with debug ===")
	
	timestamp := time.Now().Unix()
	validAfter := int64(0)
	validBefore := timestamp + 300
	
	// Generate a nonce
	nonceHex := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	nonceBytes := common.FromHex(nonceHex)
	
	amount := big.NewInt(2000)
	chainID := int64(84532)
	tokenAddress := "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
	from := testWallet.AddressString()
	to := payResp.PaymentRequirements.Recipient
	
	fmt.Printf("chainID: %d\n", chainID)
	fmt.Printf("tokenAddress: %s\n", tokenAddress)
	fmt.Printf("from: %s\n", from)
	fmt.Printf("to: %s\n", to)
	fmt.Printf("value: %s\n", amount.String())
	fmt.Printf("validAfter: %d\n", validAfter)
	fmt.Printf("validBefore: %d\n", validBefore)
	fmt.Printf("nonce (hex): 0x%s\n", nonceHex)
	fmt.Printf("nonce (bytes len): %d\n", len(nonceBytes))

	// Build the EIP-712 typed data manually to inspect
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
			"value":       amount.String(),
			"validAfter":  big.NewInt(validAfter).String(),
			"validBefore": big.NewInt(validBefore).String(),
			"nonce":       nonceBytes,
		},
	}

	// Get the hash
	hash, rawData, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		fmt.Printf("ERROR hashing: %v\n", err)
		fmt.Printf("Raw data: %s\n", rawData)
		os.Exit(1)
	}
	
	fmt.Printf("\nTypedDataAndHash succeeded!\n")
	fmt.Printf("Hash: 0x%x\n", hash)
	fmt.Printf("Raw data: %s\n", rawData)
}
