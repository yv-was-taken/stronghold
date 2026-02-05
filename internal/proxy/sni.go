package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// ExtractSNI reads the TLS ClientHello from a connection and extracts the SNI hostname.
// This is useful for transparent proxy on platforms where SO_ORIGINAL_DST is not available.
// The returned bufReader contains the bytes that were read plus remaining data.
func ExtractSNI(conn net.Conn, peekedData []byte) (string, []byte, error) {
	// We need to read more of the TLS handshake
	// TLS record header is 5 bytes: content_type(1) + version(2) + length(2)
	// Minimum ClientHello needs: record_header(5) + handshake_header(4) + client_version(2) + random(32) = 43 bytes

	// Start with the peeked data (should be at least 1 byte: 0x16 for TLS)
	buf := make([]byte, 0, 1024)
	buf = append(buf, peekedData...)

	// We need at least 5 bytes for the TLS record header
	for len(buf) < 5 {
		tmp := make([]byte, 5-len(buf))
		n, err := conn.Read(tmp)
		if err != nil {
			return "", buf, fmt.Errorf("failed to read TLS record header: %w", err)
		}
		buf = append(buf, tmp[:n]...)
	}

	// Verify this is a TLS handshake record (0x16)
	if buf[0] != 0x16 {
		return "", buf, fmt.Errorf("not a TLS handshake record: 0x%02x", buf[0])
	}

	// Get the record length (bytes 3-4, big endian)
	recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if recordLen > 16384 {
		return "", buf, fmt.Errorf("TLS record too large: %d", recordLen)
	}

	// Read the full record
	totalLen := 5 + recordLen
	for len(buf) < totalLen {
		toRead := totalLen - len(buf)
		if toRead > 4096 {
			toRead = 4096
		}
		tmp := make([]byte, toRead)
		n, err := conn.Read(tmp)
		if err != nil && err != io.EOF {
			return "", buf, fmt.Errorf("failed to read TLS record: %w", err)
		}
		if n == 0 {
			break
		}
		buf = append(buf, tmp[:n]...)
	}

	// Parse the ClientHello
	sni, err := parseClientHelloSNI(buf[5:])
	if err != nil {
		return "", buf, fmt.Errorf("failed to parse ClientHello: %w", err)
	}

	return sni, buf, nil
}

// parseClientHelloSNI parses a TLS ClientHello message and extracts the SNI extension
func parseClientHelloSNI(data []byte) (string, error) {
	if len(data) < 4 {
		return "", fmt.Errorf("handshake too short")
	}

	// Handshake header: type(1) + length(3)
	handshakeType := data[0]
	if handshakeType != 0x01 { // ClientHello
		return "", fmt.Errorf("not a ClientHello: 0x%02x", handshakeType)
	}

	// Skip handshake header (4 bytes)
	pos := 4

	if len(data) < pos+2+32 {
		return "", fmt.Errorf("ClientHello too short for version and random")
	}

	// Skip client version (2) and random (32)
	pos += 2 + 32

	// Session ID
	if len(data) < pos+1 {
		return "", fmt.Errorf("ClientHello too short for session ID length")
	}
	sessionIDLen := int(data[pos])
	pos++
	pos += sessionIDLen

	// Cipher suites
	if len(data) < pos+2 {
		return "", fmt.Errorf("ClientHello too short for cipher suites length")
	}
	cipherSuitesLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	pos += cipherSuitesLen

	// Compression methods
	if len(data) < pos+1 {
		return "", fmt.Errorf("ClientHello too short for compression methods length")
	}
	compressionLen := int(data[pos])
	pos++
	pos += compressionLen

	// Extensions
	if len(data) < pos+2 {
		// No extensions
		return "", fmt.Errorf("no extensions in ClientHello")
	}
	extensionsLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2

	extensionsEnd := pos + extensionsLen
	if extensionsEnd > len(data) {
		extensionsEnd = len(data)
	}

	// Parse extensions to find SNI (type 0x0000)
	for pos+4 <= extensionsEnd {
		extType := binary.BigEndian.Uint16(data[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
		pos += 4

		if pos+extLen > extensionsEnd {
			break
		}

		if extType == 0x0000 { // SNI extension
			return parseSNIExtension(data[pos : pos+extLen])
		}

		pos += extLen
	}

	return "", fmt.Errorf("SNI extension not found")
}

// parseSNIExtension parses the SNI extension data
func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 2 {
		return "", fmt.Errorf("SNI extension too short")
	}

	// Server name list length
	listLen := int(binary.BigEndian.Uint16(data[0:2]))
	if listLen+2 > len(data) {
		return "", fmt.Errorf("SNI list length mismatch")
	}

	pos := 2
	listEnd := 2 + listLen

	for pos+3 <= listEnd {
		nameType := data[pos]
		nameLen := int(binary.BigEndian.Uint16(data[pos+1 : pos+3]))
		pos += 3

		if pos+nameLen > listEnd {
			break
		}

		if nameType == 0x00 { // Host name
			return string(data[pos : pos+nameLen]), nil
		}

		pos += nameLen
	}

	return "", fmt.Errorf("hostname not found in SNI extension")
}
