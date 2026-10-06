-- scripts/seed_test_data.sql
-- Injects representative test data (users, masters, followers, groups, follow links, positions, holdings, margins, fills) into EnvoyTrade database.
-- All inserts use ON CONFLICT DO UPDATE / DO NOTHING so this script is safely idempotent and re-runnable.

BEGIN;

-- 0. Users: Test users for authentication and multi-tenant scoping
-- Password for both accounts is: password123 (bcrypt cost 12 hash)
INSERT INTO users (id, email, username, name, password_hash, role, phone, address, gst_number, created_at, updated_at)
VALUES
  ('a0000000-0000-0000-0000-000000000001', 'trader@envoytrade.com', 'trader', 'Demo Trader', '$2a$12$Ut7Sf7HsyDpwWGURAB4zUOBDdeYELVYMybmk6cCNrUD3nOD5o68ci', 'admin', '+919876543210', '123 Dalal Street, Fort, Mumbai, Maharashtra 400001', '27AABCU9603R1ZM', now(), now()),
  ('a0000000-0000-0000-0000-000000000002', 'bob@envoytrade.com',    'bob',    'Bob Trader',  '$2a$12$Ut7Sf7HsyDpwWGURAB4zUOBDdeYELVYMybmk6cCNrUD3nOD5o68ci', 'user',  '+919876543211', '456 MG Road, Bengaluru, Karnataka 560001',          '',                now(), now())
ON CONFLICT (id) DO UPDATE SET
  email         = EXCLUDED.email,
  username      = EXCLUDED.username,
  name          = EXCLUDED.name,
  password_hash = EXCLUDED.password_hash,
  role          = EXCLUDED.role,
  phone         = EXCLUDED.phone,
  address       = EXCLUDED.address,
  gst_number    = EXCLUDED.gst_number,
  updated_at    = now();

-- 0.1 Proxy IPs: Static proxy IP pool for broker accounts
INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, valid_from, valid_until, plan)
VALUES
  ('148.113.41.41', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u1', 'mock_p1', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.42', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u2', 'mock_p2', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.43', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u3', 'mock_p3', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.44', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u4', 'mock_p4', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.45', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u5', 'mock_p5', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.46', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u6', 'mock_p6', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.47', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u7', 'mock_p7', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.48', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u8', 'mock_p8', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.49', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u9', 'mock_p9', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
  ('148.113.41.50', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u10', 'mock_p10', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY')
ON CONFLICT (ip_address) DO UPDATE SET
  host        = EXCLUDED.host,
  port        = EXCLUDED.port,
  username    = EXCLUDED.username,
  password    = EXCLUDED.password,
  valid_from  = EXCLUDED.valid_from,
  valid_until = EXCLUDED.valid_until;

-- 1. Accounts: 2 masters and 8 followers, scoped to primary demo user
-- MASTER01 has IP assigned; MASTER02 has no IP (to test master without IP behavior)
INSERT INTO accounts (id, user_id, role, broker, broker_user_id, status, api_key, api_secret, access_token, ip_address, auth_status, active, name, created_at, updated_at)
VALUES
  ('f6e70723-b904-4427-83ee-a85771dee2e4', 'a0000000-0000-0000-0000-000000000001', 'master',   'testbroker', 'MASTER01',  'active', 'key_master01',  'secret_master01',  'token_master01',  '148.113.41.41', 'authenticated', true, 'Rajesh Sharma',   '2026-09-08 02:23:46.867428+00', '2026-09-11 14:20:40.549228+00'),
  ('c80b3596-4449-4f32-b25a-a5bfd6883c4f', 'a0000000-0000-0000-0000-000000000001', 'master',   'testbroker', 'MASTER02',  'active', 'key_master02',  'secret_master02',  'token_master02',  '',              'authenticated', true, 'Priya Patel',     '2026-09-08 02:23:46.867428+00', '2026-09-11 15:14:57.102543+00'),
  ('a5183e89-6cb2-4d32-91a2-6dc525570185', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW01A', 'active', 'key_follow01a', 'secret_follow01a', 'token_follow01a', '148.113.41.42', 'authenticated', true, 'Amit Verma',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.865876+00'),
  ('5894c29d-7740-4494-9c98-c9d59d1364bc', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW01B', 'active', 'key_follow01b', 'secret_follow01b', 'token_follow01b', '148.113.41.43', 'authenticated', true, 'Sneha Kulkarni',  '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.883042+00'),
  ('18dfc57d-0f23-49af-8ccd-1c0edcbe4788', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW01C', 'active', 'key_follow01c', 'secret_follow01c', 'token_follow01c', '148.113.41.44', 'authenticated', true, 'Vikram Malhotra', '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.901048+00'),
  ('ef0a6211-e753-42b8-a6eb-9c8d15ad90db', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW01D', 'active', 'key_follow01d', 'secret_follow01d', 'token_follow01d', '148.113.41.45', 'authenticated', true, 'Ananya Iyer',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.921094+00'),
  ('0f40d9f2-34aa-42fa-8d99-90b9255fd168', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW02A', 'active', 'key_follow02a', 'secret_follow02a', 'token_follow02a', '148.113.41.46', 'authenticated', true, 'Rohan Gupta',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.939633+00'),
  ('d0527ce3-4c4b-40c4-ba0d-af5a79f19a99', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW02B', 'active', 'key_follow02b', 'secret_follow02b', 'token_follow02b', '148.113.41.47', 'authenticated', true, 'Neha Deshmukh',   '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.957899+00'),
  ('d800e6e7-5d11-4d35-b24a-74a9b4aa4832', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW02C', 'active', 'key_follow02c', 'secret_follow02c', 'token_follow02c', '148.113.41.48', 'authenticated', true, 'Aditya Nair',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.981594+00'),
  ('4f9b42e4-e1ea-4fab-b9f7-e6197c28ea9a', 'a0000000-0000-0000-0000-000000000001', 'follower', 'testbroker', 'FOLLOW02D', 'active', 'key_follow02d', 'secret_follow02d', 'token_follow02d', '148.113.41.49', 'authenticated', true, 'Pooja Mehta',     '2026-09-08 02:23:46.867428+00', '2026-09-11 14:21:01.996682+00')
ON CONFLICT (id) DO UPDATE SET
  user_id        = EXCLUDED.user_id,
  role           = EXCLUDED.role,
  broker         = EXCLUDED.broker,
  broker_user_id = EXCLUDED.broker_user_id,
  status         = EXCLUDED.status,
  api_key        = EXCLUDED.api_key,
  api_secret     = EXCLUDED.api_secret,
  access_token   = EXCLUDED.access_token,
  ip_address     = EXCLUDED.ip_address,
  auth_status    = EXCLUDED.auth_status,
  active         = EXCLUDED.active,
  name           = EXCLUDED.name,
  created_at     = EXCLUDED.created_at,
  updated_at     = EXCLUDED.updated_at;

-- 2. Groups: Master groups linked to master accounts, scoped to primary demo user
-- Clean up legacy seeded groups where group.id equaled master_id
DELETE FROM follow_links WHERE group_id IN ('f6e70723-b904-4427-83ee-a85771dee2e4', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f');
DELETE FROM groups WHERE id IN ('f6e70723-b904-4427-83ee-a85771dee2e4', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f');

INSERT INTO groups (id, user_id, name, master_id, created_at, updated_at)
VALUES
  ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'MASTER01', 'f6e70723-b904-4427-83ee-a85771dee2e4', '2026-09-10 05:47:09.190409+00', '2026-09-10 12:08:08.896722+00'),
  ('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'MASTER02', 'c80b3596-4449-4f32-b25a-a5bfd6883c4f', '2026-09-10 05:47:09.190409+00', '2026-09-10 05:47:09.190409+00')
ON CONFLICT (id) DO UPDATE SET
  user_id    = EXCLUDED.user_id,
  name       = EXCLUDED.name,
  master_id  = EXCLUDED.master_id,
  created_at = EXCLUDED.created_at,
  updated_at = EXCLUDED.updated_at;

-- 3. Follow links: Followers linked to groups with multiplier ratios
INSERT INTO follow_links (follower_id, clone_factor, max_qty_per_order, enabled, effective_from, group_id)
VALUES
  ('a5183e89-6cb2-4d32-91a2-6dc525570185', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000001'),
  ('5894c29d-7740-4494-9c98-c9d59d1364bc', 0.500000, NULL, false, '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000001'),
  ('18dfc57d-0f23-49af-8ccd-1c0edcbe4788', 0.750000, 500,  true,  '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000001'),
  ('ef0a6211-e753-42b8-a6eb-9c8d15ad90db', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000001'),
  ('0f40d9f2-34aa-42fa-8d99-90b9255fd168', 1.000000, NULL, true,  '2026-09-10 04:54:59.917762+00', 'b0000000-0000-0000-0000-000000000002'),
  ('d0527ce3-4c4b-40c4-ba0d-af5a79f19a99', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000002'),
  ('d800e6e7-5d11-4d35-b24a-74a9b4aa4832', 0.500000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000002'),
  ('4f9b42e4-e1ea-4fab-b9f7-e6197c28ea9a', 1.000000, NULL, true,  '2026-09-08 02:23:46.867428+00', 'b0000000-0000-0000-0000-000000000002')
ON CONFLICT (follower_id) DO UPDATE SET
  clone_factor      = EXCLUDED.clone_factor,
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

-- 8. Instruments: Canonical lot sizes and tick sizes for test contracts
INSERT INTO instruments (instrument_token, exchange, tradingsymbol, lot_size, tick_size, segment, expiry, refreshed_at)
VALUES
  (408065, 'NFO', 'NIFTY26OCTFUT',        75,  0.0500, 'NFO-FUT', '2026-10-29', now()),
  (738561, 'NSE', 'RELIANCE',               1,  0.0500, 'NSE-EQ',  NULL,         now()),
  (1001,   'MCX', 'CRUDEOIL17SEP26C10600', 100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1002,   'MCX', 'CRUDEOIL17SEP26C10700', 100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1003,   'MCX', 'CRUDEOIL17SEP26C11000', 100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1004,   'MCX', 'CRUDEOIL17SEP26P8400',  100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1005,   'MCX', 'CRUDEOIL17SEP26P8500',  100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1006,   'MCX', 'CRUDEOIL17SEP26P8600',  100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1007,   'MCX', 'CRUDEOIL17SEP26P8700',  100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1008,   'MCX', 'CRUDEOILM21SEP26',       10, 0.0500, 'MCX-FUT', '2026-09-21', now()),
  (1009,   'MCX', 'CRUDEOIL17SEP26P8200',  100, 0.0500, 'MCX-OPT', '2026-09-17', now()),
  (1010,   'MCX', 'CRUDEOIL21SEP26',       100, 0.0500, 'MCX-FUT', '2026-09-21', now())
ON CONFLICT (exchange, tradingsymbol) DO UPDATE SET
  lot_size     = EXCLUDED.lot_size,
  tick_size    = EXCLUDED.tick_size,
  segment      = EXCLUDED.segment,
  expiry       = EXCLUDED.expiry,
  refreshed_at = now();

COMMIT;
