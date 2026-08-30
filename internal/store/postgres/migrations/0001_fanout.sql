-- Trimmed subset of PLAN.md §2 needed by the fan-out mechanism only.
-- account_sessions, kill_switch, and instruments are out of scope for
-- this slice (PLAN.md's WS listener, kill switch, and reconciliation
-- milestones are not built yet).

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE account_role AS ENUM ('master', 'follower');

CREATE TABLE accounts (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  role            account_role NOT NULL,
  broker          text NOT NULL DEFAULT 'zerodha',
  broker_user_id  text NOT NULL,
  status          text NOT NULL DEFAULT 'active',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (broker, broker_user_id)
);

CREATE TABLE follow_links (
  follower_id       uuid PRIMARY KEY REFERENCES accounts(id),
  master_id         uuid NOT NULL REFERENCES accounts(id),
  capital_ratio     numeric(10,6) NOT NULL CHECK (capital_ratio > 0),
  max_qty_per_order integer,
  enabled           boolean NOT NULL DEFAULT true,
  effective_from    timestamptz NOT NULL DEFAULT now(),
  CHECK (follower_id <> master_id)
);

CREATE TABLE master_fills (
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

CREATE TABLE follower_orders (
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

CREATE TABLE order_events (
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
CREATE INDEX ON order_events (account_id, occurred_at);
CREATE INDEX ON order_events (follower_order_id, id);
