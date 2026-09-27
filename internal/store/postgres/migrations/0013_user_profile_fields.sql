-- 0013_user_profile_fields.sql
-- Add phone, address, and gst_number to users table for account management.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS phone text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS address text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gst_number text NOT NULL DEFAULT '';
