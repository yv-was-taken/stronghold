-- Migration: 003_usdc_microusdc
-- Convert all USDC DECIMAL(20,6) columns to BIGINT representing microUSDC
-- (1 microUSDC = 0.000001 USDC, so $1.00 = 1000000 microUSDC)

-- ============================================================
-- accounts.balance_usdc
-- ============================================================
ALTER TABLE accounts ADD COLUMN balance_usdc_new BIGINT;
UPDATE accounts SET balance_usdc_new = (balance_usdc * 1000000)::BIGINT;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM accounts WHERE balance_usdc_new != (balance_usdc * 1000000)::BIGINT) THEN
    RAISE EXCEPTION 'Data verification failed for accounts.balance_usdc';
  END IF;
END $$;

ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_balance_non_negative;
ALTER TABLE accounts DROP COLUMN balance_usdc;
ALTER TABLE accounts RENAME COLUMN balance_usdc_new TO balance_usdc;
ALTER TABLE accounts ALTER COLUMN balance_usdc SET NOT NULL;
ALTER TABLE accounts ALTER COLUMN balance_usdc SET DEFAULT 0;
ALTER TABLE accounts ADD CONSTRAINT accounts_balance_non_negative CHECK (balance_usdc >= 0);

-- ============================================================
-- deposits.amount_usdc, fee_usdc, net_amount_usdc
-- ============================================================
ALTER TABLE deposits ADD COLUMN amount_usdc_new BIGINT;
ALTER TABLE deposits ADD COLUMN fee_usdc_new BIGINT;
ALTER TABLE deposits ADD COLUMN net_amount_usdc_new BIGINT;

UPDATE deposits SET
  amount_usdc_new = (amount_usdc * 1000000)::BIGINT,
  fee_usdc_new = (fee_usdc * 1000000)::BIGINT,
  net_amount_usdc_new = (net_amount_usdc * 1000000)::BIGINT;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM deposits WHERE amount_usdc_new != (amount_usdc * 1000000)::BIGINT) THEN
    RAISE EXCEPTION 'Data verification failed for deposits.amount_usdc';
  END IF;
  IF EXISTS (SELECT 1 FROM deposits WHERE fee_usdc_new != (fee_usdc * 1000000)::BIGINT) THEN
    RAISE EXCEPTION 'Data verification failed for deposits.fee_usdc';
  END IF;
  IF EXISTS (SELECT 1 FROM deposits WHERE net_amount_usdc_new != (net_amount_usdc * 1000000)::BIGINT) THEN
    RAISE EXCEPTION 'Data verification failed for deposits.net_amount_usdc';
  END IF;
END $$;

ALTER TABLE deposits DROP COLUMN amount_usdc;
ALTER TABLE deposits DROP COLUMN fee_usdc;
ALTER TABLE deposits DROP COLUMN net_amount_usdc;
ALTER TABLE deposits RENAME COLUMN amount_usdc_new TO amount_usdc;
ALTER TABLE deposits RENAME COLUMN fee_usdc_new TO fee_usdc;
ALTER TABLE deposits RENAME COLUMN net_amount_usdc_new TO net_amount_usdc;

ALTER TABLE deposits ALTER COLUMN amount_usdc SET NOT NULL;
ALTER TABLE deposits ALTER COLUMN fee_usdc SET NOT NULL;
ALTER TABLE deposits ALTER COLUMN fee_usdc SET DEFAULT 0;
ALTER TABLE deposits ALTER COLUMN net_amount_usdc SET NOT NULL;

-- ============================================================
-- usage_logs.cost_usdc
-- ============================================================
ALTER TABLE usage_logs ADD COLUMN cost_usdc_new BIGINT;
UPDATE usage_logs SET cost_usdc_new = (cost_usdc * 1000000)::BIGINT;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM usage_logs WHERE cost_usdc_new != (cost_usdc * 1000000)::BIGINT) THEN
    RAISE EXCEPTION 'Data verification failed for usage_logs.cost_usdc';
  END IF;
END $$;

ALTER TABLE usage_logs DROP COLUMN cost_usdc;
ALTER TABLE usage_logs RENAME COLUMN cost_usdc_new TO cost_usdc;
ALTER TABLE usage_logs ALTER COLUMN cost_usdc SET NOT NULL;
ALTER TABLE usage_logs ALTER COLUMN cost_usdc SET DEFAULT 0;

-- ============================================================
-- payment_transactions.amount_usdc
-- ============================================================
ALTER TABLE payment_transactions ADD COLUMN amount_usdc_new BIGINT;
UPDATE payment_transactions SET amount_usdc_new = (amount_usdc * 1000000)::BIGINT;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM payment_transactions WHERE amount_usdc_new != (amount_usdc * 1000000)::BIGINT) THEN
    RAISE EXCEPTION 'Data verification failed for payment_transactions.amount_usdc';
  END IF;
END $$;

ALTER TABLE payment_transactions DROP COLUMN amount_usdc;
ALTER TABLE payment_transactions RENAME COLUMN amount_usdc_new TO amount_usdc;
ALTER TABLE payment_transactions ALTER COLUMN amount_usdc SET NOT NULL;

-- ============================================================
-- Rewrite trigger functions for BIGINT arithmetic
-- ============================================================

-- Deposit net amount calculation (same logic, BIGINT types now)
CREATE OR REPLACE FUNCTION calculate_deposit_net_amount()
RETURNS TRIGGER AS $$
BEGIN
    NEW.net_amount_usdc = NEW.amount_usdc - NEW.fee_usdc;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Update account balance on completed deposit
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

-- Deduct account balance on usage
CREATE OR REPLACE FUNCTION deduct_account_balance_on_usage()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE accounts
    SET balance_usdc = balance_usdc - NEW.cost_usdc
    WHERE id = NEW.account_id;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Update column comments
COMMENT ON COLUMN accounts.balance_usdc IS 'Account balance in microUSDC (1 = 0.000001 USDC)';
COMMENT ON COLUMN deposits.amount_usdc IS 'Deposit amount in microUSDC';
COMMENT ON COLUMN deposits.fee_usdc IS 'Deposit fee in microUSDC';
COMMENT ON COLUMN deposits.net_amount_usdc IS 'Net deposit amount (amount - fee) in microUSDC';
COMMENT ON COLUMN usage_logs.cost_usdc IS 'Request cost in microUSDC';
COMMENT ON COLUMN payment_transactions.amount_usdc IS 'Payment amount in microUSDC';
