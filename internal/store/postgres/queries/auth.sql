-- name: CreateUser :exec
INSERT INTO users (id, email, username, name, password_hash, role)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserByEmail :one
SELECT id, email, username, name, phone, address, gst_number, password_hash, totp_secret, totp_enabled, role, created_at, updated_at
FROM users
WHERE LOWER(email) = LOWER($1);

-- name: GetUserByUsername :one
SELECT id, email, username, name, phone, address, gst_number, password_hash, totp_secret, totp_enabled, role, created_at, updated_at
FROM users
WHERE LOWER(username) = LOWER($1);

-- name: GetUserByID :one
SELECT id, email, username, name, phone, address, gst_number, password_hash, totp_secret, totp_enabled, role, created_at, updated_at
FROM users
WHERE id = $1;

-- name: UpdateUserProfile :exec
UPDATE users
SET email = $2, username = $3, name = $4, phone = $5, address = $6, gst_number = $7, updated_at = now()
WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, updated_at = now()
WHERE id = $1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSessionWithUser :one
SELECT s.id AS session_id, s.token_hash, s.user_id, s.ip_address, s.user_agent,
       s.expires_at, s.created_at AS session_created_at, s.last_seen_at,
       u.email, u.username, u.name, u.phone, u.address, u.gst_number, u.role, u.totp_enabled
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: TouchSession :execrows
UPDATE sessions
SET last_seen_at = now(), expires_at = $2
WHERE token_hash = $1;

-- name: DeleteSession :execrows
DELETE FROM sessions
WHERE token_hash = $1;

-- name: DeleteOtherSessions :execrows
DELETE FROM sessions
WHERE user_id = $1 AND token_hash != $2;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= now();
