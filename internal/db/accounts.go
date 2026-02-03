package db

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AccountStatus represents the status of an account
type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusClosed    AccountStatus = "closed"
)

// Account represents a Stronghold account
type Account struct {
	ID             uuid.UUID       `json:"id"`
	AccountNumber  string          `json:"account_number"`
	WalletAddress  *string         `json:"wallet_address,omitempty"`
	BalanceUSDC    float64         `json:"balance_usdc"`
	Status         AccountStatus   `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	LastLoginAt    *time.Time      `json:"last_login_at,omitempty"`
	Metadata       map[string]any  `json:"metadata,omitempty"`
	// Encrypted wallet key fields - never exposed via JSON
	EncryptedPrivateKey *string    `json:"-"`
	KMSKeyID            *string    `json:"-"`
	KeyEncryptedAt      *time.Time `json:"-"`
}

// GenerateAccountNumber creates a cryptographically secure 16-digit account number
// formatted as XXXX-XXXX-XXXX-XXXX
func GenerateAccountNumber() (string, error) {
	// Generate 16 random digits
	var parts []string
	for i := 0; i < 4; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10000))
		if err != nil {
			return "", fmt.Errorf("failed to generate random digits: %w", err)
		}
		parts = append(parts, fmt.Sprintf("%04d", n.Int64()))
	}
	return strings.Join(parts, "-"), nil
}

// CreateAccount creates a new account with a generated account number
func (db *DB) CreateAccount(ctx context.Context, walletAddress *string) (*Account, error) {
	// Generate unique account number
	var accountNumber string
	var err error
	maxAttempts := 10

	for i := 0; i < maxAttempts; i++ {
		accountNumber, err = GenerateAccountNumber()
		if err != nil {
			return nil, err
		}

		// Check if account number already exists
		var exists bool
		err = db.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM accounts WHERE account_number = $1)",
			accountNumber,
		).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("failed to check account number existence: %w", err)
		}

		if !exists {
			break
		}

		// Try again with a new number
		if i == maxAttempts-1 {
			return nil, errors.New("failed to generate unique account number after maximum attempts")
		}
	}

	// Insert the new account
	account := &Account{
		ID:            uuid.New(),
		AccountNumber: accountNumber,
		WalletAddress: walletAddress,
		BalanceUSDC:   0,
		Status:        AccountStatusActive,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		Metadata:      make(map[string]any),
	}

	_, err = db.pool.Exec(ctx, `
		INSERT INTO accounts (id, account_number, wallet_address, balance_usdc, status, created_at, updated_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, account.ID, account.AccountNumber, account.WalletAddress, account.BalanceUSDC,
		account.Status, account.CreatedAt, account.UpdatedAt, account.Metadata)

	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}

// GetAccountByID retrieves an account by its UUID
func (db *DB) GetAccountByID(ctx context.Context, id uuid.UUID) (*Account, error) {
	account := &Account{}
	err := db.QueryRow(ctx, `
		SELECT id, account_number, wallet_address, balance_usdc, status,
		       created_at, updated_at, last_login_at, metadata
		FROM accounts
		WHERE id = $1
	`, id).Scan(
		&account.ID, &account.AccountNumber, &account.WalletAddress,
		&account.BalanceUSDC, &account.Status, &account.CreatedAt,
		&account.UpdatedAt, &account.LastLoginAt, &account.Metadata,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("account not found")
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// GetAccountByNumber retrieves an account by its account number
func (db *DB) GetAccountByNumber(ctx context.Context, accountNumber string) (*Account, error) {
	// Normalize the account number (remove any existing dashes and reformat)
	normalized := normalizeAccountNumber(accountNumber)

	account := &Account{}
	err := db.QueryRow(ctx, `
		SELECT id, account_number, wallet_address, balance_usdc, status,
		       created_at, updated_at, last_login_at, metadata
		FROM accounts
		WHERE account_number = $1
	`, normalized).Scan(
		&account.ID, &account.AccountNumber, &account.WalletAddress,
		&account.BalanceUSDC, &account.Status, &account.CreatedAt,
		&account.UpdatedAt, &account.LastLoginAt, &account.Metadata,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("account not found")
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// GetAccountByWalletAddress retrieves an account by its wallet address
func (db *DB) GetAccountByWalletAddress(ctx context.Context, walletAddress string) (*Account, error) {
	account := &Account{}
	err := db.QueryRow(ctx, `
		SELECT id, account_number, wallet_address, balance_usdc, status,
		       created_at, updated_at, last_login_at, metadata
		FROM accounts
		WHERE wallet_address = $1
	`, walletAddress).Scan(
		&account.ID, &account.AccountNumber, &account.WalletAddress,
		&account.BalanceUSDC, &account.Status, &account.CreatedAt,
		&account.UpdatedAt, &account.LastLoginAt, &account.Metadata,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("account not found")
		}
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return account, nil
}

// UpdateAccount updates an account's fields
func (db *DB) UpdateAccount(ctx context.Context, account *Account) error {
	account.UpdatedAt = time.Now().UTC()

	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET wallet_address = $1, status = $2, updated_at = $3, metadata = $4
		WHERE id = $5
	`, account.WalletAddress, account.Status, account.UpdatedAt,
		account.Metadata, account.ID)

	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp for an account
func (db *DB) UpdateLastLogin(ctx context.Context, accountID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET last_login_at = $1
		WHERE id = $2
	`, now, accountID)

	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// LinkWallet links a wallet address to an account
func (db *DB) LinkWallet(ctx context.Context, accountID uuid.UUID, walletAddress string) error {
	// Check if wallet is already linked to another account
	var existingAccountID uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM accounts WHERE wallet_address = $1
	`, walletAddress).Scan(&existingAccountID)

	if err == nil && existingAccountID != accountID {
		return errors.New("wallet address already linked to another account")
	}

	_, err = db.pool.Exec(ctx, `
		UPDATE accounts
		SET wallet_address = $1, updated_at = $2
		WHERE id = $3
	`, walletAddress, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to link wallet: %w", err)
	}

	return nil
}

// UpdateBalance updates an account's balance directly (for admin operations)
func (db *DB) UpdateBalance(ctx context.Context, accountID uuid.UUID, newBalance float64) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET balance_usdc = $1, updated_at = $2
		WHERE id = $3
	`, newBalance, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	return nil
}

// SuspendAccount suspends an account
func (db *DB) SuspendAccount(ctx context.Context, accountID uuid.UUID) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET status = $1, updated_at = $2
		WHERE id = $3
	`, AccountStatusSuspended, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to suspend account: %w", err)
	}

	return nil
}

// CloseAccount closes an account
func (db *DB) CloseAccount(ctx context.Context, accountID uuid.UUID) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET status = $1, updated_at = $2
		WHERE id = $3
	`, AccountStatusClosed, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to close account: %w", err)
	}

	return nil
}

// AccountExists checks if an account exists by account number
func (db *DB) AccountExists(ctx context.Context, accountNumber string) (bool, error) {
	normalized := normalizeAccountNumber(accountNumber)

	var exists bool
	err := db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM accounts WHERE account_number = $1)",
		normalized,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check account existence: %w", err)
	}

	return exists, nil
}

// normalizeAccountNumber normalizes an account number to the standard format
func normalizeAccountNumber(input string) string {
	// Remove all non-digit characters
	digits := strings.ReplaceAll(input, "-", "")
	digits = strings.ReplaceAll(digits, " ", "")

	// If we don't have exactly 16 digits, return as-is (will fail validation)
	if len(digits) != 16 {
		return input
	}

	// Format as XXXX-XXXX-XXXX-XXXX
	return fmt.Sprintf("%s-%s-%s-%s",
		digits[0:4], digits[4:8], digits[8:12], digits[12:16])
}

// StoreEncryptedKey stores a KMS-encrypted private key for an account
func (db *DB) StoreEncryptedKey(ctx context.Context, accountID uuid.UUID, encryptedKey, kmsKeyID string) error {
	now := time.Now().UTC()
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET encrypted_private_key = $1, kms_key_id = $2, key_encrypted_at = $3, updated_at = $4
		WHERE id = $5
	`, encryptedKey, kmsKeyID, now, now, accountID)

	if err != nil {
		return fmt.Errorf("failed to store encrypted key: %w", err)
	}

	return nil
}

// GetEncryptedKey retrieves the KMS-encrypted private key for an account
func (db *DB) GetEncryptedKey(ctx context.Context, accountID uuid.UUID) (string, error) {
	var encryptedKey *string
	err := db.QueryRow(ctx, `
		SELECT encrypted_private_key
		FROM accounts
		WHERE id = $1
	`, accountID).Scan(&encryptedKey)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("account not found")
		}
		return "", fmt.Errorf("failed to get encrypted key: %w", err)
	}

	if encryptedKey == nil {
		return "", errors.New("no encrypted key stored for this account")
	}

	return *encryptedKey, nil
}

// HasEncryptedKey checks if an account has an encrypted private key stored
func (db *DB) HasEncryptedKey(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var hasKey bool
	err := db.QueryRow(ctx, `
		SELECT encrypted_private_key IS NOT NULL
		FROM accounts
		WHERE id = $1
	`, accountID).Scan(&hasKey)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, errors.New("account not found")
		}
		return false, fmt.Errorf("failed to check encrypted key: %w", err)
	}

	return hasKey, nil
}

// UpdateWalletAddress updates the wallet address for an account
func (db *DB) UpdateWalletAddress(ctx context.Context, accountID uuid.UUID, walletAddress string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET wallet_address = $1, updated_at = $2
		WHERE id = $3
	`, walletAddress, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to update wallet address: %w", err)
	}

	return nil
}
