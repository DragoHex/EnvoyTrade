-- Instrument master (PLAN.md §2). Lot size is resolved from here rather
-- than trusted from a fill payload — F&O lot sizes vary per contract and
-- get revised by the exchange, so a signal must not carry its own guess.
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
