-- name: AppendOrderEvent :exec
INSERT INTO order_events (follower_order_id, master_fill_id, account_id, event_type, payload)
VALUES ($1,$2,$3,$4,$5);

-- name: OrderEventsByFollowerOrder :many
SELECT id, follower_order_id, master_fill_id, account_id, event_type, payload, occurred_at
FROM order_events WHERE follower_order_id = $1 ORDER BY id ASC;
