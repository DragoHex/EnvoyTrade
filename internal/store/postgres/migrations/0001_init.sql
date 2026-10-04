-- 0001_init.sql
-- Unified Baseline Schema for EnvoyTrade
-- Consolidates all tables, constraints, enums, and indexes in topological dependency order.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1. Enums
DO $$ BEGIN
  CREATE TYPE account_role AS ENUM ('master', 'follower');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- 2. Multi-Tenant Users and Active Session Tokens
CREATE TABLE IF NOT EXISTS users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text NOT NULL UNIQUE,
    username        text NOT NULL UNIQUE,
    name            text NOT NULL DEFAULT '',
    password_hash   text NOT NULL,
    totp_secret     text,
    totp_enabled    boolean NOT NULL DEFAULT false,
    role            text NOT NULL DEFAULT 'user',
    phone           text NOT NULL DEFAULT '',
    address         text NOT NULL DEFAULT '',
    gst_number      text NOT NULL DEFAULT '',
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

-- 3. Proxy IP Pool & Static Egress Routing
CREATE TABLE IF NOT EXISTS proxy_ips (
    ip_address  text PRIMARY KEY,
    ip_type     text NOT NULL CHECK (ip_type IN ('ipv4', 'ipv6')),
    host        text NOT NULL,
    port        integer NOT NULL DEFAULT 443,
    username    text NOT NULL,
    password    text NOT NULL,
    valid_from  timestamptz NOT NULL,
    valid_until timestamptz NOT NULL,
    plan        text NOT NULL DEFAULT 'QUARTERLY',
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- 4. Broker Accounts (Masters and Followers)
CREATE TABLE IF NOT EXISTS accounts (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               uuid REFERENCES users(id) ON DELETE CASCADE,
    role                  account_role NOT NULL,
    broker                text NOT NULL DEFAULT 'zerodha',
    broker_user_id        text NOT NULL,
    name                  text NOT NULL DEFAULT '',
    api_key               text NOT NULL DEFAULT '',
    api_secret            text NOT NULL DEFAULT '',
    ip_address            text NOT NULL DEFAULT '',
    status                text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'error')),
    active                boolean NOT NULL DEFAULT true,
    encrypted_password    text NOT NULL DEFAULT '',
    encrypted_totp_secret text NOT NULL DEFAULT '',
    access_token          text NOT NULL DEFAULT '',
    token_expires_at      timestamptz,
    auth_status           text NOT NULL DEFAULT 'unauthenticated',
    auth_error            text NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (broker, broker_user_id)
);

CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_ip_address_unique
ON accounts (ip_address)
WHERE ip_address IS NOT NULL AND ip_address != '';

-- 5. Trading Groups (Strictly 1 Master Account per Group)
CREATE TABLE IF NOT EXISTS groups (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
    name        text NOT NULL DEFAULT '',
    master_id   uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT groups_master_id_unique UNIQUE (master_id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX IF NOT EXISTS idx_groups_user_id ON groups (user_id);
CREATE INDEX IF NOT EXISTS groups_master_id_idx ON groups (master_id);

-- 6. Follow Links (1 Follower assigned to 1 Group)
CREATE TABLE IF NOT EXISTS follow_links (
    follower_id       uuid PRIMARY KEY REFERENCES accounts(id),
    group_id          uuid NOT NULL REFERENCES groups(id),
    capital_ratio     numeric(10,6) NOT NULL CHECK (capital_ratio > 0),
    max_qty_per_order integer,
    enabled           boolean NOT NULL DEFAULT true,
    effective_from    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS follow_links_group_id_idx ON follow_links (group_id);

-- 7. Master Execution Signals & Follower Orders
CREATE TABLE IF NOT EXISTS master_fills (
    id               bigserial PRIMARY KEY,
    master_id        uuid NOT NULL REFERENCES accounts(id),
    broker_order_id  text NOT NULL,
    exchange         text NOT NULL,
    tradingsymbol    text NOT NULL,
    instrument_token bigint NOT NULL,
    transaction_type text NOT NULL,
    product          text NOT NULL,
    order_type       text NOT NULL,
    filled_quantity  integer NOT NULL,
    average_price    numeric(18,4) NOT NULL,
    status           text NOT NULL,
    order_timestamp  timestamptz NOT NULL,
    raw_payload      jsonb NOT NULL,
    received_at      timestamptz NOT NULL DEFAULT now(),
    dispatch_state   text NOT NULL DEFAULT 'pending',
    dispatched_at    timestamptz,
    UNIQUE (master_id, broker_order_id, filled_quantity, status)
);

CREATE INDEX IF NOT EXISTS master_fills_master_id_idx ON master_fills (master_id, order_timestamp DESC);

CREATE TABLE IF NOT EXISTS follower_orders (
    id              bigserial PRIMARY KEY,
    master_fill_id  bigint NOT NULL REFERENCES master_fills(id),
    follower_id     uuid NOT NULL REFERENCES accounts(id),
    idempotency_tag text NOT NULL UNIQUE,
    intended_qty    integer NOT NULL,
    lot_size        integer NOT NULL,
    sizing_reason   integer NOT NULL,
    placed_qty      integer,
    broker_order_id text,
    terminal_status text,
    filled_qty      integer NOT NULL DEFAULT 0,
    average_price   numeric(18,4),
    attempt_count   integer NOT NULL DEFAULT 0,
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (master_fill_id, follower_id)
);

CREATE INDEX IF NOT EXISTS follower_orders_follower_id_idx ON follower_orders (follower_id, created_at DESC);

-- 8. Audit Event Trail
CREATE TABLE IF NOT EXISTS order_events (
    id                bigserial PRIMARY KEY,
    follower_order_id bigint REFERENCES follower_orders(id),
    master_fill_id    bigint REFERENCES master_fills(id),
    account_id        uuid NOT NULL REFERENCES accounts(id),
    event_type        text NOT NULL,
    from_status       text,
    to_status         text,
    http_status       integer,
    latency_ms        integer,
    payload           jsonb NOT NULL,
    occurred_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS order_events_account_id_occurred_at_idx ON order_events (account_id, occurred_at);
CREATE INDEX IF NOT EXISTS order_events_follower_order_id_id_idx ON order_events (follower_order_id, id);

-- 9. Instrument Master Cache
CREATE TABLE IF NOT EXISTS instruments (
    instrument_token bigint PRIMARY KEY,
    exchange         text NOT NULL,
    tradingsymbol    text NOT NULL,
    lot_size         integer NOT NULL,
    tick_size        numeric(10,4) NOT NULL,
    segment          text,
    expiry           date,
    refreshed_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (exchange, tradingsymbol)
);

-- 10. Portfolio: Positions, Holdings, and Margins
CREATE TABLE IF NOT EXISTS account_positions (
    id              bigserial PRIMARY KEY,
    account_id      uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    product         text NOT NULL,
    instrument      text NOT NULL,
    quantity        integer NOT NULL,
    buy_price       numeric(18,4) NOT NULL DEFAULT 0,
    sell_price      numeric(18,4) NOT NULL DEFAULT 0,
    buy_quantity    integer NOT NULL DEFAULT 0,
    sell_quantity   integer NOT NULL DEFAULT 0,
    ltp             numeric(18,4) NOT NULL DEFAULT 0,
    mtm             numeric(18,4) NOT NULL DEFAULT 0,
    pnl             numeric(18,4) NOT NULL DEFAULT 0,
    action          text NOT NULL DEFAULT 'exit',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, product, instrument)
);

CREATE INDEX IF NOT EXISTS account_positions_account_id_idx ON account_positions (account_id);

CREATE TABLE IF NOT EXISTS account_holdings (
    id                  bigserial PRIMARY KEY,
    account_id          uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    instrument          text NOT NULL,
    sellable_quantity   integer NOT NULL DEFAULT 0,
    buy_average_price   numeric(18,4) NOT NULL DEFAULT 0,
    ltp                 numeric(18,4) NOT NULL DEFAULT 0,
    pnl                 numeric(18,4) NOT NULL DEFAULT 0,
    action              text NOT NULL DEFAULT 'exit',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, instrument)
);

CREATE INDEX IF NOT EXISTS account_holdings_account_id_idx ON account_holdings (account_id);

CREATE TABLE IF NOT EXISTS account_margins (
    account_id       uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    net_qty          integer NOT NULL DEFAULT 0,
    total_mtm        numeric(18,4) NOT NULL DEFAULT 0,
    realized_pnl     numeric(18,4) NOT NULL DEFAULT 0,
    account_value    numeric(18,4) NOT NULL DEFAULT 0,
    status           text NOT NULL DEFAULT 'online',
    available_cash   numeric(18,4) NOT NULL DEFAULT 0,
    available_margin numeric(18,4) NOT NULL DEFAULT 0,
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- 11. Pending Order Updates (Stash Table for Zero-Lag Ingestion)
CREATE TABLE IF NOT EXISTS pending_order_updates (
    broker_order_id  text PRIMARY KEY,
    status           text NOT NULL,
    filled_quantity  integer NOT NULL DEFAULT 0,
    average_price    numeric(18,4),
    raw_payload      jsonb NOT NULL,
    received_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pending_order_updates_received_at_idx ON pending_order_updates(received_at);
