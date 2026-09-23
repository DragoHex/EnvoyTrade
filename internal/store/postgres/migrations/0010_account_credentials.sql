-- Store encrypted credentials and live access token for automated Kite authentication.

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS encrypted_password text NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS encrypted_totp_secret text NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS access_token text NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS token_expires_at timestamptz;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS auth_status text NOT NULL DEFAULT 'unauthenticated';
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS auth_error text NOT NULL DEFAULT '';
