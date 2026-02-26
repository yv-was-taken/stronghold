-- Migration: 005_b2b_accounts
-- Add B2B account support: account_type, WorkOS SSO auth, Stripe customer linkage,
-- API keys table, and Stripe usage records table.

-- ============================================================
-- accounts table: B2B fields
-- ============================================================

-- Account type discriminator
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT 'b2c';

-- B2B auth / profile fields
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS email TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS company_name TEXT;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS workos_user_id TEXT;

-- Stripe billing linkage
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS stripe_customer_id TEXT;

-- Make account_number nullable (B2B accounts don't have one)
-- First drop the existing NOT NULL + CHECK constraint, then re-add with conditional logic
ALTER TABLE accounts ALTER COLUMN account_number DROP NOT NULL;

-- Drop the old CHECK constraint that requires XXXX-XXXX-XXXX-XXXX format unconditionally
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS valid_account_number;

-- Add conditional CHECK: B2C requires account_number, B2B requires email
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_b2c_account_number'
    ) THEN
        ALTER TABLE accounts ADD CONSTRAINT chk_b2c_account_number
            CHECK (account_type != 'b2c' OR account_number IS NOT NULL);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_b2b_email'
    ) THEN
        ALTER TABLE accounts ADD CONSTRAINT chk_b2b_email
            CHECK (account_type != 'b2b' OR email IS NOT NULL);
    END IF;

    -- Re-add account_number format check (only when not null)
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'valid_account_number_format'
    ) THEN
        ALTER TABLE accounts ADD CONSTRAINT valid_account_number_format
            CHECK (account_number IS NULL OR account_number ~ '^[0-9]{4}-[0-9]{4}-[0-9]{4}-[0-9]{4}$');
    END IF;

    -- Valid account_type values
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'valid_account_type'
    ) THEN
        ALTER TABLE accounts ADD CONSTRAINT valid_account_type
            CHECK (account_type IN ('b2c', 'b2b'));
    END IF;
END $$;

-- Unique partial indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_email_unique
    ON accounts(email) WHERE email IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_stripe_customer_unique
    ON accounts(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_workos_user_id_unique
    ON accounts(workos_user_id) WHERE workos_user_id IS NOT NULL;

-- Index for account_type lookups
CREATE INDEX IF NOT EXISTS idx_accounts_account_type ON accounts(account_type);

-- ============================================================
-- api_keys table
-- ============================================================
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_account_id ON api_keys(account_id);

-- ============================================================
-- stripe_usage_records table
-- ============================================================
CREATE TABLE IF NOT EXISTS stripe_usage_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    usage_log_id UUID REFERENCES usage_logs(id),
    stripe_meter_event_id TEXT,
    endpoint TEXT NOT NULL,
    amount_usdc BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stripe_usage_account_created
    ON stripe_usage_records(account_id, created_at);

-- ============================================================
-- Comments
-- ============================================================
COMMENT ON COLUMN accounts.account_type IS 'Account type: b2c (personal, account number) or b2b (business, WorkOS SSO)';
COMMENT ON COLUMN accounts.email IS 'B2B login email address';
COMMENT ON COLUMN accounts.company_name IS 'B2B company/organization name';
COMMENT ON COLUMN accounts.workos_user_id IS 'WorkOS user ID for B2B SSO authentication';
COMMENT ON COLUMN accounts.stripe_customer_id IS 'Stripe Customer ID for B2B billing';
COMMENT ON TABLE api_keys IS 'API keys for B2B server-to-server authentication';
COMMENT ON TABLE stripe_usage_records IS 'Metered usage records sent to Stripe for B2B billing';
