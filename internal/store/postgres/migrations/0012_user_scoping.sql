-- 0012_user_scoping.sql
-- Add user_id to accounts and groups for strict multi-tenant isolation.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS user_id uuid REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts (user_id);

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS user_id uuid REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_groups_user_id ON groups (user_id);
