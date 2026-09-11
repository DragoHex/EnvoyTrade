-- Positions, Holdings, and Margins for dashboard trading drawer (PLAN.md §2).

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

CREATE TABLE IF NOT EXISTS account_margins (
  account_id      uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  net_qty         integer NOT NULL DEFAULT 0,
  total_mtm       numeric(18,4) NOT NULL DEFAULT 0,
  realized_pnl    numeric(18,4) NOT NULL DEFAULT 0,
  account_value   numeric(18,4) NOT NULL DEFAULT 0,
  status          text NOT NULL DEFAULT 'online',
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS follower_orders_follower_id_idx ON follower_orders (follower_id, created_at DESC);
CREATE INDEX IF NOT EXISTS master_fills_master_id_idx ON master_fills (master_id, order_timestamp DESC);
CREATE INDEX IF NOT EXISTS account_positions_account_id_idx ON account_positions (account_id);
CREATE INDEX IF NOT EXISTS account_holdings_account_id_idx ON account_holdings (account_id);
