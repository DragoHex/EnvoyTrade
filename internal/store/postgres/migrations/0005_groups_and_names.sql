-- Groups and Account names (PLAN.md §2).
-- Promotes groups to a first-class table and adds human-readable name to accounts and groups.

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS groups (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL DEFAULT '',
  master_id   uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

DO $$ BEGIN
  ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_master_id_fkey;
  ALTER TABLE groups ADD CONSTRAINT groups_master_id_fkey FOREIGN KEY (master_id) REFERENCES accounts(id) ON DELETE CASCADE;
EXCEPTION WHEN OTHERS THEN
  NULL;
END $$;

-- Backfill existing master accounts into groups table (matching group id to master account id)
INSERT INTO groups (id, name, master_id)
SELECT id, COALESCE(NULLIF(name, ''), broker_user_id), id
FROM accounts
WHERE role = 'master'
ON CONFLICT (id) DO NOTHING;

-- Modify follow_links to reference groups(id) instead of master accounts(id)
ALTER TABLE follow_links ADD COLUMN IF NOT EXISTS group_id uuid REFERENCES groups(id);

DO $$ BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'follow_links' AND column_name = 'master_id'
  ) THEN
    UPDATE follow_links f
    SET group_id = g.id
    FROM groups g
    WHERE g.master_id = f.master_id AND f.group_id IS NULL;

    ALTER TABLE follow_links DROP CONSTRAINT IF EXISTS follow_links_check;
    ALTER TABLE follow_links DROP COLUMN IF EXISTS master_id;
  END IF;
END $$;

ALTER TABLE follow_links ALTER COLUMN group_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS follow_links_group_id_idx ON follow_links (group_id);
CREATE INDEX IF NOT EXISTS groups_master_id_idx ON groups (master_id);
