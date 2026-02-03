package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPrivateKey_ValidFile(t *testing.T) {
	// Create a temp file with a valid 64-char hex key
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "key.txt")
	validKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	err := os.WriteFile(keyFile, []byte(validKey), 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	key, err := readPrivateKey(keyFile)
	if err != nil {
		t.Fatalf("readPrivateKey failed: %v", err)
	}
	defer key.Zero()

	if key.String() != validKey {
		t.Errorf("got key %q, want %q", key.String(), validKey)
	}
}

func TestReadPrivateKey_FileNotFound(t *testing.T) {
	_, err := readPrivateKey("/nonexistent/path/to/key.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestReadPrivateKey_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "empty.txt")
	err := os.WriteFile(keyFile, []byte(""), 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err = readPrivateKey(keyFile)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
	if !contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestReadPrivateKey_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "large.txt")
	// Create a file larger than MaxKeyFileSize (1024 bytes)
	largeData := make([]byte, MaxKeyFileSize+100)
	for i := range largeData {
		largeData[i] = 'a'
	}
	err := os.WriteFile(keyFile, largeData, 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err = readPrivateKey(keyFile)
	if err == nil {
		t.Fatal("expected error for large file, got nil")
	}
	if !contains(err.Error(), "too large") {
		t.Errorf("error should mention 'too large', got: %v", err)
	}
}

func TestReadPrivateKey_WithWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "key.txt")
	validKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	// Key with leading/trailing whitespace and newline
	err := os.WriteFile(keyFile, []byte("  "+validKey+"  \n"), 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	key, err := readPrivateKey(keyFile)
	if err != nil {
		t.Fatalf("readPrivateKey failed: %v", err)
	}
	defer key.Zero()

	if key.String() != validKey {
		t.Errorf("got key %q, want %q (whitespace should be trimmed)", key.String(), validKey)
	}
}

func TestReadPrivateKey_With0xPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "key.txt")
	validKey := "0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	err := os.WriteFile(keyFile, []byte(validKey), 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	key, err := readPrivateKey(keyFile)
	if err != nil {
		t.Fatalf("readPrivateKey failed: %v", err)
	}
	defer key.Zero()

	// Note: readPrivateKey returns the raw key; 0x prefix is stripped by ValidatePrivateKeyHex
	if key.String() != validKey {
		t.Errorf("got key %q, want %q", key.String(), validKey)
	}
}

func TestReadPrivateKey_EnvVar(t *testing.T) {
	validKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	t.Setenv("STRONGHOLD_PRIVATE_KEY", validKey)

	key, err := readPrivateKey("")
	if err != nil {
		t.Fatalf("readPrivateKey failed: %v", err)
	}
	defer key.Zero()

	if key.String() != validKey {
		t.Errorf("got key %q, want %q", key.String(), validKey)
	}
}

func TestReadPrivateKey_ZerosFileData(t *testing.T) {
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "key.txt")
	validKey := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	err := os.WriteFile(keyFile, []byte(validKey), 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	key, err := readPrivateKey(keyFile)
	if err != nil {
		t.Fatalf("readPrivateKey failed: %v", err)
	}

	// Verify key is correct before zeroing
	if key.String() != validKey {
		t.Errorf("got key %q, want %q", key.String(), validKey)
	}

	// Zero the key
	key.Zero()

	// Verify all bytes are zeroed
	for i, b := range key.Bytes() {
		if b != 0 {
			t.Errorf("byte %d not zeroed: got %d", i, b)
		}
	}
}

func TestReadPrivateKey_NoSource(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv("STRONGHOLD_PRIVATE_KEY")

	// No file, no env var, and stdin is not a pipe in tests
	_, err := readPrivateKey("")
	if err == nil {
		t.Fatal("expected error when no source provided, got nil")
	}
	if !contains(err.Error(), "no private key provided") {
		t.Errorf("error should mention 'no private key provided', got: %v", err)
	}
}

func TestReadPrivateKey_PermissionDenied(t *testing.T) {
	// Skip on Windows where permission handling is different
	if os.Getenv("GOOS") == "windows" {
		t.Skip("skipping permission test on Windows")
	}

	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "noperm.txt")
	err := os.WriteFile(keyFile, []byte("test"), 0000)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err = readPrivateKey(keyFile)
	if err == nil {
		t.Fatal("expected error for permission denied, got nil")
	}
	// Error could be "permission denied" from stat or read
	if !contains(err.Error(), "permission") {
		t.Errorf("error should mention 'permission', got: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
