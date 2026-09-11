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

-- name: PendingFollowerOrders :many
SELECT id, master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size, sizing_reason,
       placed_qty, broker_order_id, terminal_status, filled_qty, average_price, attempt_count,
       last_error, created_at, updated_at
FROM follower_orders
WHERE terminal_status IS NULL AND created_at < $1
ORDER BY id ASC;

-- name: ListFollowerOrdersWithMasterFillByAccount :many
SELECT fo.id, fo.master_fill_id, fo.follower_id, fo.idempotency_tag, fo.intended_qty,
       fo.lot_size, fo.sizing_reason, fo.placed_qty, fo.broker_order_id, fo.terminal_status,
       fo.filled_qty, fo.average_price, fo.attempt_count, fo.last_error, fo.created_at, fo.updated_at,
       mf.tradingsymbol, mf.exchange, mf.transaction_type, mf.product, mf.order_type,
       mf.order_timestamp AS master_order_timestamp, mf.raw_payload AS master_raw_payload
FROM follower_orders fo
JOIN master_fills mf ON fo.master_fill_id = mf.id
WHERE fo.follower_id = $1
ORDER BY fo.created_at DESC
LIMIT 100;

-- name: ListOpenFollowerOrdersPaginated :many
SELECT fo.id, fo.master_fill_id, fo.follower_id, fo.idempotency_tag, fo.intended_qty,
       fo.lot_size, fo.sizing_reason, fo.placed_qty, fo.broker_order_id, fo.terminal_status,
       fo.filled_qty, fo.average_price, fo.attempt_count, fo.last_error, fo.created_at, fo.updated_at,
       mf.tradingsymbol, mf.exchange, mf.transaction_type, mf.product, mf.order_type,
       mf.order_timestamp AS master_order_timestamp, mf.raw_payload AS master_raw_payload
FROM follower_orders fo
JOIN master_fills mf ON fo.master_fill_id = mf.id
WHERE fo.follower_id = $1 AND fo.terminal_status IS NULL AND fo.intended_qty > 0
ORDER BY fo.created_at DESC, fo.id DESC
LIMIT $2 OFFSET $3;

-- name: CountOpenFollowerOrders :one
SELECT COUNT(*) FROM follower_orders fo
WHERE fo.follower_id = $1 AND fo.terminal_status IS NULL AND fo.intended_qty > 0;

-- name: ListClosedFollowerOrdersPaginated :many
SELECT fo.id, fo.master_fill_id, fo.follower_id, fo.idempotency_tag, fo.intended_qty,
       fo.lot_size, fo.sizing_reason, fo.placed_qty, fo.broker_order_id, fo.terminal_status,
       fo.filled_qty, fo.average_price, fo.attempt_count, fo.last_error, fo.created_at, fo.updated_at,
       mf.tradingsymbol, mf.exchange, mf.transaction_type, mf.product, mf.order_type,
       mf.order_timestamp AS master_order_timestamp, mf.raw_payload AS master_raw_payload
FROM follower_orders fo
JOIN master_fills mf ON fo.master_fill_id = mf.id
WHERE fo.follower_id = $1 AND fo.terminal_status = 'COMPLETE'
ORDER BY fo.created_at DESC, fo.id DESC
LIMIT $2 OFFSET $3;

-- name: CountClosedFollowerOrders :one
SELECT COUNT(*) FROM follower_orders fo
WHERE fo.follower_id = $1 AND fo.terminal_status = 'COMPLETE';

-- name: ListRejectedFollowerOrdersPaginated :many
SELECT fo.id, fo.master_fill_id, fo.follower_id, fo.idempotency_tag, fo.intended_qty,
       fo.lot_size, fo.sizing_reason, fo.placed_qty, fo.broker_order_id, fo.terminal_status,
       fo.filled_qty, fo.average_price, fo.attempt_count, fo.last_error, fo.created_at, fo.updated_at,
       mf.tradingsymbol, mf.exchange, mf.transaction_type, mf.product, mf.order_type,
       mf.order_timestamp AS master_order_timestamp, mf.raw_payload AS master_raw_payload
FROM follower_orders fo
JOIN master_fills mf ON fo.master_fill_id = mf.id
WHERE fo.follower_id = $1 AND (fo.terminal_status IN ('REJECTED', 'CANCELLED', 'DEAD_LETTERED', 'dead_lettered') OR fo.intended_qty = 0)
ORDER BY fo.created_at DESC, fo.id DESC
LIMIT $2 OFFSET $3;

-- name: CountRejectedFollowerOrders :one
SELECT COUNT(*) FROM follower_orders fo
WHERE fo.follower_id = $1 AND (fo.terminal_status IN ('REJECTED', 'CANCELLED', 'DEAD_LETTERED', 'dead_lettered') OR fo.intended_qty = 0);
