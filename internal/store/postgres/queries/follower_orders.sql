-- name: InsertFollowerOrder :one
INSERT INTO follower_orders
  (master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size, sizing_reason)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id;

-- name: GetFollowerOrder :one
SELECT id, master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size, sizing_reason,
       placed_qty, broker_order_id, terminal_status, filled_qty, average_price, attempt_count,
       last_error, created_at, updated_at
FROM follower_orders WHERE id = $1;

-- name: FollowerOrdersByMasterFill :many
SELECT id, follower_id FROM follower_orders WHERE master_fill_id = $1 ORDER BY id ASC;

-- name: UpdateFollowerOrderPlaced :exec
UPDATE follower_orders SET broker_order_id = $2, placed_qty = $3, attempt_count = attempt_count + 1, updated_at = now()
WHERE id = $1;

-- name: UpdateFollowerOrderFailed :exec
UPDATE follower_orders SET terminal_status = $2, last_error = $3, attempt_count = attempt_count + 1, updated_at = now()
WHERE id = $1;

-- name: UpdateFollowerOrderStatus :one
UPDATE follower_orders
SET terminal_status = $2, filled_qty = $3, average_price = $4, updated_at = now()
WHERE broker_order_id = $1
RETURNING id;
