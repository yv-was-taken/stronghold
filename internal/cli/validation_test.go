package cli

import (
	"testing"
)

func TestValidatePrivateKeyHex(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCleaned string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "valid 64 char key",
			input:       "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantCleaned: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantErr:     false,
		},
		{
			name:        "valid key with 0x prefix",
			input:       "0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantCleaned: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			wantErr:     false,
		},
		{
			name:        "mixed case hex is valid",
			input:       "AbCdEf0123456789ABCDEF0123456789abcdef0123456789ABCDEF0123456789",
			wantCleaned: "AbCdEf0123456789ABCDEF0123456789abcdef0123456789ABCDEF0123456789",
			wantErr:     false,
		},
		{
			name:    "invalid hex chars (Z)",
			input:   "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ",
			wantErr: true,
			errMsg:  "invalid hex format",
		},
		{
			name:    "special characters",
			input:   "abcdef0123456789!@#$%^&*0123456789abcdef0123456789abcdef01234567",
			wantErr: true,
			errMsg:  "invalid hex format",
		},
		{
			name:    "too short",
			input:   "abc",
			wantErr: true,
			errMsg:  "invalid length",
		},
		{
			name:    "too long (65 chars without 0x)",
			input:   "abcdef0123456789abcdef0123456789abcdef0123456789abcdef01234567890",
			wantErr: true,
			errMsg:  "invalid length",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "invalid length",
		},
		{
			name:    "only 0x prefix",
			input:   "0x",
			wantErr: true,
			errMsg:  "invalid length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned, err := ValidatePrivateKeyHex(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePrivateKeyHex(%q) expected error, got nil", tt.input)
					return
				}
				ve, ok := err.(*ValidationError)
				if !ok {
					t.Fatalf("expected ValidationError, got %T: %v", err, err)
				}
				if tt.errMsg != "" && ve.Message != tt.errMsg {
					// Check if error message contains expected substring
					if len(ve.Message) < len(tt.errMsg) || ve.Message[:len(tt.errMsg)] != tt.errMsg {
						t.Errorf("error message = %q, want prefix %q", ve.Message, tt.errMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePrivateKeyHex(%q) unexpected error: %v", tt.input, err)
					return
				}
				if cleaned != tt.wantCleaned {
					t.Errorf("ValidatePrivateKeyHex(%q) = %q, want %q", tt.input, cleaned, tt.wantCleaned)
				}
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "test_field",
		Message: "test message",
	}
	expected := "test_field: test message"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestPrivateKeyHexLengthConstant(t *testing.T) {
	if PrivateKeyHexLength != 64 {
		t.Errorf("PrivateKeyHexLength = %d, want 64", PrivateKeyHexLength)
	}
}

func TestMaxKeyFileSizeConstant(t *testing.T) {
	if MaxKeyFileSize != 1024 {
		t.Errorf("MaxKeyFileSize = %d, want 1024", MaxKeyFileSize)
	}
}

func TestDefaultBlockchainConstant(t *testing.T) {
	if DefaultBlockchain != "base" {
		t.Errorf("DefaultBlockchain = %q, want %q", DefaultBlockchain, "base")
	}
}
