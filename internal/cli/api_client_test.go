package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAPIClient(t *testing.T) {
	client := NewAPIClient("https://api.example.com")
	if client == nil {
		t.Fatal("NewAPIClient returned nil")
	}
	if client.baseURL != "https://api.example.com" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "https://api.example.com")
	}
	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestDoRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/test" {
			t.Errorf("Path = %s, want /test", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	var result map[string]string
	err := client.doRequest(http.MethodPost, "/test", http.StatusOK, nil, &result)
	if err != nil {
		t.Fatalf("doRequest failed: %v", err)
	}
	if result["result"] != "ok" {
		t.Errorf("result = %v, want {result: ok}", result)
	}
}

func TestDoRequest_StatusMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	err := client.doRequest(http.MethodGet, "/missing", http.StatusOK, nil, nil)
	if err == nil {
		t.Fatal("expected error for status mismatch, got nil")
	}
	// Should include method, endpoint, and status in error
	errStr := err.Error()
	if !contains(errStr, "404") && !contains(errStr, "GET") {
		t.Errorf("error should include status and method, got: %v", err)
	}
}

func TestDoRequest_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request"})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	err := client.doRequest(http.MethodPost, "/test", http.StatusOK, nil, nil)
	if err == nil {
		t.Fatal("expected error for API error response, got nil")
	}
	if !contains(err.Error(), "invalid request") {
		t.Errorf("error should include API error message, got: %v", err)
	}
}

func TestDoRequest_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	var result map[string]string
	err := client.doRequest(http.MethodGet, "/test", http.StatusOK, nil, &result)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !contains(err.Error(), "parse") {
		t.Errorf("error should mention parse failure, got: %v", err)
	}
}

func TestDoRequest_WithRequestBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["key"] != "value" {
			t.Errorf("request body key = %q, want %q", body["key"], "value")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"received": "true"})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	reqBody := map[string]string{"key": "value"}
	var result map[string]string
	err := client.doRequest(http.MethodPost, "/test", http.StatusOK, reqBody, &result)
	if err != nil {
		t.Fatalf("doRequest failed: %v", err)
	}
}

func TestDoRequest_NilResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	// Passing nil for respBody should not cause issues
	err := client.doRequest(http.MethodDelete, "/test", http.StatusNoContent, nil, nil)
	if err != nil {
		t.Fatalf("doRequest failed with nil respBody: %v", err)
	}
}

func TestCreateAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/account" {
			t.Errorf("Path = %s, want /v1/auth/account", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateAccountResponse{
			AccountNumber: "1234-5678-9012-3456",
			ExpiresAt:     "2025-12-31T23:59:59Z",
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	resp, err := client.CreateAccount(&CreateAccountRequest{})
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}
	if resp.AccountNumber != "1234-5678-9012-3456" {
		t.Errorf("AccountNumber = %q, want %q", resp.AccountNumber, "1234-5678-9012-3456")
	}
}

func TestLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/login" {
			t.Errorf("Path = %s, want /v1/auth/login", r.URL.Path)
		}
		var req LoginRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.AccountNumber != "1234-5678-9012-3456" {
			t.Errorf("AccountNumber = %q, want %q", req.AccountNumber, "1234-5678-9012-3456")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LoginResponse{
			AccountNumber: "1234-5678-9012-3456",
			ExpiresAt:     "2025-12-31T23:59:59Z",
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	resp, err := client.Login("1234-5678-9012-3456")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if resp.AccountNumber != "1234-5678-9012-3456" {
		t.Errorf("AccountNumber = %q, want %q", resp.AccountNumber, "1234-5678-9012-3456")
	}
}

func TestUpdateWallet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/wallet" {
			t.Errorf("Path = %s, want /v1/auth/wallet", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("Method = %s, want PUT", r.Method)
		}
		var req UpdateWalletRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.PrivateKey == "" {
			t.Error("PrivateKey should not be empty")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	err := client.UpdateWallet("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatalf("UpdateWallet failed: %v", err)
	}
}

func TestDoRequest_NetworkError(t *testing.T) {
	// Use an invalid URL to trigger network error
	client := NewAPIClient("http://localhost:1")
	err := client.doRequest(http.MethodGet, "/test", http.StatusOK, nil, nil)
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}
