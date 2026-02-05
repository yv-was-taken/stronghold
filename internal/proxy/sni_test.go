package proxy

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestParseClientHelloSNI(t *testing.T) {
	tests := []struct {
		name        string
		data        string // hex encoded
		expectedSNI string
		expectError bool
	}{
		{
			name: "valid ClientHello with SNI",
			// Minimal ClientHello with SNI extension for "example.com"
			// handshake type (0x01) + length (3 bytes) + version + random + session_id_len + cipher_suites + compression + extensions
			data: "010000" + // ClientHello, length placeholder (will be adjusted)
				"4f" + // adjusted length (79 bytes of content)
				"0303" + // TLS 1.2
				"0102030405060708091011121314151617181920212223242526272829303132" + // 32 byte random
				"00" + // no session ID
				"0002" + "c02f" + // 2 bytes cipher suites, one cipher
				"01" + "00" + // 1 compression method
				"0018" + // extensions length (24 bytes)
				"0000" + "0010" + // SNI extension, 16 bytes
				"000e" + // SNI list length (14 bytes)
				"00" + "000b" + // host name type, 11 bytes
				hex.EncodeToString([]byte("example.com")), // "example.com"
			expectedSNI: "example.com",
			expectError: false,
		},
		{
			name: "valid ClientHello with SNI - google.com",
			data: "010000" +
				"4e" + // adjusted length
				"0303" + // TLS 1.2
				"0102030405060708091011121314151617181920212223242526272829303132" + // 32 byte random
				"00" + // no session ID
				"0002" + "c02f" + // cipher suites
				"01" + "00" + // compression
				"0017" + // extensions length (23 bytes)
				"0000" + "000f" + // SNI extension, 15 bytes
				"000d" + // SNI list length (13 bytes)
				"00" + "000a" + // host name type, 10 bytes
				hex.EncodeToString([]byte("google.com")),
			expectedSNI: "google.com",
			expectError: false,
		},
		{
			name:        "too short handshake",
			data:        "010000",
			expectedSNI: "",
			expectError: true,
		},
		{
			name:        "not a ClientHello",
			data:        "020000040303", // ServerHello type
			expectedSNI: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.data)
			if err != nil {
				t.Fatalf("failed to decode test data: %v", err)
			}

			sni, err := parseClientHelloSNI(data)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none, sni=%s", sni)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if sni != tt.expectedSNI {
					t.Errorf("expected SNI %q, got %q", tt.expectedSNI, sni)
				}
			}
		})
	}
}

func TestParseSNIExtension(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectedSNI string
		expectError bool
	}{
		{
			name: "valid SNI extension - example.com",
			data: func() []byte {
				hostname := []byte("example.com")
				// SNI list length (2 bytes) + name_type (1) + name_length (2) + hostname
				listLen := 1 + 2 + len(hostname)
				result := make([]byte, 2+listLen)
				result[0] = byte(listLen >> 8)
				result[1] = byte(listLen)
				result[2] = 0x00 // host_name type
				result[3] = byte(len(hostname) >> 8)
				result[4] = byte(len(hostname))
				copy(result[5:], hostname)
				return result
			}(),
			expectedSNI: "example.com",
			expectError: false,
		},
		{
			name:        "too short",
			data:        []byte{0x00},
			expectedSNI: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sni, err := parseSNIExtension(tt.data)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none, sni=%s", sni)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if sni != tt.expectedSNI {
					t.Errorf("expected SNI %q, got %q", tt.expectedSNI, sni)
				}
			}
		})
	}
}

// mockConn is a simple mock connection for testing
type mockConn struct {
	*bytes.Reader
}

func (m *mockConn) Write(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Close() error                        { return nil }
func (m *mockConn) LocalAddr() interface{}              { return nil }
func (m *mockConn) RemoteAddr() interface{}             { return nil }
func (m *mockConn) SetDeadline(t interface{}) error     { return nil }
func (m *mockConn) SetReadDeadline(t interface{}) error { return nil }
func (m *mockConn) SetWriteDeadline(t interface{}) error { return nil }
