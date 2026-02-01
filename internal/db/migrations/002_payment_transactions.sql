-- Payment transactions for atomic settlement
-- Migration: 002_payment_transactions

-- Payment status enum for state machine
CREATE TYPE payment_status AS ENUM (
    'reserved',    -- Payment verified, not yet executed
    'executing',   -- Service in progress
    'settling',    -- Settlement in progress
    'completed',   -- Settlement confirmed
    'failed',      -- Settlement failed (retryable)
    'expired'      -- Reservation expired
);

-- Payment transactions table to track atomic payment lifecycle
CREATE TABLE payment_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    payment_nonce VARCHAR(64) UNIQUE NOT NULL,
    payment_header TEXT NOT NULL,
    payer_address VARCHAR(42) NOT NULL,
    receiver_address VARCHAR(42) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    amount_usdc DECIMAL(20,6) NOT NULL,
    network VARCHAR(32) NOT NULL,
    status payment_status NOT NULL DEFAULT 'reserved',
    facilitator_payment_id VARCHAR(255),
    settlement_attempts INT DEFAULT 0,
    last_error TEXT,
    service_result JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    executed_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT valid_payer_address CHECK (payer_address ~ '^0x[a-fA-F0-9]{40}$'),
    CONSTRAINT valid_receiver_address CHECK (receiver_address ~ '^0x[a-fA-F0-9]{40}$')
);

-- Index for nonce lookups (idempotency)
CREATE INDEX idx_payment_tx_nonce ON payment_transactions(payment_nonce);

-- Index for status queries
CREATE INDEX idx_payment_tx_status ON payment_transactions(status);

-- Partial index for pending settlements (retry worker)
CREATE INDEX idx_payment_tx_pending ON payment_transactions(status, created_at)
    WHERE status IN ('executing', 'settling', 'failed');

-- Index for expiration cleanup
CREATE INDEX idx_payment_tx_expires ON payment_transactions(expires_at)
    WHERE status = 'reserved';

-- Index for payer history
CREATE INDEX idx_payment_tx_payer ON payment_transactions(payer_address, created_at);

-- Add link from usage_logs to payment transactions
ALTER TABLE usage_logs ADD COLUMN payment_transaction_id UUID
    REFERENCES payment_transactions(id);

CREATE INDEX idx_usage_logs_payment_tx ON usage_logs(payment_transaction_id);

-- Comments for documentation
COMMENT ON TABLE payment_transactions IS 'Atomic payment lifecycle tracking for x402 reserve-commit pattern';
COMMENT ON COLUMN payment_transactions.payment_nonce IS 'Unique nonce from x402 payload, used as idempotency key';
COMMENT ON COLUMN payment_transactions.payment_header IS 'Full X-Payment header for settlement retry';
COMMENT ON COLUMN payment_transactions.status IS 'State machine: reserved -> executing -> settling -> completed/failed/expired';
COMMENT ON COLUMN payment_transactions.service_result IS 'Cached scan result for idempotent replay';
COMMENT ON COLUMN payment_transactions.settlement_attempts IS 'Number of settlement retry attempts';
