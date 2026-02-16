package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"stronghold/internal/wallet"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	rpcCheckTimeout       = 5 * time.Second
	rpcCongestedThreshold = 1500 * time.Millisecond
	apiHealthTimeout      = 5 * time.Second
)

type endpointHealth struct {
	Status  string
	Latency time.Duration
	Detail  string
}

type apiHealthResponse struct {
	Status string `json:"status"`
}

var (
	checkAPIHealthFunc       = checkAPIHealth
	checkBaseRPCFunc         = checkBaseRPC
	checkSolanaRPCFunc       = checkSolanaRPC
	rpcStatusFromLatencyFunc = rpcStatusFromLatency
)

// Health checks API and network RPC health.
func Health() error {
	config, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var wg sync.WaitGroup

	var apiStatus endpointHealth
	var baseStatus endpointHealth
	var solanaStatus endpointHealth

	wg.Add(3)
	go func() {
		defer wg.Done()
		apiStatus = checkAPIHealthFunc(config.API.Endpoint)
	}()
	go func() {
		defer wg.Done()
		baseStatus = checkBaseRPCFunc()
	}()
	go func() {
		defer wg.Done()
		solanaStatus = checkSolanaRPCFunc()
	}()
	wg.Wait()

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║         Stronghold Health               ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	fmt.Println("API:")
	printHealthLine("Stronghold API", config.API.Endpoint, apiStatus)
	fmt.Println()

	fmt.Println("RPC Networks:")
	printHealthLine("Base", wallet.BaseMainnetRPC, baseStatus)
	printHealthLine("Solana", wallet.SolanaMainnetRPC, solanaStatus)
	fmt.Println()
	fmt.Println("Legend:")
	fmt.Println("  up        - endpoint responded normally")
	fmt.Println("  congested - endpoint responded but latency exceeded threshold")
	fmt.Println("  down      - endpoint did not respond before timeout")
	fmt.Println()

	return nil
}

func checkAPIHealth(baseURL string) endpointHealth {
	healthURL := strings.TrimRight(baseURL, "/") + "/health"
	client := &http.Client{Timeout: apiHealthTimeout}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, healthURL, nil)
	if err != nil {
		return endpointHealth{
			Status: "down",
			Detail: err.Error(),
		}
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return endpointHealth{
			Status:  "down",
			Latency: latency,
			Detail:  err.Error(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusInternalServerError {
		return endpointHealth{
			Status:  "down",
			Latency: latency,
			Detail:  fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	var body apiHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return endpointHealth{
			Status:  "down",
			Latency: latency,
			Detail:  "invalid health response",
		}
	}

	// Map API-level degraded to "congested" for a consistent health vocabulary.
	switch body.Status {
	case "healthy", "ready", "alive":
		return endpointHealth{
			Status:  "up",
			Latency: latency,
		}
	case "degraded":
		return endpointHealth{
			Status:  "congested",
			Latency: latency,
			Detail:  "reported degraded",
		}
	default:
		return endpointHealth{
			Status:  "down",
			Latency: latency,
			Detail:  fmt.Sprintf("reported %q", body.Status),
		}
	}
}

func checkBaseRPC() endpointHealth {
	ctx, cancel := context.WithTimeout(context.Background(), rpcCheckTimeout)
	defer cancel()

	start := time.Now()
	client, err := ethclient.DialContext(ctx, wallet.BaseMainnetRPC)
	if err != nil {
		return endpointHealth{
			Status:  "down",
			Latency: time.Since(start),
			Detail:  err.Error(),
		}
	}
	defer client.Close()

	if _, err := client.BlockNumber(ctx); err != nil {
		return endpointHealth{
			Status:  "down",
			Latency: time.Since(start),
			Detail:  err.Error(),
		}
	}

	latency := time.Since(start)
	return endpointHealth{
		Status:  rpcStatusFromLatencyFunc(latency),
		Latency: latency,
	}
}

func checkSolanaRPC() endpointHealth {
	ctx, cancel := context.WithTimeout(context.Background(), rpcCheckTimeout)
	defer cancel()

	start := time.Now()
	client := rpc.New(wallet.SolanaMainnetRPC)
	if _, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized); err != nil {
		return endpointHealth{
			Status:  "down",
			Latency: time.Since(start),
			Detail:  err.Error(),
		}
	}

	latency := time.Since(start)
	return endpointHealth{
		Status:  rpcStatusFromLatencyFunc(latency),
		Latency: latency,
	}
}

func rpcStatusFromLatency(latency time.Duration) string {
	if latency > rpcCongestedThreshold {
		return "congested"
	}
	return "up"
}

func printHealthLine(name, target string, health endpointHealth) {
	status := healthStatusText(health.Status)
	fmt.Printf("  %-13s %s (%s)\n", name+":", status, target)
	if health.Latency > 0 {
		fmt.Printf("    Latency: %s\n", health.Latency.Round(time.Millisecond))
	}
	if health.Detail != "" {
		fmt.Printf("    Detail:  %s\n", health.Detail)
	}
}

func healthStatusText(status string) string {
	switch status {
	case "up":
		return successStyle.Render(status)
	case "congested":
		return warningStyle.Render(status)
	default:
		return errorStyle.Render(status)
	}
}
