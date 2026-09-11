-- name: CreateFollowLink :exec
INSERT INTO follow_links (follower_id, group_id, capital_ratio, max_qty_per_order, enabled)
VALUES ($1, $2, $3, $4, $5);

-- name: EnabledFollowLinks :many
SELECT f.follower_id, g.id AS group_id, g.master_id, f.capital_ratio, COALESCE(f.max_qty_per_order, 0) AS max_qty_per_order, f.enabled, f.effective_from
FROM follow_links f
JOIN groups g ON g.id = f.group_id
WHERE g.master_id = $1 AND f.enabled = true;

-- name: SetFollowLinkEnabled :execrows
UPDATE follow_links SET enabled = $2 WHERE follower_id = $1;

-- name: MasterIDForFollower :one
SELECT g.master_id
FROM follow_links f
JOIN groups g ON g.id = f.group_id
WHERE f.follower_id = $1;

-- name: GroupIDForFollower :one
SELECT group_id FROM follow_links WHERE follower_id = $1;

-- name: DeleteFollowLink :execrows
DELETE FROM follow_links WHERE follower_id = $1;
