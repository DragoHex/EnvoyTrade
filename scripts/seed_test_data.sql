-- scripts/seed_test_data.sql
-- Injects representative test data (masters, followers, groups, follow links, positions, holdings, margins, fills) into EnvoyTrade database.
-- All inserts use ON CONFLICT DO UPDATE / DO NOTHING so this script is safely idempotent and re-runnable.

BEGIN;

-- 1. Accounts: 2 masters and 8 followers
INSERT INTO accounts (id, role, broker, broker_user_id, status, api_secret, active, name, created_at, updated_at)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'master',   'zerodha', 'MASTER01',  'active', '', true, 'Rajesh Sharma',   '2026-09-08 02:23:46.867428+00', '2026-09-11 14:20:40.549228+00'),
  ('c80b3596-4449-4f32-b25a-a5bfd6883c4f', 'master',   'zerodha', 'MASTER02',  'active', '', true, 'Priya Patel',     '2026-09-08 02:23:46.867428+00', '2026-09-11 15:14:57.102543+00'),
  ('a5183e89-6cb2-4d32-91a2-6dc525570185', 'follower', 'zerodha', 'FOLLOW01A', 'active', '', true, 'Amit Verma',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.865876+00'),
  ('5894c29d-7740-4494-9c98-c9d59d1364bc', 'follower', 'zerodha', 'FOLLOW01B', 'active', '', true, 'Sneha Kulkarni',  '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.883042+00'),
  ('18dfc57d-0f23-49af-8ccd-1c0edcbe4788', 'follower', 'zerodha', 'FOLLOW01C', 'error',  '', true, 'Vikram Malhotra', '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.901048+00'),
  ('ef0a6211-e753-42b8-a6eb-9c8d15ad90db', 'follower', 'zerodha', 'FOLLOW01D', 'active', '', true, 'Ananya Iyer',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.921094+00'),
  ('0f40d9f2-34aa-42fa-8d99-90b9255fd168', 'follower', 'zerodha', 'FOLLOW02A', 'active', '', true, 'Rohan Gupta',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.939633+00'),
  ('d0527ce3-4c4b-40c4-ba0d-af5a79f19a99', 'follower', 'zerodha', 'FOLLOW02B', 'active', '', true, 'Neha Deshmukh',   '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.957899+00'),
  ('d800e6e7-5d11-4d35-b24a-74a9b4aa4832', 'follower', 'zerodha', 'FOLLOW02C', 'active', '', true, 'Aditya Nair',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.981594+00'),
  ('4f9b42e4-e1ea-4fab-b9f7-e6197c28ea9a', 'follower', 'zerodha', 'FOLLOW02D', 'error',  '', true, 'Pooja Mehta',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.996682+00')
ON CONFLICT (id) DO UPDATE SET
  role           = EXCLUDED.role,
  broker         = EXCLUDED.broker,
  broker_user_id = EXCLUDED.broker_user_id,
  status         = EXCLUDED.status,
  api_secret     = EXCLUDED.api_secret,
  active         = EXCLUDED.active,
  name           = EXCLUDED.name,
  created_at     = EXCLUDED.created_at,
  updated_at     = EXCLUDED.updated_at;

-- 2. Groups: Master groups linked to master accounts
INSERT INTO groups (id, name, master_id, created_at, updated_at)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'MASTER01', 'f6e70723-b904-4427-83ee-a85771dee2e4', '2026-09-10 05:47:09.190409+00', '2026-09-10 12:08:08.896722+00'),
  ('c80b3596-4449-4f32-b25a-a5bfd6883c4f', 'MASTER02', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f', '2026-09-10 05:47:09.190409+00', '2026-09-10 05:47:09.190409+00')
ON CONFLICT (id) DO UPDATE SET
  name       = EXCLUDED.name,
  master_id  = EXCLUDED.master_id,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at;

-- 3. Follow links: Followers linked to groups with multiplier ratios
INSERT INTO follow_links (follower_id, capital_ratio, max_qty_per_order, enabled, effective_from, group_id)
VALUES
  ('5894c29d-7740-4494-9c98-c9d59d1364bc', 0.500000, NULL, false, '2026-09-08 02:23:46.867428+00', 'f6e70723-b904-4427-83ee-a85771dee2e4'),
  ('18dfc57d-0f23-49af-8ccd-1c0edcbe4788', 0.750000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'f6e70723-b904-4427-83ee-a85771dee2e4'),
  ('ef0a6211-e753-42b8-a6eb-9c8d15ad90db', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'f6e70723-b904-4427-83ee-a85771dee2e4'),
  ('0f40d9f2-34aa-42fa-8d99-90b9255fd168', 1.000000, NULL, true,  '2026-09-10 04:54:59.917762+00', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f'),
  ('d0527ce3-4c4b-40c4-ba0d-af5a79f19a99', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f'),
  ('d800e6e7-5d11-4d35-b24a-74a9b4aa4832', 0.500000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f'),
  ('4f9b42e4-e1ea-4fab-b9f7-e6197c28ea9a', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f')
ON CONFLICT (follower_id) DO UPDATE SET
  capital_ratio     = EXCLUDED.capital_ratio,
  max_qty_per_order = EXCLUDED.max_qty_per_order,
  enabled           = EXCLUDED.enabled,
  effective_from    = EXCLUDED.effective_from,
  group_id          = EXCLUDED.group_id;

-- 4. Margins: Summary metrics for master account MASTER01
INSERT INTO account_margins (account_id, net_qty, total_mtm, realized_pnl, account_value, status, updated_at)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', -890, 380.0000, 0.0000, 2163520.8400, 'online', now())
ON CONFLICT (account_id) DO UPDATE SET
  net_qty       = EXCLUDED.net_qty,
  total_mtm     = EXCLUDED.total_mtm,
  realized_pnl  = EXCLUDED.realized_pnl,
  account_value = EXCLUDED.account_value,
  status        = EXCLUDED.status,
  updated_at    = EXCLUDED.updated_at;

-- 5. Positions: Open and closed positions for master account MASTER01
INSERT INTO account_positions (account_id, product, instrument, quantity, buy_price, sell_price, buy_quantity, sell_quantity, ltp, mtm, pnl, action, updated_at)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26C10600', -100, 0.0000,    111.1000, 0,   100, 114.4000, -330.0000, -330.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26C10700', -100, 0.0000,    137.2000, 0,   100, 101.6000, 3560.0000, 3560.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26C11000', -200, 0.0000,    67.1000,  0,   200, 71.0000,  -780.0000, -780.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26P8400',  -200, 0.0000,    45.6500,  0,   200, 43.0000,   530.0000,  530.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26P8500',  -100, 0.0000,    53.2000,  0,   100, 50.5000,   270.0000,  270.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26P8600',  -100, 0.0000,    59.7000,  0,   100, 60.6000,   -90.0000,  -90.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26P8700',  -100, 0.0000,    53.9000,  0,   100, 73.3000, -1940.0000, -1940.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOILM21SEP26',       10,   9896.0000, 0.0000,   10,  0,   9580.0000, -3160.0000, -3160.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL17SEP26P8200',   0,    23.0000,   40.2000,  100, 100, 29.7000,  1720.0000, 1720.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'CNC', 'CRUDEOIL21SEP26',        0,    9885.0000, 9891.0000, 100, 100, 9584.0000,  600.0000,  600.0000, 'exit', now())
ON CONFLICT (account_id, product, instrument) DO UPDATE SET
  quantity      = EXCLUDED.quantity,
  buy_price     = EXCLUDED.buy_price,
  sell_price    = EXCLUDED.sell_price,
  buy_quantity  = EXCLUDED.buy_quantity,
  sell_quantity = EXCLUDED.sell_quantity,
  ltp           = EXCLUDED.ltp,
  mtm           = EXCLUDED.mtm,
  pnl           = EXCLUDED.pnl,
  action        = EXCLUDED.action,
  updated_at    = now();

-- 6. Holdings: Equity holdings for master account MASTER01
INSERT INTO account_holdings (account_id, instrument, sellable_quantity, buy_average_price, ltp, pnl, action, updated_at)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ASIANPAINT-EQ', 1,   2369.2000, 2469.2000,    100.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ATHERENERG',    100,  891.1000, 1656.0000,  76490.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'BHARTIARTL-EQ', 1,   756.0100, 1842.5000,   1086.4900, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'DELHIVERY',     1,   259.8500,  438.5000,    178.6500, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'EXIDEIND-EQ',   100,  331.8000,  414.4000,   8260.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'GOLDBEES',      200,  122.8000,  125.1800,    476.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'HAL',           1,   2574.3100, 4921.7000,   2347.3900, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'HCLTECH-EQ',    0,   1363.8700, 1219.3000, -15567.0000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'HDFCBANK',      100,  860.1700,  703.8500, -15631.6700, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'HINDZINC',      1,   491.4000,  577.5000,     86.1000, 'exit', now()),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'IEX-EQ',        1,   131.5000,  114.0300,    -17.4700, 'exit', now())
ON CONFLICT (account_id, instrument) DO UPDATE SET
  sellable_quantity = EXCLUDED.sellable_quantity,
  buy_average_price = EXCLUDED.buy_average_price,
  ltp               = EXCLUDED.ltp,
  pnl               = EXCLUDED.pnl,
  action            = EXCLUDED.action,
  updated_at        = now();

-- 7. Master fills: Closed order history for master account MASTER01
INSERT INTO master_fills
  (master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
   transaction_type, product, order_type, filled_quantity, average_price,
   status, order_timestamp, raw_payload, dispatch_state)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-001', 'MCX', 'CRUDEOIL17SEP26C10600', 0, 'SELL', 'CNC', 'LIMIT', 100, 111.1000, 'COMPLETE', '2026-09-11 14:24:05+00', '{}'::jsonb, 'dispatched'),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-002', 'MCX', 'CRUDEOIL17SEP26C10700', 0, 'SELL', 'CNC', 'LIMIT', 100, 137.2000, 'COMPLETE', '2026-09-11 11:18:42+00', '{}'::jsonb, 'dispatched'),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-003', 'MCX', 'CRUDEOIL21SEP26',      0, 'SELL', 'CNC', 'LIMIT', 100, 9891.0000, 'COMPLETE', '2026-09-11 09:22:37+00', '{}'::jsonb, 'dispatched'),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-004', 'MCX', 'CRUDEOILM21SEP26',     0, 'BUY',  'CNC', 'LIMIT',  10, 9896.0000, 'COMPLETE', '2026-09-11 09:22:34+00', '{}'::jsonb, 'dispatched'),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-005', 'MCX', 'CRUDEOIL21SEP26',      0, 'BUY',  'CNC', 'LIMIT', 100, 9885.0000, 'COMPLETE', '2026-09-11 09:19:01+00', '{}'::jsonb, 'dispatched'),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-006', 'MCX', 'CRUDEOIL17SEP26P8200',  0, 'BUY',  'CNC', 'LIMIT', 100,   23.0000, 'COMPLETE', '2026-09-11 09:17:37+00', '{}'::jsonb, 'dispatched'),
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'ORD-M1-007', 'MCX', 'CRUDEOIL17SEP26P8700',  0, 'SELL', 'CNC', 'LIMIT', 100,   53.9000, 'COMPLETE', '2026-09-11 09:17:28+00', '{}'::jsonb, 'dispatched')
ON CONFLICT (master_id, broker_order_id, filled_quantity, status) DO NOTHING;

COMMIT;
