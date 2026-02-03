package cli

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

// ZeroString is a utility function to zero out a string's underlying bytes.
// Note: This only works if the string was created from a mutable byte slice
// and that byte slice is still accessible. For strings created from literals
// or through other means, this may not effectively zero the memory.
// Use SecureBytes instead when possible for better guarantees.
func ZeroString(s *string) {
	if s == nil || *s == "" {
		return
	}
	// Convert string to byte slice for zeroing
	// Note: This creates a new allocation, so the original string memory
	// may not be zeroed. This is a best-effort approach.
	b := []byte(*s)
	for i := range b {
		b[i] = 0
	}
	*s = ""
}
