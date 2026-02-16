package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = orig

	data, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("failed to read captured stdout: %v", readErr)
	}

	return string(data), runErr
}

func TestRPCStatusFromLatency(t *testing.T) {
	if got := rpcStatusFromLatency(500 * time.Millisecond); got != "up" {
		t.Fatalf("expected up for low latency, got %q", got)
	}
	if got := rpcStatusFromLatency(2 * time.Second); got != "congested" {
		t.Fatalf("expected congested for high latency, got %q", got)
	}
}

func TestCheckAPIHealth_Mappings(t *testing.T) {
	t.Run("healthy maps to up", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(apiHealthResponse{Status: "healthy"})
		}))
		defer server.Close()

		res := checkAPIHealth(server.URL)
		if res.Status != "up" {
			t.Fatalf("expected up, got %q", res.Status)
		}
	})

	t.Run("degraded maps to congested", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(apiHealthResponse{Status: "degraded"})
		}))
		defer server.Close()

		res := checkAPIHealth(server.URL)
		if res.Status != "congested" {
			t.Fatalf("expected congested, got %q", res.Status)
		}
	})

	t.Run("5xx maps to down", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		res := checkAPIHealth(server.URL)
		if res.Status != "down" {
			t.Fatalf("expected down, got %q", res.Status)
		}
	})
}

func TestHealth_OutputIncludesRPCStatuses(t *testing.T) {
	origAPI := checkAPIHealthFunc
	origBase := checkBaseRPCFunc
	origSol := checkSolanaRPCFunc
	defer func() {
		checkAPIHealthFunc = origAPI
		checkBaseRPCFunc = origBase
		checkSolanaRPCFunc = origSol
	}()

	checkAPIHealthFunc = func(string) endpointHealth {
		return endpointHealth{Status: "up", Latency: 10 * time.Millisecond}
	}
	checkBaseRPCFunc = func() endpointHealth {
		return endpointHealth{Status: "congested", Latency: 2 * time.Second, Detail: "high latency"}
	}
	checkSolanaRPCFunc = func() endpointHealth {
		return endpointHealth{Status: "down", Latency: 5 * time.Second, Detail: "timeout"}
	}

	out, err := captureStdout(t, Health)
	if err != nil {
		t.Fatalf("Health() returned error: %v", err)
	}

	for _, want := range []string{
		"Stronghold Health",
		"API:",
		"RPC Networks:",
		"Base:",
		"Solana:",
		"congested",
		"down",
		"Legend:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("health output missing %q:\n%s", want, out)
		}
	}
}
