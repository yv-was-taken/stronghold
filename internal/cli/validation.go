package cli

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// ValidationError represents a validation error with field context
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidatePrivateKeyHex validates a private key hex string.
// Returns the cleaned key (without 0x prefix) or an error.
// The key must be exactly 64 hex characters (32 bytes) after removing the optional 0x prefix.
func ValidatePrivateKeyHex(privateKey string) (string, error) {
	cleaned := strings.TrimPrefix(privateKey, "0x")

	if len(cleaned) != PrivateKeyHexLength {
		return "", &ValidationError{
			Field:   "private_key",
			Message: fmt.Sprintf("invalid length: expected %d hex characters, got %d", PrivateKeyHexLength, len(cleaned)),
		}
	}

	if _, err := hex.DecodeString(cleaned); err != nil {
		return "", &ValidationError{
			Field:   "private_key",
			Message: "invalid hex format: must contain only 0-9, a-f characters",
		}
	}

	return cleaned, nil
}
