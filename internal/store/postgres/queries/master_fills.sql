-- name: InsertMasterFill :one
INSERT INTO master_fills
  (master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
   transaction_type, product, order_type, filled_quantity, average_price,
   status, order_timestamp, raw_payload, dispatch_state)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
RETURNING id;

-- name: SetMasterFillDispatchState :exec
UPDATE master_fills SET dispatch_state = $2, dispatched_at = CASE WHEN $2 = 'dispatched' THEN now() ELSE dispatched_at END
WHERE id = $1;

-- name: LatestMasterFill :one
SELECT id, master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
       transaction_type, product, order_type, filled_quantity, average_price,
       status, order_timestamp, raw_payload
FROM master_fills WHERE master_id = $1
ORDER BY order_timestamp DESC LIMIT 1;

-- name: ListMasterFillsByAccount :many
SELECT id, master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
       transaction_type, product, order_type, filled_quantity, average_price,
       status, order_timestamp, raw_payload, received_at, dispatch_state, dispatched_at
FROM master_fills
WHERE master_id = $1
ORDER BY order_timestamp DESC
LIMIT 100;

-- name: ListOpenMasterOrdersPaginated :many
SELECT id, master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
       transaction_type, product, order_type, filled_quantity, average_price,
       status, order_timestamp, raw_payload, received_at, dispatch_state, dispatched_at
FROM master_fills
WHERE master_id = $1 AND status NOT IN ('COMPLETE', 'REJECTED', 'CANCELLED')
ORDER BY order_timestamp DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountOpenMasterOrders :one
SELECT COUNT(*) FROM master_fills
WHERE master_id = $1 AND status NOT IN ('COMPLETE', 'REJECTED', 'CANCELLED');

-- name: ListClosedMasterOrdersPaginated :many
SELECT id, master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
       transaction_type, product, order_type, filled_quantity, average_price,
       status, order_timestamp, raw_payload, received_at, dispatch_state, dispatched_at
FROM master_fills
WHERE master_id = $1 AND status = 'COMPLETE'
ORDER BY order_timestamp DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountClosedMasterOrders :one
SELECT COUNT(*) FROM master_fills
WHERE master_id = $1 AND status = 'COMPLETE';

-- name: ListRejectedMasterOrdersPaginated :many
SELECT id, master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
       transaction_type, product, order_type, filled_quantity, average_price,
       status, order_timestamp, raw_payload, received_at, dispatch_state, dispatched_at
FROM master_fills
WHERE master_id = $1 AND status IN ('REJECTED', 'CANCELLED')
ORDER BY order_timestamp DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountRejectedMasterOrders :one
SELECT COUNT(*) FROM master_fills
WHERE master_id = $1 AND status IN ('REJECTED', 'CANCELLED');
