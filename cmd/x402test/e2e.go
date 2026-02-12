//go:build ignore

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"stronghold/internal/wallet"
)

func main() {
	apiURL := os.Getenv("STRONGHOLD_API_URL")
	if apiURL == "" {
		apiURL = "https://api.getstronghold.xyz"
	}
	fmt.Printf("API URL: %s\n", apiURL)

	evmKey := os.Getenv("TEST_PRIVATE_KEY")
	solanaKey := os.Getenv("TEST_SOLANA_PRIVATE_KEY")

	if evmKey == "" && solanaKey == "" {
		fmt.Println("ERROR: Set TEST_PRIVATE_KEY (EVM hex) and/or TEST_SOLANA_PRIVATE_KEY (Solana base58)")
		os.Exit(1)
	}

	exitCode := 0

	if evmKey != "" {
		fmt.Println("\n========================================")
		fmt.Println("  EVM (Base) E2E Test")
		fmt.Println("========================================")
		if !runEVMTest(apiURL, evmKey) {
			exitCode = 1
		}
	}

	if solanaKey != "" {
		fmt.Println("\n========================================")
		fmt.Println("  Solana E2E Test")
		fmt.Println("========================================")
		if !runSolanaTest(apiURL, solanaKey) {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}

// get402Response makes an unauthenticated request to get 402 payment requirements
func get402Response(apiURL string) ([]byte, error) {
	reqBody := map[string]string{"text": "Hello, this is a test of x402 payments."}
	reqJSON, _ := json.Marshal(reqBody)

	resp, err := http.Post(apiURL+"/v1/scan/content", "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)

	if resp.StatusCode != 402 {
		return nil, fmt.Errorf("expected 402, got %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// findAcceptOption finds a payment option for the given network in the accepts array
func findAcceptOption(body []byte, network string) (*wallet.PaymentRequirements, error) {
	var payResp struct {
		PaymentRequirements *wallet.PaymentRequirements   `json:"payment_requirements"`
		Accepts             []*wallet.PaymentRequirements  `json:"accepts"`
	}
	if err := json.Unmarshal(body, &payResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Search accepts array for matching network
	for _, opt := range payResp.Accepts {
		if opt.Network == network {
			return opt, nil
		}
	}

	// Fallback to payment_requirements if it matches
	if payResp.PaymentRequirements != nil && payResp.PaymentRequirements.Network == network {
		return payResp.PaymentRequirements, nil
	}

	// List available networks for debugging
	var available []string
	for _, opt := range payResp.Accepts {
		available = append(available, opt.Network)
	}
	return nil, fmt.Errorf("no %s option in accepts (available: %v)", network, available)
}

// verifyWithFacilitator calls the facilitator /verify endpoint directly for debugging
func verifyWithFacilitator(facilitatorURL string, facilitatorReq *wallet.FacilitatorRequest) {
	facilitatorJSON, _ := json.MarshalIndent(facilitatorReq, "", "  ")
	fmt.Printf("Facilitator request:\n%s\n", string(facilitatorJSON))

	resp, err := http.Post(facilitatorURL+"/verify", "application/json", bytes.NewReader(facilitatorJSON))
	if err != nil {
		fmt.Printf("ERROR calling facilitator: %v\n", err)
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("Facilitator status: %d\n", resp.StatusCode)
	fmt.Printf("Facilitator response: %s\n", string(body))
}

// retryWithPayment makes the actual paid API request
func retryWithPayment(apiURL string, paymentHeader string) bool {
	reqBody := map[string]string{"text": "Hello, this is a test of x402 payments."}
	reqJSON, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", apiURL+"/v1/scan/content", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment", paymentHeader)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("ERROR making paid request: %v\n", err)
		return false
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))

	if resp.StatusCode == 200 {
		fmt.Println("\nSUCCESS! x402 payment accepted!")
		return true
	}
	fmt.Println("\nPayment rejected")
	return false
}

// runEVMTest runs the EVM (Base) E2E test flow
func runEVMTest(apiURL, privateKey string) bool {
	if len(privateKey) > 2 && privateKey[:2] == "0x" {
		privateKey = privateKey[2:]
	}

	testWallet, err := wallet.NewTestWalletFromKey(privateKey)
	if err != nil {
		fmt.Printf("ERROR creating EVM wallet: %v\n", err)
		return false
	}
	fmt.Printf("EVM Wallet: %s\n", testWallet.AddressString())

	// Step 1: Get payment requirements
	fmt.Println("\n--- Step 1: Get payment requirements ---")
	body, err := get402Response(apiURL)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return false
	}

	payReq, err := findAcceptOption(body, "base")
	if err != nil {
		// Try base-sepolia as fallback
		payReq, err = findAcceptOption(body, "base-sepolia")
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return false
		}
	}

	fmt.Printf("Payment requirements:\n")
	fmt.Printf("  Network: %s\n", payReq.Network)
	fmt.Printf("  Recipient: %s\n", payReq.Recipient)
	fmt.Printf("  Amount: %s\n", payReq.Amount)

	// Step 2: Create payment
	fmt.Println("\n--- Step 2: Create x402 payment ---")
	paymentHeader, err := testWallet.CreateX402Payment(payReq)
	if err != nil {
		fmt.Printf("ERROR creating payment: %v\n", err)
		return false
	}

	// Parse and display
	var payload wallet.X402Payload
	parts := bytes.SplitN([]byte(paymentHeader), []byte(";"), 2)
	if len(parts) == 2 {
		decoded, _ := base64.StdEncoding.DecodeString(string(parts[1]))
		json.Unmarshal(decoded, &payload)
		fmt.Printf("Payment payload:\n")
		fmt.Printf("  Payer: %s\n", payload.Payer)
		fmt.Printf("  Receiver: %s\n", payload.Receiver)
		fmt.Printf("  Amount: %s\n", payload.Amount)
		fmt.Printf("  Nonce: %s\n", payload.Nonce)
		if len(payload.Signature) > 66 {
			fmt.Printf("  Signature: %s...\n", payload.Signature[:66])
		}
	}

	// Step 2.5: Local signature verification
	fmt.Println("\n--- Step 2.5: Local VerifyPaymentSignature ---")
	if err := wallet.VerifyPaymentSignature(&payload, payload.Payer); err != nil {
		fmt.Printf("Local verification FAILED: %v\n", err)
	} else {
		fmt.Println("Local verification: PASSED")
	}

	// Step 3: Direct facilitator verify
	fmt.Println("\n--- Step 3: Direct facilitator /verify ---")
	facilitatorReq := wallet.BuildFacilitatorRequest(&payload, payReq)
	facilitatorURL := payReq.FacilitatorURL
	if facilitatorURL == "" {
		facilitatorURL = "https://x402.org/facilitator"
	}
	verifyWithFacilitator(facilitatorURL, facilitatorReq)

	// Step 4: Retry with payment header
	fmt.Println("\n--- Step 4: Retry with X-Payment header ---")
	return retryWithPayment(apiURL, paymentHeader)
}

// runSolanaTest runs the Solana E2E test flow
func runSolanaTest(apiURL, base58Key string) bool {
	testWallet, err := wallet.NewTestSolanaWalletFromKey(base58Key)
	if err != nil {
		fmt.Printf("ERROR creating Solana wallet: %v\n", err)
		return false
	}
	fmt.Printf("Solana Wallet: %s\n", testWallet.AddressString())

	// Use real RPC for E2E — fetches a real blockhash so transactions are valid for settlement.
	// Override with SOLANA_RPC_URL env var if needed (e.g., for a private RPC endpoint).
	rpcURL := os.Getenv("SOLANA_RPC_URL")
	if rpcURL == "" {
		rpcURL = wallet.SolanaDevnetRPC
	}
	testWallet.SetRPCURL(rpcURL)
	fmt.Printf("Solana RPC: %s\n", rpcURL)

	// Step 1: Get payment requirements
	fmt.Println("\n--- Step 1: Get payment requirements ---")
	body, err := get402Response(apiURL)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return false
	}

	payReq, err := findAcceptOption(body, "solana")
	if err != nil {
		// Try solana-devnet as fallback
		payReq, err = findAcceptOption(body, "solana-devnet")
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			return false
		}
	}

	fmt.Printf("Payment requirements:\n")
	fmt.Printf("  Network: %s\n", payReq.Network)
	fmt.Printf("  Recipient: %s\n", payReq.Recipient)
	fmt.Printf("  Amount: %s\n", payReq.Amount)
	if payReq.FeePayer != "" {
		fmt.Printf("  FeePayer: %s\n", payReq.FeePayer)
	}

	// Step 2: Create payment
	fmt.Println("\n--- Step 2: Create x402 Solana payment ---")
	paymentHeader, err := testWallet.CreateX402Payment(payReq)
	if err != nil {
		fmt.Printf("ERROR creating payment: %v\n", err)
		return false
	}

	// Parse and display
	var payload wallet.X402Payload
	parts := bytes.SplitN([]byte(paymentHeader), []byte(";"), 2)
	if len(parts) == 2 {
		decoded, _ := base64.StdEncoding.DecodeString(string(parts[1]))
		json.Unmarshal(decoded, &payload)
		fmt.Printf("Payment payload:\n")
		fmt.Printf("  Payer: %s\n", payload.Payer)
		fmt.Printf("  Receiver: %s\n", payload.Receiver)
		fmt.Printf("  Amount: %s\n", payload.Amount)
		fmt.Printf("  Nonce: %s\n", payload.Nonce)
		if len(payload.Transaction) > 40 {
			fmt.Printf("  Transaction: %s...\n", payload.Transaction[:40])
		}
	}

	// Step 3: Direct facilitator verify
	fmt.Println("\n--- Step 3: Direct facilitator /verify ---")
	facilitatorReq := wallet.BuildFacilitatorRequest(&payload, payReq)
	facilitatorURL := payReq.FacilitatorURL
	if facilitatorURL == "" {
		facilitatorURL = "https://x402.org/facilitator"
	}
	verifyWithFacilitator(facilitatorURL, facilitatorReq)

	// Step 4: Retry with payment header
	fmt.Println("\n--- Step 4: Retry with X-Payment header ---")
	return retryWithPayment(apiURL, paymentHeader)
}
