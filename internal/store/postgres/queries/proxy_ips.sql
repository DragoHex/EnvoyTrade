-- name: ListProxyIPs :many
SELECT 
    p.ip_address,
    p.ip_type,
    p.host,
    p.port,
    p.valid_from,
    p.valid_until,
    p.plan,
    p.created_at,
    a.id AS assigned_account_id,
    a.name AS assigned_account_name
FROM proxy_ips p
LEFT JOIN accounts a ON a.ip_address = p.ip_address
ORDER BY p.ip_type ASC, p.ip_address ASC;

-- name: AvailableProxyIPs :many
SELECT 
    p.ip_address,
    p.ip_type,
    p.host,
    p.port,
    p.valid_from,
    p.valid_until,
    p.plan,
    p.created_at
FROM proxy_ips p
LEFT JOIN accounts a ON a.ip_address = p.ip_address
WHERE (a.id IS NULL OR ($1::uuid IS NOT NULL AND a.id = $1::uuid))
  AND ($2::text = '' OR p.ip_type = $2::text)
ORDER BY p.ip_address ASC;

-- name: ProxyIPByAddress :one
SELECT 
    p.ip_address,
    p.ip_type,
    p.host,
    p.port,
    p.username,
    p.password,
    p.valid_from,
    p.valid_until,
    p.plan,
    p.created_at
FROM proxy_ips p
WHERE p.ip_address = $1;
