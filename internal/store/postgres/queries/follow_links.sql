-- name: CreateFollowLink :exec
INSERT INTO follow_links (follower_id, master_id, capital_ratio, max_qty_per_order, enabled)
VALUES ($1, $2, $3, $4, $5);

-- name: EnabledFollowLinks :many
SELECT follower_id, master_id, capital_ratio, COALESCE(max_qty_per_order, 0) AS max_qty_per_order, enabled, effective_from
FROM follow_links WHERE master_id = $1 AND enabled = true;

-- name: SetFollowLinkEnabled :execrows
UPDATE follow_links SET enabled = $2 WHERE follower_id = $1;

-- name: MasterIDForFollower :one
SELECT master_id FROM follow_links WHERE follower_id = $1;

-- name: DeleteFollowLink :execrows
DELETE FROM follow_links WHERE follower_id = $1;
