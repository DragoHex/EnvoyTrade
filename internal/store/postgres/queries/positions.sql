-- name: ListAccountPositions :many
SELECT id, account_id, product, instrument, quantity, buy_price, sell_price,
       buy_quantity, sell_quantity, ltp, mtm, pnl, action, created_at, updated_at
FROM account_positions
WHERE account_id = $1
ORDER BY id ASC;

-- name: ListOpenPositionsByAccount :many
SELECT id, account_id, product, instrument, quantity, buy_price, sell_price,
       buy_quantity, sell_quantity, ltp, mtm, pnl, action, created_at, updated_at
FROM account_positions
WHERE account_id = $1 AND quantity != 0
ORDER BY updated_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountOpenPositionsByAccount :one
SELECT COUNT(*) FROM account_positions
WHERE account_id = $1 AND quantity != 0;

-- name: ListClosedPositionsByAccount :many
SELECT id, account_id, product, instrument, quantity, buy_price, sell_price,
       buy_quantity, sell_quantity, ltp, mtm, pnl, action, created_at, updated_at
FROM account_positions
WHERE account_id = $1 AND quantity = 0
ORDER BY updated_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountClosedPositionsByAccount :one
SELECT COUNT(*) FROM account_positions
WHERE account_id = $1 AND quantity = 0;

-- name: UpsertAccountPosition :exec
INSERT INTO account_positions
  (account_id, product, instrument, quantity, buy_price, sell_price,
   buy_quantity, sell_quantity, ltp, mtm, pnl, action, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())
ON CONFLICT (account_id, product, instrument)
DO UPDATE SET
  quantity = EXCLUDED.quantity,
  buy_price = EXCLUDED.buy_price,
  sell_price = EXCLUDED.sell_price,
  buy_quantity = EXCLUDED.buy_quantity,
  sell_quantity = EXCLUDED.sell_quantity,
  ltp = EXCLUDED.ltp,
  mtm = EXCLUDED.mtm,
  pnl = EXCLUDED.pnl,
  action = EXCLUDED.action,
  updated_at = now();
