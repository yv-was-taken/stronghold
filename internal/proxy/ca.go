package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CA holds the certificate authority for MITM TLS interception
type CA struct {
	cert    *x509.Certificate
	key     *rsa.PrivateKey
	certPEM []byte
	keyPEM  []byte
}

// NewCA generates a new root CA certificate
func NewCA() (*CA, error) {
	// Generate 2048-bit RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Create CA certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Stronghold Security"},
			CommonName:   "Stronghold Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	// Self-sign the CA certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return &CA{
		cert:    cert,
		key:     key,
		certPEM: certPEM,
		keyPEM:  keyPEM,
	}, nil
}

// LoadCA loads a CA from PEM files
func LoadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA key: %w", err)
	}

	// Parse certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode CA certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Parse key
	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode CA key PEM")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA key: %w", err)
	}

	return &CA{
		cert:    cert,
		key:     key,
		certPEM: certPEM,
		keyPEM:  keyPEM,
	}, nil
}

// LoadOrCreateCA loads an existing CA or creates a new one
func LoadOrCreateCA(caDir string) (*CA, error) {
	certPath := filepath.Join(caDir, "ca.crt")
	keyPath := filepath.Join(caDir, "ca.key")

	// Try to load existing CA
	if _, err := os.Stat(certPath); err == nil {
		return LoadCA(certPath, keyPath)
	}

	// Create new CA
	ca, err := NewCA()
	if err != nil {
		return nil, err
	}

	// Save to disk
	if err := os.MkdirAll(caDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create CA directory: %w", err)
	}

	if err := os.WriteFile(certPath, ca.certPEM, 0644); err != nil {
		return nil, fmt.Errorf("failed to write CA certificate: %w", err)
	}

	if err := os.WriteFile(keyPath, ca.keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("failed to write CA key: %w", err)
	}

	slog.Info("Generated new CA certificate", "cert", certPath, "key", keyPath)

	return ca, nil
}

// GenerateCert creates a certificate for a specific host, signed by this CA
func (ca *CA) GenerateCert(host string) (*tls.Certificate, error) {
	// Generate key for this host
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate host key: %w", err)
	}

	// Generate serial number
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 1 year
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{host},
	}

	// Sign with our CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create host certificate: %w", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}, nil
}

// CertPEM returns the CA certificate in PEM format
func (ca *CA) CertPEM() []byte {
	return ca.certPEM
}

// Certificate returns the parsed CA certificate
func (ca *CA) Certificate() *x509.Certificate {
	return ca.cert
}
