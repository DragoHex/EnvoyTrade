-- name: CreateAccount :exec
INSERT INTO accounts (id, name, role, broker, broker_user_id, api_secret) VALUES ($1, $2, $3, $4, $5, $6);

-- name: AccountByBrokerUserID :one
SELECT id, role, api_secret FROM accounts WHERE broker_user_id = $1;

-- name: SetAccountActive :execrows
UPDATE accounts SET active = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountName :execrows
UPDATE accounts SET name = $2, updated_at = now() WHERE id = $1;

-- name: MasterActive :one
SELECT active FROM accounts WHERE id = $1 AND role = 'master';

-- name: AccountRole :one
SELECT role FROM accounts WHERE id = $1;

-- name: Accounts :many
SELECT a.id, a.name, a.role, a.broker, a.broker_user_id, a.active, a.status,
       g.id AS group_id, g.name AS group_name, g.master_id,
       f.capital_ratio, f.max_qty_per_order,
       COALESCE(f.enabled, true) AS enabled
FROM accounts a
LEFT JOIN follow_links f ON f.follower_id = a.id
LEFT JOIN groups g ON g.id = f.group_id
WHERE sqlc.narg('ids')::uuid[] IS NULL OR a.id = ANY(sqlc.narg('ids')::uuid[])
ORDER BY a.role, a.name, a.broker_user_id;

-- name: UpdateFollowLinkTerms :execrows
UPDATE follow_links SET capital_ratio = $2, max_qty_per_order = $3 WHERE follower_id = $1;

-- name: SetAccountStatus :execrows
UPDATE accounts SET status = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = $1;
