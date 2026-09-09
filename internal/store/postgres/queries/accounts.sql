-- name: CreateAccount :exec
INSERT INTO accounts (id, role, broker, broker_user_id, api_secret) VALUES ($1, $2, $3, $4, $5);

-- name: AccountByBrokerUserID :one
SELECT id, role, api_secret FROM accounts WHERE broker_user_id = $1;

-- name: SetAccountActive :execrows
UPDATE accounts SET active = $2, updated_at = now() WHERE id = $1;

-- name: MasterActive :one
SELECT active FROM accounts WHERE id = $1 AND role = 'master';

-- name: AccountRole :one
SELECT role FROM accounts WHERE id = $1;

-- name: Accounts :many
SELECT a.id, a.role, a.broker, a.broker_user_id, a.active, a.status,
       f.master_id, f.capital_ratio, f.max_qty_per_order,
       COALESCE(f.enabled, true) AS enabled
FROM accounts a
LEFT JOIN follow_links f ON f.follower_id = a.id
WHERE sqlc.narg('ids')::uuid[] IS NULL OR a.id = ANY(sqlc.narg('ids')::uuid[])
ORDER BY a.role, a.broker_user_id;

-- name: UpdateFollowLinkTerms :execrows
UPDATE follow_links SET capital_ratio = $2, max_qty_per_order = $3 WHERE follower_id = $1;

-- name: SetAccountStatus :execrows
UPDATE accounts SET status = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = $1;
