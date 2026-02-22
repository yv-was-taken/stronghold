// Package settlement provides background workers for payment settlement retry and cleanup
package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"stronghold/internal/config"
	"stronghold/internal/db"
	"stronghold/internal/wallet"
)

// WorkerConfig holds configuration for the settlement worker
type WorkerConfig struct {
	// RetryInterval is how often to check for failed settlements
	RetryInterval time.Duration
	// MaxRetryAttempts is the maximum number of settlement retry attempts
	MaxRetryAttempts int
	// BatchSize is the maximum number of payments to process per retry cycle
	BatchSize int
	// ExpirationCheckInterval is how often to check for expired reservations
	ExpirationCheckInterval time.Duration
}

// DefaultWorkerConfig returns sensible defaults for the worker
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		RetryInterval:           10 * time.Second,
		MaxRetryAttempts:        5,
		BatchSize:               100,
		ExpirationCheckInterval: 1 * time.Minute,
	}
}

// Worker handles background settlement retry and reservation expiration
type Worker struct {
	db         *db.DB
	x402Config *config.X402Config
	config     *WorkerConfig
	httpClient *http.Client
	stopCh     chan struct{}
	wg         sync.WaitGroup
	rngMu      sync.Mutex
	rng        *rand.Rand
}

// NewWorker creates a new settlement worker
func NewWorker(database *db.DB, x402Config *config.X402Config, cfg *WorkerConfig) *Worker {
	if cfg == nil {
		cfg = DefaultWorkerConfig()
	}
	return &Worker{
		db:         database,
		x402Config: x402Config,
		config:     cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		stopCh: make(chan struct{}),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Start begins the background worker
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(2)

	// Settlement retry worker
	go func() {
		defer w.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("settlement retry worker panicked, restarting", "panic", r)
				time.Sleep(1 * time.Second)
				w.wg.Add(1)
				go func() {
					defer w.wg.Done()
					defer func() {
						if r := recover(); r != nil {
							slog.Error("settlement retry worker panicked again", "panic", r)
						}
					}()
					w.runRetryLoop(ctx)
				}()
			}
		}()
		w.runRetryLoop(ctx)
	}()

	// Expiration cleanup worker
	go func() {
		defer w.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("settlement expiration worker panicked, restarting", "panic", r)
				time.Sleep(1 * time.Second)
				w.wg.Add(1)
				go func() {
					defer w.wg.Done()
					defer func() {
						if r := recover(); r != nil {
							slog.Error("settlement expiration worker panicked again", "panic", r)
						}
					}()
					w.runExpirationLoop(ctx)
				}()
			}
		}()
		w.runExpirationLoop(ctx)
	}()

	slog.Info("settlement worker started")
}

// Stop gracefully stops the worker
func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	slog.Info("settlement worker stopped")
}

// runRetryLoop periodically retries failed settlements
func (w *Worker) runRetryLoop(ctx context.Context) {
	ticker := time.NewTicker(w.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.retryFailedSettlements(ctx)
		}
	}
}

// runExpirationLoop periodically expires stale reservations
func (w *Worker) runExpirationLoop(ctx context.Context) {
	ticker := time.NewTicker(w.config.ExpirationCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.expireStaleReservations(ctx)
		}
	}
}

// retryFailedSettlements processes payments that failed settlement.
// Uses optimistic per-row claiming instead of holding a long-lived transaction.
func (w *Worker) retryFailedSettlements(ctx context.Context) {
	// Query candidates without holding a transaction or row locks
	candidates, err := w.db.GetSettlementCandidates(ctx, w.config.MaxRetryAttempts, w.config.BatchSize)
	if err != nil {
		slog.Error("failed to get settlement candidates", "error", err)
		return
	}

	if len(candidates) == 0 {
		return
	}

	slog.Info("found settlement candidates", "count", len(candidates))

	for _, payment := range candidates {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		// Skip payments with nil ExecutedAt (shouldn't happen but prevents panic)
		if payment.ExecutedAt == nil {
			slog.Warn("payment has nil ExecutedAt, skipping", "id", payment.ID)
			continue
		}

		// Calculate backoff delay based on attempt number
		backoff := w.calculateBackoff(payment.SettlementAttempts)
		timeSinceExecution := time.Since(*payment.ExecutedAt)
		if timeSinceExecution < backoff {
			continue
		}

		// Claim the payment atomically (optimistic lock — another worker may beat us)
		if payment.Status == db.PaymentStatusFailed {
			// Transition failed -> settling
			if err := w.db.MarkSettling(ctx, payment.ID); err != nil {
				// Another worker claimed it, or state changed — skip
				continue
			}
		} else if payment.Status == db.PaymentStatusSettling {
			// Stuck settling payment — claim by incrementing attempts
			claimed, err := w.db.ClaimForSettlement(ctx, payment.ID)
			if err != nil {
				slog.Error("failed to claim settling payment", "payment_id", payment.ID, "error", err)
				continue
			}
			if !claimed {
				continue
			}
		}

		// Attempt settlement (HTTP call — no transaction held)
		paymentID, err := w.settlePayment(payment.PaymentHeader)
		if err != nil {
			slog.Error("settlement retry failed", "payment_id", payment.ID, "attempt", payment.SettlementAttempts+1, "error", err)
			if err := w.db.FailSettlement(ctx, payment.ID, err.Error()); err != nil {
				slog.Error("failed to record settlement failure", "payment_id", payment.ID, "error", err)
			}
			continue
		}

		// Success!
		if err := w.db.CompleteSettlement(ctx, payment.ID, paymentID); err != nil {
			slog.Error("failed to mark payment as completed", "payment_id", payment.ID, "error", err)
			continue
		}

		slog.Info("successfully settled payment on retry", "payment_id", payment.ID, "attempt", payment.SettlementAttempts+1)
	}
}

// expireStaleReservations marks old reserved payments as expired
func (w *Worker) expireStaleReservations(ctx context.Context) {
	count, err := w.db.ExpireStaleReservations(ctx)
	if err != nil {
		slog.Error("failed to expire stale reservations", "error", err)
		return
	}

	if count > 0 {
		slog.Info("expired stale payment reservations", "count", count)
	}
}

// calculateBackoff returns the backoff duration for a given attempt number.
// Uses exponential backoff with jitter to prevent thundering herd:
// Base delays: 2s, 4s, 8s, 16s, capped at 30s, plus random jitter up to 50% of delay.
func (w *Worker) calculateBackoff(attempts int) time.Duration {
	baseDelay := 2 * time.Second
	maxDelay := 30 * time.Second

	delay := baseDelay
	for i := 0; i < attempts; i++ {
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
			break
		}
	}

	// Add random jitter: 0 to 50% of the delay
	var jitter time.Duration
	if w.rng == nil {
		jitter = time.Duration(rand.Int63n(int64(delay / 2)))
	} else {
		w.rngMu.Lock()
		jitter = time.Duration(w.rng.Int63n(int64(delay / 2)))
		w.rngMu.Unlock()
	}
	return delay + jitter
}

// settlePayment attempts to settle a payment with the facilitator
func (w *Worker) settlePayment(paymentHeader string) (string, error) {
	payload, err := wallet.ParseX402Payment(paymentHeader)
	if err != nil {
		return "", fmt.Errorf("failed to parse payment: %w", err)
	}

	// Look up the wallet address for this payment's network
	recipientAddr := w.x402Config.WalletForNetwork(payload.Network)
	if recipientAddr == "" {
		return "", fmt.Errorf("no wallet configured for network: %s", payload.Network)
	}

	// Build the original payment requirements for facilitator
	originalReq := &wallet.PaymentRequirements{
		Scheme:    "x402",
		Network:   payload.Network,
		Recipient: recipientAddr,
		Amount:    payload.Amount,
		Currency:  "USDC",
	}

	// Use x402 v2 format with paymentPayload and paymentRequirements
	facilitatorReq := wallet.BuildFacilitatorRequest(payload, originalReq)

	settleBody, err := json.Marshal(facilitatorReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal settle request: %w", err)
	}

	facilitatorURL := w.x402Config.FacilitatorURL

	req, err := http.NewRequest("POST", facilitatorURL+"/settle", bytes.NewReader(settleBody))
	if err != nil {
		return "", fmt.Errorf("failed to create settle request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call facilitator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("facilitator settlement failed: %s", resp.Status)
	}

	// x402-rs SettleResponseWire does NOT use rename_all, so fields are snake_case
	var settleResult struct {
		Success     bool   `json:"success"`
		Transaction string `json:"transaction,omitempty"`
		Network     string `json:"network,omitempty"`
		Payer       string `json:"payer,omitempty"`
		ErrorReason string `json:"error_reason,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settleResult); err != nil {
		return "", fmt.Errorf("failed to decode settle response: %w", err)
	}

	if !settleResult.Success {
		reason := settleResult.ErrorReason
		if reason == "" {
			reason = "unknown"
		}
		return "", fmt.Errorf("facilitator returned success=false: %s", reason)
	}

	return settleResult.Transaction, nil
}
