-- name: GetAccountMargins :one
SELECT account_id, net_qty, total_mtm, realized_pnl, account_value, status, available_cash, available_margin, updated_at
FROM account_margins
WHERE account_id = $1;

-- name: UpsertAccountMargins :exec
INSERT INTO account_margins
  (account_id, net_qty, total_mtm, realized_pnl, account_value, available_cash, available_margin, status, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (account_id)
DO UPDATE SET
  net_qty = EXCLUDED.net_qty,
  total_mtm = EXCLUDED.total_mtm,
  realized_pnl = EXCLUDED.realized_pnl,
  account_value = EXCLUDED.account_value,
  available_cash = EXCLUDED.available_cash,
  available_margin = EXCLUDED.available_margin,
  status = EXCLUDED.status,
  updated_at = now();
