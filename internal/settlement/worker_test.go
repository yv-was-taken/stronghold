package settlement

import (
	"context"
	"testing"
	"time"

	"stronghold/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorker_ExponentialBackoff(t *testing.T) {
	w := &Worker{}

	testCases := []struct {
		attempts     int
		expectedMin  time.Duration
		expectedMax  time.Duration
	}{
		{0, 5 * time.Second, 5 * time.Second},        // First attempt: 5s
		{1, 10 * time.Second, 10 * time.Second},      // Second attempt: 10s
		{2, 20 * time.Second, 20 * time.Second},      // Third attempt: 20s
		{3, 40 * time.Second, 40 * time.Second},      // Fourth attempt: 40s
		{4, 80 * time.Second, 80 * time.Second},      // Fifth attempt: 80s
		{5, 160 * time.Second, 160 * time.Second},    // Sixth attempt: 160s
		{6, 5 * time.Minute, 5 * time.Minute},        // Capped at 5 minutes
		{10, 5 * time.Minute, 5 * time.Minute},       // Still capped
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			backoff := w.calculateBackoff(tc.attempts)
			assert.GreaterOrEqual(t, backoff, tc.expectedMin)
			assert.LessOrEqual(t, backoff, tc.expectedMax)
		})
	}
}

func TestDefaultWorkerConfig(t *testing.T) {
	cfg := DefaultWorkerConfig()

	assert.Equal(t, 30*time.Second, cfg.RetryInterval)
	assert.Equal(t, 5, cfg.MaxRetryAttempts)
	assert.Equal(t, 100, cfg.BatchSize)
	assert.Equal(t, 1*time.Minute, cfg.ExpirationCheckInterval)
}

func TestNewWorker(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}

	t.Run("with default config", func(t *testing.T) {
		worker := NewWorker(nil, x402cfg, nil)
		assert.NotNil(t, worker)
		assert.NotNil(t, worker.config)
		assert.Equal(t, 30*time.Second, worker.config.RetryInterval)
	})

	t.Run("with custom config", func(t *testing.T) {
		customCfg := &WorkerConfig{
			RetryInterval:           10 * time.Second,
			MaxRetryAttempts:        3,
			BatchSize:               50,
			ExpirationCheckInterval: 30 * time.Second,
		}

		worker := NewWorker(nil, x402cfg, customCfg)
		assert.NotNil(t, worker)
		assert.Equal(t, 10*time.Second, worker.config.RetryInterval)
		assert.Equal(t, 3, worker.config.MaxRetryAttempts)
	})
}

func TestWorker_GracefulShutdown(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}

	cfg := &WorkerConfig{
		RetryInterval:           100 * time.Millisecond,
		MaxRetryAttempts:        3,
		BatchSize:               10,
		ExpirationCheckInterval: 100 * time.Millisecond,
	}

	worker := NewWorker(nil, x402cfg, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker
	worker.Start(ctx)

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop should complete within reasonable time
	done := make(chan struct{})
	go func() {
		cancel() // Cancel context
		worker.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - worker stopped gracefully
	case <-time.After(2 * time.Second):
		t.Fatal("Worker did not shut down within 2 seconds")
	}
}

func TestWorker_ContextCancellation(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}

	cfg := &WorkerConfig{
		RetryInterval:           100 * time.Millisecond,
		MaxRetryAttempts:        3,
		BatchSize:               10,
		ExpirationCheckInterval: 100 * time.Millisecond,
	}

	worker := NewWorker(nil, x402cfg, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker
	worker.Start(ctx)

	// Cancel context
	cancel()

	// Worker should stop on context cancellation
	done := make(chan struct{})
	go func() {
		worker.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Worker did not stop on context cancellation")
	}
}

// Integration tests that require real database
func TestWorker_RetriesFailedSettlements_Integration(t *testing.T) {
	t.Skip("Integration test requires database and mock facilitator")

	// This would test:
	// 1. Create a payment in failed state
	// 2. Start worker
	// 3. Mock facilitator to succeed
	// 4. Verify payment becomes completed
}

func TestWorker_ExpiresStaleReservations_Integration(t *testing.T) {
	t.Skip("Integration test requires database")

	// This would test:
	// 1. Create a payment in reserved state with expired timestamp
	// 2. Run expireStaleReservations
	// 3. Verify payment is now expired
}

func TestWorker_BackoffTiming(t *testing.T) {
	w := &Worker{}

	// Test the progression of backoff times
	backoffs := []time.Duration{}
	for i := 0; i < 10; i++ {
		backoffs = append(backoffs, w.calculateBackoff(i))
	}

	// Verify each backoff is double the previous (until cap)
	for i := 1; i < len(backoffs); i++ {
		expected := backoffs[i-1] * 2
		if expected > 5*time.Minute {
			expected = 5 * time.Minute
		}
		assert.Equal(t, expected, backoffs[i], "Backoff at attempt %d should be correct", i)
	}
}

func TestWorker_HttpClientTimeout(t *testing.T) {
	x402cfg := &config.X402Config{
		WalletAddress:  "0x1234567890123456789012345678901234567890",
		FacilitatorURL: "https://x402.org/facilitator",
		Network:        "base-sepolia",
	}

	worker := NewWorker(nil, x402cfg, nil)

	assert.Equal(t, 30*time.Second, worker.httpClient.Timeout)
}

func TestWorker_StopChannelClosed(t *testing.T) {
	x402cfg := &config.X402Config{}

	worker := NewWorker(nil, x402cfg, nil)

	// Stop without starting - should not panic
	require.NotPanics(t, func() {
		close(worker.stopCh)
	})
}

// TestWorker_RunRetryLoop_ExitsOnStop tests that the retry loop exits when stopped
func TestWorker_RunRetryLoop_ExitsOnStop(t *testing.T) {
	x402cfg := &config.X402Config{}

	cfg := &WorkerConfig{
		RetryInterval:           50 * time.Millisecond,
		MaxRetryAttempts:        3,
		BatchSize:               10,
		ExpirationCheckInterval: 50 * time.Millisecond,
	}

	worker := NewWorker(nil, x402cfg, cfg)

	ctx := context.Background()
	done := make(chan struct{})

	go func() {
		worker.runRetryLoop(ctx)
		close(done)
	}()

	// Let loop start
	time.Sleep(25 * time.Millisecond)

	// Close stop channel
	close(worker.stopCh)

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("runRetryLoop did not exit on stop")
	}
}

// TestWorker_RunExpirationLoop_ExitsOnStop tests that the expiration loop exits when stopped
func TestWorker_RunExpirationLoop_ExitsOnStop(t *testing.T) {
	x402cfg := &config.X402Config{}

	cfg := &WorkerConfig{
		RetryInterval:           50 * time.Millisecond,
		MaxRetryAttempts:        3,
		BatchSize:               10,
		ExpirationCheckInterval: 50 * time.Millisecond,
	}

	worker := NewWorker(nil, x402cfg, cfg)

	ctx := context.Background()
	done := make(chan struct{})

	go func() {
		worker.runExpirationLoop(ctx)
		close(done)
	}()

	// Let loop start
	time.Sleep(25 * time.Millisecond)

	// Close stop channel
	close(worker.stopCh)

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("runExpirationLoop did not exit on stop")
	}
}

func TestWorker_CalculateBackoff_MaxCap(t *testing.T) {
	w := &Worker{}

	// Even with very high attempt counts, should never exceed 5 minutes
	for attempts := 0; attempts < 100; attempts++ {
		backoff := w.calculateBackoff(attempts)
		assert.LessOrEqual(t, backoff, 5*time.Minute,
			"Backoff should never exceed 5 minutes, got %v for attempt %d", backoff, attempts)
	}
}
