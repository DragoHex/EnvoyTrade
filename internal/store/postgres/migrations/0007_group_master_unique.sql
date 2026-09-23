-- Ensure one group per master account at DB level (groups.master_id UNIQUE DEFERRABLE).

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'groups_master_id_unique'
  ) THEN
    -- Reassign any followers of duplicate groups to the primary group for that master
    WITH ranked AS (
      SELECT id, master_id,
        ROW_NUMBER() OVER (
          PARTITION BY master_id
          ORDER BY (SELECT COUNT(*) FROM follow_links fl WHERE fl.group_id = groups.id) DESC,
                   updated_at DESC,
                   id DESC
        ) AS rn,
        FIRST_VALUE(id) OVER (
          PARTITION BY master_id
          ORDER BY (SELECT COUNT(*) FROM follow_links fl WHERE fl.group_id = groups.id) DESC,
                   updated_at DESC,
                   id DESC
        ) AS canonical_id
      FROM groups
    ),
    duplicates AS (
      SELECT id, canonical_id FROM ranked WHERE rn > 1
    )
    UPDATE follow_links fl
    SET group_id = d.canonical_id
    FROM duplicates d
    WHERE fl.group_id = d.id;

    -- Delete duplicate groups
    DELETE FROM groups
    WHERE id IN (
      SELECT id FROM (
        SELECT id,
          ROW_NUMBER() OVER (
            PARTITION BY master_id
            ORDER BY (SELECT COUNT(*) FROM follow_links fl WHERE fl.group_id = groups.id) DESC,
                     updated_at DESC,
                     id DESC
          ) AS rn
        FROM groups
      ) sub WHERE sub.rn > 1
    );

    ALTER TABLE groups ADD CONSTRAINT groups_master_id_unique UNIQUE (master_id) DEFERRABLE INITIALLY DEFERRED;
  END IF;
END $$;
