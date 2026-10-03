-- 0011_users_and_sessions.sql
-- Platform users and active session tokens.

CREATE TABLE IF NOT EXISTS users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text NOT NULL UNIQUE,
    username        text NOT NULL UNIQUE,
    name            text NOT NULL DEFAULT '',
    password_hash   text NOT NULL,
    totp_secret     text,
    totp_enabled    boolean NOT NULL DEFAULT false,
    role            text NOT NULL DEFAULT 'user',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users (LOWER(email));
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username));

CREATE TABLE IF NOT EXISTS sessions (
    id              bigserial PRIMARY KEY,
    token_hash      text NOT NULL UNIQUE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip_address      text,
    user_agent      text,
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_seen_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);
