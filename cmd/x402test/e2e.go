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
	privateKey := os.Getenv("TEST_PRIVATE_KEY")
	if privateKey == "" {
		fmt.Println("ERROR: TEST_PRIVATE_KEY not set")
		os.Exit(1)
	}

	if len(privateKey) > 2 && privateKey[:2] == "0x" {
		privateKey = privateKey[2:]
	}

	testWallet, err := wallet.NewTestWalletFromKey(privateKey)
	if err != nil {
		fmt.Printf("ERROR creating wallet: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wallet: %s\n", testWallet.AddressString())

	apiURL := os.Getenv("STRONGHOLD_API_URL")
	if apiURL == "" {
		apiURL = "https://api.getstronghold.xyz"
	}
	fmt.Printf("API URL: %s\n", apiURL)

	// Step 1: Make initial request to get payment requirements
	fmt.Println("\n=== Step 1: Get payment requirements ===")
	reqBody := map[string]string{"text": "Hello, this is a test of x402 payments."}
	reqJSON, _ := json.Marshal(reqBody)

	resp, err := http.Post(apiURL+"/v1/scan/content", "application/json", bytes.NewReader(reqJSON))
	if err != nil {
		fmt.Printf("ERROR making request: %v\n", err)
		os.Exit(1)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)

	if resp.StatusCode != 402 {
		fmt.Printf("Expected 402, got %d\n", resp.StatusCode)
		fmt.Printf("Response: %s\n", string(body))
		os.Exit(1)
	}

	var payResp struct {
		PaymentRequirements *wallet.PaymentRequirements `json:"payment_requirements"`
	}
	if err := json.Unmarshal(body, &payResp); err != nil {
		fmt.Printf("ERROR parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Payment requirements:\n")
	fmt.Printf("  Network: %s\n", payResp.PaymentRequirements.Network)
	fmt.Printf("  Recipient: %s\n", payResp.PaymentRequirements.Recipient)
	fmt.Printf("  Amount: %s\n", payResp.PaymentRequirements.Amount)

	// Step 2: Create x402 payment
	fmt.Println("\n=== Step 2: Create x402 payment ===")
	paymentHeader, err := testWallet.CreateX402Payment(payResp.PaymentRequirements)
	if err != nil {
		fmt.Printf("ERROR creating payment: %v\n", err)
		os.Exit(1)
	}

	// Parse and display the payment for debugging
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
		fmt.Printf("  Signature: %s\n", payload.Signature[:66]+"...")
	}

	// Step 2.5: Local signature verification
	fmt.Println("\n=== Step 2.5: Local VerifyPaymentSignature ===")
	if err := wallet.VerifyPaymentSignature(&payload, payload.Payer); err != nil {
		fmt.Printf("Local verification FAILED: %v\n", err)
	} else {
		fmt.Println("Local verification: ✅ PASSED")
	}

	// Step 3: Direct facilitator verify to debug
	fmt.Println("\n=== Step 3: Direct facilitator /verify ===")
	facilitatorReq := wallet.BuildFacilitatorRequest(&payload, payResp.PaymentRequirements)
	facilitatorJSON, _ := json.MarshalIndent(facilitatorReq, "", "  ")
	fmt.Printf("Facilitator request:\n%s\n", string(facilitatorJSON))

	facilitatorResp, err := http.Post("https://x402.org/facilitator/verify", "application/json", bytes.NewReader(facilitatorJSON))
	if err != nil {
		fmt.Printf("ERROR calling facilitator: %v\n", err)
		os.Exit(1)
	}
	facilitatorBody, _ := io.ReadAll(facilitatorResp.Body)
	facilitatorResp.Body.Close()
	fmt.Printf("Facilitator status: %d\n", facilitatorResp.StatusCode)
	fmt.Printf("Facilitator response: %s\n", string(facilitatorBody))

	// Step 4: Retry with payment header
	fmt.Println("\n=== Step 4: Retry with X-PAYMENT header ===")
	req, _ := http.NewRequest("POST", apiURL+"/v1/scan/content", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PAYMENT", paymentHeader)

	client := &http.Client{Timeout: 30 * time.Second}
	resp2, err := client.Do(req)
	if err != nil {
		fmt.Printf("ERROR making paid request: %v\n", err)
		os.Exit(1)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	fmt.Printf("Status: %d\n", resp2.StatusCode)
	fmt.Printf("Response: %s\n", string(body2))

	if resp2.StatusCode == 200 {
		fmt.Println("\n✅ SUCCESS! x402 payment accepted!")
	} else {
		fmt.Println("\n❌ Payment rejected")
	}
}
