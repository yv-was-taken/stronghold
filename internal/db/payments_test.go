package db

import (
	"context"
	"testing"
	"time"

	"stronghold/internal/usdc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPaymentTransactionStatusFlow tests the state machine transitions
func TestPaymentTransactionStatusFlow(t *testing.T) {
	// Skip if no database connection available
	pool := getTestPool(t)
	if pool == nil {
		t.Skip("No database connection available")
	}
	db := &DB{pool: pool}
	ctx := context.Background()

	// Create a payment transaction
	tx := &PaymentTransaction{
		PaymentNonce:    "test-nonce-" + uuid.New().String(),
		PaymentHeader:   "x402;test-header",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        "/v1/scan/content",
		AmountUSDC:      usdc.MicroUSDC(1000),
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	// Test: Create payment (should be in reserved state)
	err := db.CreatePaymentTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("Failed to create payment transaction: %v", err)
	}
	if tx.Status != PaymentStatusReserved {
		t.Errorf("Expected status %s, got %s", PaymentStatusReserved, tx.Status)
	}

	// Test: Transition to executing
	err = db.TransitionStatus(ctx, tx.ID, PaymentStatusReserved, PaymentStatusExecuting)
	if err != nil {
		t.Fatalf("Failed to transition to executing: %v", err)
	}

	// Test: Verify status via GetPaymentByNonce
	fetched, err := db.GetPaymentByNonce(ctx, tx.PaymentNonce)
	if err != nil {
		t.Fatalf("Failed to get payment by nonce: %v", err)
	}
	if fetched.Status != PaymentStatusExecuting {
		t.Errorf("Expected status %s, got %s", PaymentStatusExecuting, fetched.Status)
	}

	// Test: Record execution result
	result := map[string]interface{}{
		"decision": "allow",
		"scores": map[string]float64{
			"heuristic": 0.1,
			"ml":        0.2,
		},
	}
	err = db.RecordExecution(ctx, tx.ID, result)
	if err != nil {
		t.Fatalf("Failed to record execution: %v", err)
	}

	// Verify service_result stored but status remains executing
	// (RecordExecution no longer transitions status — the middleware owns that)
	fetched, err = db.GetPaymentByID(ctx, tx.ID)
	if err != nil {
		t.Fatalf("Failed to get payment by ID: %v", err)
	}
	if fetched.Status != PaymentStatusExecuting {
		t.Errorf("Expected status %s, got %s", PaymentStatusExecuting, fetched.Status)
	}
	if fetched.ServiceResult == nil {
		t.Error("Expected service result to be stored")
	}
	if fetched.ExecutedAt == nil {
		t.Error("Expected executed_at to be set")
	}

	// Transition to settling (normally done by middleware after handler returns)
	err = db.TransitionStatus(ctx, tx.ID, PaymentStatusExecuting, PaymentStatusSettling)
	if err != nil {
		t.Fatalf("Failed to transition to settling: %v", err)
	}

	// Test: Complete settlement
	err = db.CompleteSettlement(ctx, tx.ID, "payment-id-123")
	if err != nil {
		t.Fatalf("Failed to complete settlement: %v", err)
	}

	// Verify final state
	fetched, err = db.GetPaymentByID(ctx, tx.ID)
	if err != nil {
		t.Fatalf("Failed to get payment by ID: %v", err)
	}
	if fetched.Status != PaymentStatusCompleted {
		t.Errorf("Expected status %s, got %s", PaymentStatusCompleted, fetched.Status)
	}
	if fetched.FacilitatorPaymentID == nil || *fetched.FacilitatorPaymentID != "payment-id-123" {
		t.Error("Expected facilitator payment ID to be set")
	}
	if fetched.SettledAt == nil {
		t.Error("Expected settled_at to be set")
	}

	// Cleanup
	_, _ = db.pool.Exec(ctx, "DELETE FROM payment_transactions WHERE id = $1", tx.ID)
}

// TestPaymentTransactionFailedSettlement tests the failure and retry flow
func TestPaymentTransactionFailedSettlement(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		t.Skip("No database connection available")
	}
	db := &DB{pool: pool}
	ctx := context.Background()

	// Create and transition to settling
	tx := &PaymentTransaction{
		PaymentNonce:    "test-fail-nonce-" + uuid.New().String(),
		PaymentHeader:   "x402;test-header",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        "/v1/scan/content",
		AmountUSDC:      usdc.MicroUSDC(1000),
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	err := db.CreatePaymentTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	_ = db.TransitionStatus(ctx, tx.ID, PaymentStatusReserved, PaymentStatusExecuting)
	_ = db.TransitionStatus(ctx, tx.ID, PaymentStatusExecuting, PaymentStatusSettling)

	// Test: Fail settlement
	err = db.FailSettlement(ctx, tx.ID, "facilitator unavailable")
	if err != nil {
		t.Fatalf("Failed to fail settlement: %v", err)
	}

	fetched, _ := db.GetPaymentByID(ctx, tx.ID)
	if fetched.Status != PaymentStatusFailed {
		t.Errorf("Expected status %s, got %s", PaymentStatusFailed, fetched.Status)
	}
	if fetched.SettlementAttempts != 1 {
		t.Errorf("Expected 1 settlement attempt, got %d", fetched.SettlementAttempts)
	}
	if fetched.LastError == nil || *fetched.LastError != "facilitator unavailable" {
		t.Error("Expected error message to be stored")
	}

	// Test: Retry (transition back to settling)
	err = db.MarkSettling(ctx, tx.ID)
	if err != nil {
		t.Fatalf("Failed to mark settling: %v", err)
	}

	fetched, _ = db.GetPaymentByID(ctx, tx.ID)
	if fetched.Status != PaymentStatusSettling {
		t.Errorf("Expected status %s, got %s", PaymentStatusSettling, fetched.Status)
	}

	// Test: Complete on retry
	err = db.CompleteSettlement(ctx, tx.ID, "payment-id-456")
	if err != nil {
		t.Fatalf("Failed to complete settlement on retry: %v", err)
	}

	fetched, _ = db.GetPaymentByID(ctx, tx.ID)
	if fetched.Status != PaymentStatusCompleted {
		t.Errorf("Expected status %s, got %s", PaymentStatusCompleted, fetched.Status)
	}

	// Cleanup
	_, _ = db.pool.Exec(ctx, "DELETE FROM payment_transactions WHERE id = $1", tx.ID)
}

// TestPaymentIdempotency tests that duplicate nonces are rejected
func TestPaymentIdempotency(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		t.Skip("No database connection available")
	}
	db := &DB{pool: pool}
	ctx := context.Background()

	nonce := "idempotent-nonce-" + uuid.New().String()

	tx1 := &PaymentTransaction{
		PaymentNonce:    nonce,
		PaymentHeader:   "x402;test-header-1",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        "/v1/scan/content",
		AmountUSDC:      usdc.MicroUSDC(1000),
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	err := db.CreatePaymentTransaction(ctx, tx1)
	if err != nil {
		t.Fatalf("Failed to create first payment: %v", err)
	}

	// Try to create with same nonce - should fail
	tx2 := &PaymentTransaction{
		PaymentNonce:    nonce, // Same nonce
		PaymentHeader:   "x402;test-header-2",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        "/v1/scan/content",
		AmountUSDC:      usdc.MicroUSDC(1000),
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	err = db.CreatePaymentTransaction(ctx, tx2)
	if err == nil {
		t.Error("Expected duplicate nonce to fail")
	}

	// Cleanup
	_, _ = db.pool.Exec(ctx, "DELETE FROM payment_transactions WHERE payment_nonce = $1", nonce)
}

// TestExpireStaleReservations tests the expiration cleanup
func TestExpireStaleReservations(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		t.Skip("No database connection available")
	}
	db := &DB{pool: pool}
	ctx := context.Background()

	// Create an already-expired reservation
	tx := &PaymentTransaction{
		PaymentNonce:    "expired-nonce-" + uuid.New().String(),
		PaymentHeader:   "x402;test-header",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        "/v1/scan/content",
		AmountUSDC:      usdc.MicroUSDC(1000),
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(-1 * time.Minute), // Already expired
	}

	err := db.CreatePaymentTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	// Run expiration
	count, err := db.ExpireStaleReservations(ctx)
	if err != nil {
		t.Fatalf("Failed to expire stale reservations: %v", err)
	}
	if count < 1 {
		t.Error("Expected at least 1 expired reservation")
	}

	// Verify status
	fetched, _ := db.GetPaymentByID(ctx, tx.ID)
	if fetched.Status != PaymentStatusExpired {
		t.Errorf("Expected status %s, got %s", PaymentStatusExpired, fetched.Status)
	}

	// Cleanup
	_, _ = db.pool.Exec(ctx, "DELETE FROM payment_transactions WHERE id = $1", tx.ID)
}

// TestInvalidStatusTransition tests that invalid transitions fail
func TestInvalidStatusTransition(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		t.Skip("No database connection available")
	}
	db := &DB{pool: pool}
	ctx := context.Background()

	tx := &PaymentTransaction{
		PaymentNonce:    "invalid-transition-" + uuid.New().String(),
		PaymentHeader:   "x402;test-header",
		PayerAddress:    "0x1234567890123456789012345678901234567890",
		ReceiverAddress: "0x0987654321098765432109876543210987654321",
		Endpoint:        "/v1/scan/content",
		AmountUSDC:      usdc.MicroUSDC(1000),
		Network:         "base-sepolia",
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	err := db.CreatePaymentTransaction(ctx, tx)
	if err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	// Try invalid transition: reserved -> completed (should skip executing/settling)
	err = db.TransitionStatus(ctx, tx.ID, PaymentStatusReserved, PaymentStatusCompleted)
	// This should "succeed" at the DB level but conceptually is wrong
	// Our state machine is enforced by the sequence of calls, not the DB

	// Try transition from wrong state
	err = db.TransitionStatus(ctx, tx.ID, PaymentStatusExecuting, PaymentStatusSettling)
	if err == nil {
		t.Error("Expected transition from wrong state to fail")
	}

	// Cleanup
	_, _ = db.pool.Exec(ctx, "DELETE FROM payment_transactions WHERE id = $1", tx.ID)
}

// getTestPool returns a connection pool for testing, or nil if unavailable
func getTestPool(t *testing.T) *pgxpool.Pool {
	cfg := LoadConfig()
	if cfg.Password == "" {
		return nil
	}

	db, err := New(cfg)
	if err != nil {
		t.Logf("Could not connect to database: %v", err)
		return nil
	}

	return db.pool
}
