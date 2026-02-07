-- Initial schema for Stronghold Mullvad-style authentication
-- Migration: 001_initial_schema

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Account status enum
CREATE TYPE account_status AS ENUM ('active', 'suspended', 'closed');

-- Deposit status enum
CREATE TYPE deposit_status AS ENUM ('pending', 'completed', 'failed', 'cancelled');

-- Deposit provider enum
CREATE TYPE deposit_provider AS ENUM ('stripe', 'direct');

-- Accounts table - core account entity
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_number VARCHAR(19) UNIQUE NOT NULL, -- formatted with dashes: XXXX-XXXX-XXXX-XXXX
    wallet_address VARCHAR(42), -- Ethereum address for x402 (0x...)
    balance_usdc DECIMAL(20,6) NOT NULL DEFAULT 0.000000,
    status account_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB DEFAULT '{}'::jsonb,

    CONSTRAINT valid_account_number CHECK (account_number ~ '^[0-9]{4}-[0-9]{4}-[0-9]{4}-[0-9]{4}$'),
    CONSTRAINT valid_wallet_address CHECK (wallet_address IS NULL OR wallet_address ~ '^0x[a-fA-F0-9]{40}$'),
    CONSTRAINT accounts_balance_non_negative CHECK (balance_usdc >= 0)
);

-- Create indexes for account lookups (UNIQUE on account_number already creates an implicit index)
CREATE UNIQUE INDEX accounts_wallet_address_unique ON accounts(wallet_address) WHERE wallet_address IS NOT NULL;
CREATE INDEX idx_accounts_status ON accounts(status);

-- Sessions table - JWT refresh token storage
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    refresh_token_hash VARCHAR(64) NOT NULL, -- SHA-256 hash
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMP WITH TIME ZONE,
    ip_address INET,
    user_agent TEXT,

    CONSTRAINT valid_token_hash CHECK (LENGTH(refresh_token_hash) = 64)
);

-- Create indexes for session management
CREATE INDEX idx_sessions_account_id ON sessions(account_id);
CREATE INDEX idx_sessions_refresh_hash ON sessions(refresh_token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- Usage logs table - billing and analytics
CREATE TABLE usage_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    request_id VARCHAR(64) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,
    cost_usdc DECIMAL(20,6) NOT NULL DEFAULT 0.000000,
    status VARCHAR(50) NOT NULL,
    threat_detected BOOLEAN NOT NULL DEFAULT FALSE,
    threat_type VARCHAR(100),
    request_size_bytes INTEGER,
    response_size_bytes INTEGER,
    latency_ms INTEGER,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for usage queries
CREATE INDEX idx_usage_logs_account_id ON usage_logs(account_id);
CREATE INDEX idx_usage_logs_created_at ON usage_logs(created_at);
CREATE INDEX idx_usage_logs_account_created ON usage_logs(account_id, created_at);
CREATE INDEX idx_usage_logs_request_id ON usage_logs(request_id);

-- Deposits table - payment tracking
CREATE TABLE deposits (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider deposit_provider NOT NULL,
    amount_usdc DECIMAL(20,6) NOT NULL,
    fee_usdc DECIMAL(20,6) NOT NULL DEFAULT 0.000000,
    net_amount_usdc DECIMAL(20,6) NOT NULL, -- amount - fee
    status deposit_status NOT NULL DEFAULT 'pending',
    provider_transaction_id VARCHAR(255) UNIQUE,
    wallet_address VARCHAR(42), -- for direct deposits
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,

    CONSTRAINT valid_deposit_wallet CHECK (wallet_address IS NULL OR wallet_address ~ '^0x[a-fA-F0-9]{40}$')
);

-- Create indexes for deposit queries
CREATE INDEX idx_deposits_account_id ON deposits(account_id);
CREATE INDEX idx_deposits_status ON deposits(status);
CREATE INDEX idx_deposits_created_at ON deposits(created_at);
CREATE INDEX idx_deposits_provider_tx ON deposits(provider_transaction_id);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to auto-update updated_at on accounts
CREATE TRIGGER update_accounts_updated_at
    BEFORE UPDATE ON accounts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Function to calculate net amount on deposit insert/update
CREATE OR REPLACE FUNCTION calculate_deposit_net_amount()
RETURNS TRIGGER AS $$
BEGIN
    NEW.net_amount_usdc = NEW.amount_usdc - NEW.fee_usdc;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to auto-calculate net amount on deposits
CREATE TRIGGER calculate_deposit_net_amount_trigger
    BEFORE INSERT OR UPDATE ON deposits
    FOR EACH ROW
    EXECUTE FUNCTION calculate_deposit_net_amount();

-- Function to update account balance on completed deposit
CREATE OR REPLACE FUNCTION update_account_balance_on_deposit()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'completed' AND (OLD.status IS NULL OR OLD.status != 'completed') THEN
        UPDATE accounts
        SET balance_usdc = balance_usdc + NEW.net_amount_usdc
        WHERE id = NEW.account_id;
    END IF;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to auto-update account balance on deposit completion
CREATE TRIGGER update_account_balance_on_deposit_trigger
    AFTER INSERT OR UPDATE ON deposits
    FOR EACH ROW
    EXECUTE FUNCTION update_account_balance_on_deposit();

-- Function to update account balance on usage
CREATE OR REPLACE FUNCTION deduct_account_balance_on_usage()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE accounts
    SET balance_usdc = balance_usdc - NEW.cost_usdc
    WHERE id = NEW.account_id;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to auto-deduct account balance on usage log creation
CREATE TRIGGER deduct_account_balance_on_usage_trigger
    AFTER INSERT ON usage_logs
    FOR EACH ROW
    EXECUTE FUNCTION deduct_account_balance_on_usage();

-- Comments for documentation
COMMENT ON TABLE accounts IS 'Core account entity with Mullvad-style 16-digit account numbers';
COMMENT ON TABLE sessions IS 'JWT refresh token storage for session management';
COMMENT ON TABLE usage_logs IS 'Billing and analytics for API usage';
COMMENT ON TABLE deposits IS 'Payment tracking for account funding';
COMMENT ON COLUMN accounts.account_number IS 'Formatted as XXXX-XXXX-XXXX-XXXX';
COMMENT ON COLUMN accounts.wallet_address IS 'Ethereum address for x402 payments';
-- Processed webhook events for idempotency (H6)
CREATE TABLE processed_webhook_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    processed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_processed_webhook_events_processed_at ON processed_webhook_events(processed_at);
