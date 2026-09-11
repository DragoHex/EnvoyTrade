-- name: ListAccountHoldings :many
SELECT id, account_id, instrument, sellable_quantity, buy_average_price,
       ltp, pnl, action, created_at, updated_at
FROM account_holdings
WHERE account_id = $1
ORDER BY instrument ASC;

-- name: ListAccountHoldingsPaginated :many
SELECT id, account_id, instrument, sellable_quantity, buy_average_price,
       ltp, pnl, action, created_at, updated_at
FROM account_holdings
WHERE account_id = $1
ORDER BY instrument ASC
LIMIT $2 OFFSET $3;

-- name: CountAccountHoldings :one
SELECT COUNT(*) FROM account_holdings
WHERE account_id = $1;

-- name: UpsertAccountHolding :exec
INSERT INTO account_holdings
  (account_id, instrument, sellable_quantity, buy_average_price, ltp, pnl, action, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (account_id, instrument)
DO UPDATE SET
  sellable_quantity = EXCLUDED.sellable_quantity,
  buy_average_price = EXCLUDED.buy_average_price,
  ltp = EXCLUDED.ltp,
  pnl = EXCLUDED.pnl,
  action = EXCLUDED.action,
  updated_at = now();
