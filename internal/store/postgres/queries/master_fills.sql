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
