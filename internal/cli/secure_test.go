package cli

import (
	"testing"
)

func TestNewSecureBytes(t *testing.T) {
	data := []byte("test data")
	sb := NewSecureBytes(data)

	if sb == nil {
		t.Fatal("NewSecureBytes returned nil")
	}
	if string(sb.Bytes()) != "test data" {
		t.Errorf("Bytes() = %q, want %q", string(sb.Bytes()), "test data")
	}
}

func TestSecureBytes_String(t *testing.T) {
	sb := NewSecureBytes([]byte("hello"))
	if sb.String() != "hello" {
		t.Errorf("String() = %q, want %q", sb.String(), "hello")
	}
}

func TestSecureBytes_Zero(t *testing.T) {
	data := []byte("sensitive data")
	sb := NewSecureBytes(data)

	// Verify data is present
	if sb.String() != "sensitive data" {
		t.Fatal("data not stored correctly")
	}

	// Zero the data
	sb.Zero()

	// Verify all bytes are zero
	for i, b := range sb.Bytes() {
		if b != 0 {
			t.Errorf("byte %d = %d, want 0", i, b)
		}
	}

	// Original data slice should also be zeroed (shares memory)
	for i, b := range data {
		if b != 0 {
			t.Errorf("original data byte %d = %d, want 0", i, b)
		}
	}
}

func TestSecureBytes_ZeroMultipleCalls(t *testing.T) {
	sb := NewSecureBytes([]byte("test"))

	// Multiple Zero() calls should be safe
	sb.Zero()
	sb.Zero()
	sb.Zero()

	// Should not panic
}

func TestSecureBytes_Len(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want int
	}{
		{"empty", []byte{}, 0},
		{"single byte", []byte{1}, 1},
		{"multiple bytes", []byte("hello"), 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := NewSecureBytes(tt.data)
			if got := sb.Len(); got != tt.want {
				t.Errorf("Len() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSecureBytes_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		sb   *SecureBytes
		want bool
	}{
		{"nil SecureBytes", nil, true},
		{"empty data", NewSecureBytes([]byte{}), true},
		{"non-empty data", NewSecureBytes([]byte("x")), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sb.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecureBytes_NilSafety(t *testing.T) {
	var sb *SecureBytes

	// All methods should be nil-safe
	if sb.Bytes() != nil {
		t.Error("Bytes() on nil should return nil")
	}
	if sb.String() != "" {
		t.Error("String() on nil should return empty string")
	}
	if sb.Len() != 0 {
		t.Error("Len() on nil should return 0")
	}
	if !sb.IsEmpty() {
		t.Error("IsEmpty() on nil should return true")
	}

	// Zero on nil should not panic
	sb.Zero()
}

func TestZeroString(t *testing.T) {
	// Use a heap-allocated string (not a literal) since ZeroString uses unsafe
	// to zero the backing memory, which would segfault on read-only literal data.
	b := []byte("sensitive-key-data")
	s := string(b)
	ZeroString(&s)
	if s != "" {
		t.Errorf("ZeroString should set string to empty, got %q", s)
	}
	// Verify the original backing bytes were zeroed
	// (string(b) makes a copy, so b is separate; but we verify s is cleared)
}

func TestZeroString_HeapString(t *testing.T) {
	// Simulate the real use case: a string from an API response (heap-allocated)
	original := []byte("0xdeadbeef1234567890abcdef")
	s := string(original)
	ZeroString(&s)
	if s != "" {
		t.Errorf("ZeroString should set string to empty, got %q", s)
	}
}

func TestZeroString_NilSafe(t *testing.T) {
	// Should not panic on nil
	ZeroString(nil)

	// Should not panic on empty string pointer
	empty := ""
	ZeroString(&empty)
}
