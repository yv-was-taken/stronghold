package db

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AccountStatus represents the status of an account
type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusClosed    AccountStatus = "closed"
)

// AccountType discriminator
const (
	AccountTypeB2C = "b2c"
	AccountTypeB2B = "b2b"
)

var (
	evmWalletAddressRegex    = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)
	solanaWalletAddressRegex = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
)

var (
	ErrEVMWalletAddressConflict    = errors.New("evm wallet address already linked to another account")
	ErrSolanaWalletAddressConflict = errors.New("solana wallet address already linked to another account")
	ErrInvalidEVMWalletAddress     = errors.New("invalid evm wallet address")
	ErrInvalidSolanaWalletAddress  = errors.New("invalid solana wallet address")
	ErrEmailAlreadyExists          = errors.New("email already registered")
)

// accountSelectColumns is the standard column list for account queries.
const accountSelectColumns = `id, account_number, account_type, evm_wallet_address, solana_wallet_address,
       balance_usdc, status, wallet_escrow_enabled, totp_enabled,
       created_at, updated_at, last_login_at, metadata,
       email, company_name, password_hash, stripe_customer_id`

// scanAccount scans a row into an Account struct matching accountSelectColumns.
func scanAccount(row interface{ Scan(dest ...any) error }) (*Account, error) {
	account := &Account{}
	var accountNumber *string // nullable for B2B accounts
	err := row.Scan(
		&account.ID, &accountNumber, &account.AccountType,
		&account.EVMWalletAddress, &account.SolanaWalletAddress,
		&account.BalanceUSDC, &account.Status,
		&account.WalletEscrow, &account.TOTPEnabled,
		&account.CreatedAt, &account.UpdatedAt, &account.LastLoginAt, &account.Metadata,
		&account.Email, &account.CompanyName, &account.PasswordHash, &account.StripeCustomerID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("account not found")
		}
		return nil, fmt.Errorf("failed to scan account: %w", err)
	}
	if accountNumber != nil {
		account.AccountNumber = *accountNumber
	}
	return account, nil
}

// Account represents a Stronghold account
type Account struct {
	ID                  uuid.UUID      `json:"id"`
	AccountNumber       string         `json:"account_number"`
	AccountType         string         `json:"account_type"`
	EVMWalletAddress    *string        `json:"evm_wallet_address,omitempty"`
	SolanaWalletAddress *string        `json:"solana_wallet_address,omitempty"`
	BalanceUSDC         usdc.MicroUSDC `json:"balance_usdc"`
	Status              AccountStatus  `json:"status"`
	WalletEscrow        bool           `json:"wallet_escrow_enabled"`
	TOTPEnabled         bool           `json:"totp_enabled"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	LastLoginAt         *time.Time     `json:"last_login_at,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	// B2B fields
	Email            *string `json:"email,omitempty"`
	CompanyName      *string `json:"company_name,omitempty"`
	PasswordHash     *string `json:"-"`
	StripeCustomerID *string `json:"stripe_customer_id,omitempty"`
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

// CreateAccount creates a new B2C account with a generated account number
func (db *DB) CreateAccount(ctx context.Context, evmWalletAddress *string, solanaWalletAddress *string) (*Account, error) {
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
		ID:                  uuid.New(),
		AccountNumber:       accountNumber,
		AccountType:         AccountTypeB2C,
		EVMWalletAddress:    evmWalletAddress,
		SolanaWalletAddress: solanaWalletAddress,
		BalanceUSDC:         0,
		Status:              AccountStatusActive,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		Metadata:            make(map[string]any),
	}

	_, err = db.pool.Exec(ctx, `
		INSERT INTO accounts (id, account_number, account_type, evm_wallet_address, solana_wallet_address, balance_usdc, status, created_at, updated_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, account.ID, account.AccountNumber, account.AccountType, account.EVMWalletAddress, account.SolanaWalletAddress,
		account.BalanceUSDC, account.Status, account.CreatedAt, account.UpdatedAt, account.Metadata)

	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}

// CreateB2BAccount creates a new B2B account with email/password auth
func (db *DB) CreateB2BAccount(ctx context.Context, email, passwordHash, companyName string) (*Account, error) {
	account := &Account{
		ID:           uuid.New(),
		AccountType:  AccountTypeB2B,
		Email:        &email,
		CompanyName:  &companyName,
		PasswordHash: &passwordHash,
		BalanceUSDC:  0,
		Status:       AccountStatusActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Metadata:     make(map[string]any),
	}

	_, err := db.pool.Exec(ctx, `
		INSERT INTO accounts (id, account_type, email, company_name, password_hash, balance_usdc, status, created_at, updated_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, account.ID, account.AccountType, account.Email, account.CompanyName, account.PasswordHash,
		account.BalanceUSDC, account.Status, account.CreatedAt, account.UpdatedAt, account.Metadata)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "email") {
				return nil, ErrEmailAlreadyExists
			}
		}
		return nil, fmt.Errorf("failed to create B2B account: %w", err)
	}

	return account, nil
}

// GetAccountByEmail retrieves a B2B account by email address
func (db *DB) GetAccountByEmail(ctx context.Context, email string) (*Account, error) {
	return scanAccount(db.QueryRow(ctx,
		`SELECT `+accountSelectColumns+` FROM accounts WHERE email = $1`, email))
}

// UpdateStripeCustomerID updates the Stripe customer ID for an account
func (db *DB) UpdateStripeCustomerID(ctx context.Context, accountID uuid.UUID, customerID string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts SET stripe_customer_id = $1, updated_at = $2 WHERE id = $3
	`, customerID, time.Now().UTC(), accountID)
	if err != nil {
		return fmt.Errorf("failed to update stripe customer ID: %w", err)
	}
	return nil
}

// GetAccountByStripeCustomerID retrieves an account by Stripe customer ID
func (db *DB) GetAccountByStripeCustomerID(ctx context.Context, customerID string) (*Account, error) {
	return scanAccount(db.QueryRow(ctx,
		`SELECT `+accountSelectColumns+` FROM accounts WHERE stripe_customer_id = $1`, customerID))
}

// DeductBalance atomically deducts from account balance. Returns true if deduction succeeded
// (balance was sufficient), false if balance was insufficient.
func (db *DB) DeductBalance(ctx context.Context, accountID uuid.UUID, amount usdc.MicroUSDC) (bool, error) {
	result, err := db.pool.Exec(ctx, `
		UPDATE accounts SET balance_usdc = balance_usdc - $1, updated_at = $2
		WHERE id = $3 AND balance_usdc >= $1
	`, amount, time.Now().UTC(), accountID)
	if err != nil {
		return false, fmt.Errorf("failed to deduct balance: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

// GetAccountByID retrieves an account by its UUID
func (db *DB) GetAccountByID(ctx context.Context, id uuid.UUID) (*Account, error) {
	return scanAccount(db.QueryRow(ctx,
		`SELECT `+accountSelectColumns+` FROM accounts WHERE id = $1`, id))
}

// GetAccountByNumber retrieves an account by its account number
func (db *DB) GetAccountByNumber(ctx context.Context, accountNumber string) (*Account, error) {
	normalized := normalizeAccountNumber(accountNumber)
	return scanAccount(db.QueryRow(ctx,
		`SELECT `+accountSelectColumns+` FROM accounts WHERE account_number = $1`, normalized))
}

// GetAccountByWalletAddress retrieves an account by wallet address, auto-detecting chain by format.
// EVM addresses start with "0x", everything else is treated as Solana.
func (db *DB) GetAccountByWalletAddress(ctx context.Context, walletAddress string) (*Account, error) {
	if strings.HasPrefix(walletAddress, "0x") {
		return db.GetAccountByEVMWallet(ctx, walletAddress)
	}
	return db.GetAccountBySolanaWallet(ctx, walletAddress)
}

// GetAccountByEVMWallet retrieves an account by its EVM wallet address
func (db *DB) GetAccountByEVMWallet(ctx context.Context, evmAddress string) (*Account, error) {
	return scanAccount(db.QueryRow(ctx,
		`SELECT `+accountSelectColumns+` FROM accounts WHERE evm_wallet_address = $1`, evmAddress))
}

// GetAccountBySolanaWallet retrieves an account by its Solana wallet address
func (db *DB) GetAccountBySolanaWallet(ctx context.Context, solanaAddress string) (*Account, error) {
	return scanAccount(db.QueryRow(ctx,
		`SELECT `+accountSelectColumns+` FROM accounts WHERE solana_wallet_address = $1`, solanaAddress))
}

// UpdateAccount updates an account's fields
func (db *DB) UpdateAccount(ctx context.Context, account *Account) error {
	account.UpdatedAt = time.Now().UTC()

	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET evm_wallet_address = $1, solana_wallet_address = $2, status = $3, updated_at = $4, metadata = $5
		WHERE id = $6
	`, account.EVMWalletAddress, account.SolanaWalletAddress, account.Status, account.UpdatedAt,
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

// LinkWallet links a wallet address to an account, auto-detecting chain by address format.
// EVM addresses start with "0x", everything else is treated as Solana.
func (db *DB) LinkWallet(ctx context.Context, accountID uuid.UUID, walletAddress string) error {
	if strings.HasPrefix(walletAddress, "0x") {
		return db.LinkEVMWallet(ctx, accountID, walletAddress)
	}
	return db.LinkSolanaWallet(ctx, accountID, walletAddress)
}

// LinkEVMWallet links an EVM wallet address to an account
func (db *DB) LinkEVMWallet(ctx context.Context, accountID uuid.UUID, evmAddress string) error {
	// Check if wallet is already linked to another account
	var existingAccountID uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM accounts WHERE evm_wallet_address = $1
	`, evmAddress).Scan(&existingAccountID)

	if err == nil && existingAccountID != accountID {
		return errors.New("wallet address already linked to another account")
	}

	_, err = db.pool.Exec(ctx, `
		UPDATE accounts
		SET evm_wallet_address = $1, updated_at = $2
		WHERE id = $3
	`, evmAddress, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to link wallet: %w", err)
	}

	return nil
}

// LinkSolanaWallet links a Solana wallet address to an account
func (db *DB) LinkSolanaWallet(ctx context.Context, accountID uuid.UUID, solanaAddress string) error {
	// Check if wallet is already linked to another account
	var existingAccountID uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM accounts WHERE solana_wallet_address = $1
	`, solanaAddress).Scan(&existingAccountID)

	if err == nil && existingAccountID != accountID {
		return errors.New("wallet address already linked to another account")
	}

	_, err = db.pool.Exec(ctx, `
		UPDATE accounts
		SET solana_wallet_address = $1, updated_at = $2
		WHERE id = $3
	`, solanaAddress, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to link wallet: %w", err)
	}

	return nil
}

// UpdateBalance updates an account's balance directly (for admin operations)
func (db *DB) UpdateBalance(ctx context.Context, accountID uuid.UUID, newBalance usdc.MicroUSDC) error {
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

// DeleteAccount permanently deletes an account by ID.
// Used only for rollback during registration when a subsequent step (e.g. Stripe customer creation) fails.
func (db *DB) DeleteAccount(ctx context.Context, accountID uuid.UUID) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
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

// UpdateWalletAddress updates the wallet address for an account, auto-detecting chain by format.
// EVM addresses start with "0x", everything else is treated as Solana.
func (db *DB) UpdateWalletAddress(ctx context.Context, accountID uuid.UUID, walletAddress string) error {
	var column string
	if strings.HasPrefix(walletAddress, "0x") {
		column = "evm_wallet_address"
	} else {
		column = "solana_wallet_address"
	}

	_, err := db.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE accounts
		SET %s = $1, updated_at = $2
		WHERE id = $3
	`, column), walletAddress, time.Now().UTC(), accountID)

	if err != nil {
		return fmt.Errorf("failed to update wallet address: %w", err)
	}

	return nil
}

// UpdateWalletAddresses updates both EVM and Solana wallet addresses for an account.
// Pass nil for either address to leave that field unchanged.
func (db *DB) UpdateWalletAddresses(ctx context.Context, accountID uuid.UUID, evmAddr *string, solanaAddr *string) error {
	if evmAddr != nil {
		normalizedEVM := strings.TrimSpace(*evmAddr)
		if normalizedEVM == "" || !evmWalletAddressRegex.MatchString(normalizedEVM) {
			return ErrInvalidEVMWalletAddress
		}
		evmAddr = &normalizedEVM
	}

	if solanaAddr != nil {
		normalizedSolana := strings.TrimSpace(*solanaAddr)
		if normalizedSolana == "" || !solanaWalletAddressRegex.MatchString(normalizedSolana) {
			return ErrInvalidSolanaWalletAddress
		}
		solanaAddr = &normalizedSolana
	}

	// Check if EVM address is already linked to another account
	if evmAddr != nil {
		var existingAccountID uuid.UUID
		err := db.QueryRow(ctx, `
			SELECT id FROM accounts WHERE evm_wallet_address = $1
		`, *evmAddr).Scan(&existingAccountID)

		if err == nil {
			if existingAccountID != accountID {
				return ErrEVMWalletAddressConflict
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to check evm wallet address ownership: %w", err)
		}
	}

	// Check if Solana address is already linked to another account
	if solanaAddr != nil {
		var existingAccountID uuid.UUID
		err := db.QueryRow(ctx, `
			SELECT id FROM accounts WHERE solana_wallet_address = $1
		`, *solanaAddr).Scan(&existingAccountID)

		if err == nil {
			if existingAccountID != accountID {
				return ErrSolanaWalletAddressConflict
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to check solana wallet address ownership: %w", err)
		}
	}

	// Build SET clauses dynamically based on which addresses are provided
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if evmAddr != nil {
		setClauses = append(setClauses, fmt.Sprintf("evm_wallet_address = $%d", argIdx))
		args = append(args, *evmAddr)
		argIdx++
	}

	if solanaAddr != nil {
		setClauses = append(setClauses, fmt.Sprintf("solana_wallet_address = $%d", argIdx))
		args = append(args, *solanaAddr)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil // Nothing to update
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now().UTC())
	argIdx++

	args = append(args, accountID)
	query := fmt.Sprintf("UPDATE accounts SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), argIdx)

	_, err := db.pool.Exec(ctx, query, args...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				if strings.Contains(pgErr.ConstraintName, "accounts_evm_wallet_address_unique") {
					return ErrEVMWalletAddressConflict
				}
				if strings.Contains(pgErr.ConstraintName, "accounts_solana_wallet_address_unique") {
					return ErrSolanaWalletAddressConflict
				}
			case "23514":
				if strings.Contains(pgErr.ConstraintName, "valid_evm_wallet_address") {
					return ErrInvalidEVMWalletAddress
				}
				if strings.Contains(pgErr.ConstraintName, "valid_solana_wallet_address") {
					return ErrInvalidSolanaWalletAddress
				}
			}
		}
		return fmt.Errorf("failed to update wallet addresses: %w", err)
	}

	return nil
}

// SetTOTPSecret stores the encrypted TOTP secret for an account.
func (db *DB) SetTOTPSecret(ctx context.Context, accountID uuid.UUID, encryptedSecret string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET totp_secret_encrypted = $1, updated_at = $2
		WHERE id = $3
	`, encryptedSecret, time.Now().UTC(), accountID)
	if err != nil {
		return fmt.Errorf("failed to store TOTP secret: %w", err)
	}
	return nil
}

// GetTOTPSecret retrieves the encrypted TOTP secret for an account.
func (db *DB) GetTOTPSecret(ctx context.Context, accountID uuid.UUID) (string, error) {
	var encrypted *string
	err := db.QueryRow(ctx, `
		SELECT totp_secret_encrypted
		FROM accounts
		WHERE id = $1
	`, accountID).Scan(&encrypted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("account not found")
		}
		return "", fmt.Errorf("failed to get TOTP secret: %w", err)
	}
	if encrypted == nil || *encrypted == "" {
		return "", errors.New("TOTP not configured")
	}
	return *encrypted, nil
}

// SetTOTPEnabled updates whether TOTP is enabled for an account.
func (db *DB) SetTOTPEnabled(ctx context.Context, accountID uuid.UUID, enabled bool) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET totp_enabled = $1, updated_at = $2
		WHERE id = $3
	`, enabled, time.Now().UTC(), accountID)
	if err != nil {
		return fmt.Errorf("failed to update TOTP status: %w", err)
	}
	return nil
}

// SetWalletEscrowEnabled updates whether server-side wallet storage is enabled for an account.
func (db *DB) SetWalletEscrowEnabled(ctx context.Context, accountID uuid.UUID, enabled bool) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE accounts
		SET wallet_escrow_enabled = $1, updated_at = $2
		WHERE id = $3
	`, enabled, time.Now().UTC(), accountID)
	if err != nil {
		return fmt.Errorf("failed to update wallet escrow status: %w", err)
	}
	return nil
}
