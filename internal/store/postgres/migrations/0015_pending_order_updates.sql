-- 0015_pending_order_updates.sql
-- Staging table for early postbacks that arrive before the worker's
-- PlaceOrder placement write commits to follower_orders (Stash Table Pattern).

CREATE TABLE IF NOT EXISTS pending_order_updates (
    broker_order_id  TEXT PRIMARY KEY,
    status           TEXT NOT NULL,
    filled_quantity  INTEGER NOT NULL DEFAULT 0,
    average_price    NUMERIC(18,4),
    raw_payload      JSONB NOT NULL,
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pending_order_updates_received_at_idx
    ON pending_order_updates(received_at);
