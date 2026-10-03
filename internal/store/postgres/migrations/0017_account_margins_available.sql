ALTER TABLE account_margins ADD COLUMN IF NOT EXISTS available_cash numeric(18,4) NOT NULL DEFAULT 0;
ALTER TABLE account_margins ADD COLUMN IF NOT EXISTS available_margin numeric(18,4) NOT NULL DEFAULT 0;
