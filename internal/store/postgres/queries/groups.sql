-- name: Groups :many
SELECT a.id, a.broker_user_id, a.broker, a.status, a.active,
       COUNT(f.follower_id)::int AS follower_count
FROM accounts a
LEFT JOIN follow_links f ON f.master_id = a.id
WHERE a.role = 'master'
GROUP BY a.id, a.broker_user_id, a.broker, a.status, a.active
ORDER BY a.created_at ASC;

-- name: GroupMasterInfo :one
SELECT broker_user_id, active FROM accounts WHERE id = $1 AND role = 'master';

-- name: GroupFollowerRows :many
SELECT a.id, a.broker_user_id, f.enabled, a.status
FROM follow_links f
JOIN accounts a ON a.id = f.follower_id
WHERE f.master_id = $1
ORDER BY a.created_at ASC;
