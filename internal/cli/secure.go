package cli

import (
	"unsafe"
)

// SecureBytes wraps sensitive byte data with explicit zeroing capability.
// This type ensures that sensitive data like private keys can be securely
// cleared from memory when no longer needed.
type SecureBytes struct {
	data []byte
}

// NewSecureBytes creates a new SecureBytes wrapper around the given data.
// The caller should call Zero() when the data is no longer needed.
func NewSecureBytes(data []byte) *SecureBytes {
	return &SecureBytes{data: data}
}

// Bytes returns the underlying byte slice.
// The returned slice shares memory with the SecureBytes, so zeroing
// the SecureBytes will also zero this slice.
func (s *SecureBytes) Bytes() []byte {
	if s == nil {
		return nil
	}
	return s.data
}

// String returns the data as a string.
// Note: This creates a copy of the data as a string.
func (s *SecureBytes) String() string {
	if s == nil || s.data == nil {
		return ""
	}
	return string(s.data)
}

// Zero securely clears all bytes in the underlying slice.
// This should be called when the sensitive data is no longer needed.
// It is safe to call Zero() multiple times.
func (s *SecureBytes) Zero() {
	if s == nil || s.data == nil {
		return
	}
	for i := range s.data {
		s.data[i] = 0
	}
}

// Len returns the length of the underlying data.
func (s *SecureBytes) Len() int {
	if s == nil || s.data == nil {
		return 0
	}
	return len(s.data)
}

// IsEmpty returns true if the SecureBytes is nil or has no data.
func (s *SecureBytes) IsEmpty() bool {
	return s == nil || len(s.data) == 0
}

// ZeroString zeros the backing memory of a string in place using unsafe.StringData.
// Go strings are normally immutable, so we use unsafe to access the underlying
// byte array directly. This is necessary for security-sensitive data like private
// keys where we need to ensure the plaintext is cleared from memory.
//
// IMPORTANT: This only works for heap-allocated strings (e.g., from API responses,
// string([]byte{...}), fmt.Sprintf). Passing a string literal may cause a fault
// because literals reside in read-only memory. All callers in this codebase pass
// heap-allocated strings from API responses, so this is safe.
// Use SecureBytes instead when possible for better guarantees.
func ZeroString(s *string) {
	if s == nil || len(*s) == 0 {
		return
	}
	// unsafe.StringData returns a pointer to the string's underlying bytes.
	// unsafe.Slice converts it to a mutable byte slice so we can zero each byte.
	p := unsafe.StringData(*s)
	b := unsafe.Slice(p, len(*s))
	for i := range b {
		b[i] = 0
	}
	*s = ""
}
