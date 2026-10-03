-- 0016_account_status_check.sql
-- Enforces that accounts.status only ever holds valid enum values ('active', 'error').

-- Normalize any historical or stray 'ok' values to 'active' before adding constraint
UPDATE accounts SET status = 'active' WHERE status = 'ok';

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'check_account_status'
  ) THEN
    ALTER TABLE accounts ADD CONSTRAINT check_account_status CHECK (status IN ('active', 'error'));
  END IF;
END $$;
