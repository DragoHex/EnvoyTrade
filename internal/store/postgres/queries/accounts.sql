-- name: CreateAccount :exec
INSERT INTO accounts (id, name, role, broker, broker_user_id, api_key, api_secret, ip_address, user_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: CreateAccountWithCredentials :exec
INSERT INTO accounts (id, name, role, broker, broker_user_id, api_key, api_secret, ip_address, encrypted_password, encrypted_totp_secret, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: AccountByBrokerUserID :one
SELECT id, role, api_secret FROM accounts WHERE broker_user_id = $1;

-- name: SetAccountActive :execrows
UPDATE accounts SET active = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountName :execrows
UPDATE accounts SET name = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountIPAddress :execrows
UPDATE accounts SET ip_address = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountAPIKey :execrows
UPDATE accounts SET api_key = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountAPISecret :execrows
UPDATE accounts SET api_secret = $2, updated_at = now() WHERE id = $1;

-- name: SetAccountEncryptedCredentials :execrows
UPDATE accounts
SET encrypted_password = CASE WHEN sqlc.arg('encrypted_password')::text != '' THEN sqlc.arg('encrypted_password')::text ELSE encrypted_password END,
    encrypted_totp_secret = CASE WHEN sqlc.arg('encrypted_totp_secret')::text != '' THEN sqlc.arg('encrypted_totp_secret')::text ELSE encrypted_totp_secret END,
    updated_at = now()
WHERE id = $1;

-- name: SetAccountAccessToken :execrows
UPDATE accounts SET access_token = $2, token_expires_at = $3, auth_status = $4, auth_error = $5, updated_at = now() WHERE id = $1;

-- name: AccountAuthInfo :one
SELECT id, role, broker, broker_user_id, api_key, api_secret, ip_address, encrypted_password, encrypted_totp_secret, access_token, token_expires_at, auth_status, auth_error
FROM accounts WHERE id = $1;

-- name: MasterActive :one
SELECT active FROM accounts WHERE id = $1 AND role = 'master';

-- name: AccountRole :one
SELECT role FROM accounts WHERE id = $1;

-- name: Accounts :many
SELECT a.id, a.name, a.role, a.broker, a.broker_user_id, a.api_key, a.api_secret, a.active, a.status, a.ip_address,
       a.auth_status, a.auth_error,
       g.id AS group_id, g.name AS group_name, g.master_id,
       f.clone_factor, f.max_qty_per_order,
       COALESCE(f.enabled, true) AS enabled
FROM accounts a
LEFT JOIN follow_links f ON f.follower_id = a.id
LEFT JOIN groups g ON (g.id = f.group_id OR (a.role = 'master' AND g.master_id = a.id))
WHERE (sqlc.narg('ids')::uuid[] IS NULL OR a.id = ANY(sqlc.narg('ids')::uuid[]))
  AND (sqlc.narg('user_id')::uuid IS NULL OR a.user_id = sqlc.narg('user_id')::uuid)
ORDER BY LOWER(COALESCE(NULLIF(a.name, ''), a.broker_user_id)) ASC, a.id ASC;

-- name: UpdateFollowLinkTerms :execrows
UPDATE follow_links SET clone_factor = $2, max_qty_per_order = $3 WHERE follower_id = $1;

-- name: SetAccountStatus :execrows
UPDATE accounts SET status = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = $1;
