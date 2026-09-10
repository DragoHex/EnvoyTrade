-- name: Groups :many
SELECT g.id, g.name, g.master_id,
       a.broker_user_id AS master_broker_user_id,
       a.name AS master_name,
       a.broker,
       a.status,
       a.active,
       COUNT(f.follower_id)::int AS follower_count
FROM groups g
JOIN accounts a ON a.id = g.master_id
LEFT JOIN follow_links f ON f.group_id = g.id
GROUP BY g.id, g.name, g.master_id, a.broker_user_id, a.name, a.broker, a.status, a.active
ORDER BY g.created_at ASC;

-- name: GroupInfo :one
SELECT g.id, g.name, g.master_id,
       a.broker_user_id AS master_broker_user_id,
       a.name AS master_name,
       a.active AS master_active
FROM groups g
JOIN accounts a ON a.id = g.master_id
WHERE g.id = $1 OR g.master_id = $1
LIMIT 1;

-- name: GroupFollowerRows :many
SELECT a.id, a.name, a.broker_user_id, f.enabled, a.status
FROM follow_links f
JOIN accounts a ON a.id = f.follower_id
WHERE f.group_id = $1
ORDER BY a.created_at ASC;

-- name: CreateGroup :exec
INSERT INTO groups (id, name, master_id)
VALUES ($1, $2, $3);

-- name: UpdateGroupName :execrows
UPDATE groups SET name = $2, updated_at = now() WHERE id = $1;

-- name: UpdateGroupMaster :execrows
UPDATE groups SET master_id = $2, updated_at = now() WHERE id = $1;

-- name: DeleteGroup :execrows
DELETE FROM groups WHERE id = $1;

-- name: GroupByID :one
SELECT id, name, master_id, created_at, updated_at FROM groups WHERE id = $1;

-- name: GroupByMasterID :one
SELECT id, name, master_id, created_at, updated_at FROM groups WHERE master_id = $1 LIMIT 1;
