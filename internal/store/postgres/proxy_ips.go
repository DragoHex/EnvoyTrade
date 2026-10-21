package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListProxyIPs lists all proxy IPs in the system along with their current assignment status.
func (s *Store) ListProxyIPs(ctx context.Context) ([]domain.ProxyIP, error) {
	const query = `
SELECT 
    p.ip_address,
    p.ip_type,
    p.host,
    p.port,
    p.username,
    p.whitelisted_brokers,
    p.valid_from,
    p.valid_until,
    p.plan,
    p.created_at,
    a.id AS assigned_account_id,
    a.name AS assigned_account_name
FROM proxy_ips p
LEFT JOIN accounts a ON a.ip_address = p.ip_address
ORDER BY p.ip_type ASC, p.ip_address ASC;`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ProxyIP
	for rows.Next() {
		var (
			p           domain.ProxyIP
			accountID   pgtype.UUID
			accountName *string
			validFrom   pgtype.Timestamptz
			validUntil  pgtype.Timestamptz
			createdAt   pgtype.Timestamptz
		)
		if err := rows.Scan(
			&p.IPAddress,
			&p.IPType,
			&p.Host,
			&p.Port,
			&p.Username,
			&p.WhitelistedBrokers,
			&validFrom,
			&validUntil,
			&p.Plan,
			&createdAt,
			&accountID,
			&accountName,
		); err != nil {
			return nil, err
		}
		if validFrom.Valid {
			p.ValidFrom = validFrom.Time.In(domain.IST)
		}
		if validUntil.Valid {
			p.ValidUntil = validUntil.Time.In(domain.IST)
		}
		if createdAt.Valid {
			p.CreatedAt = createdAt.Time.In(domain.IST)
		}
		if accountID.Valid {
			u := uuid.UUID(accountID.Bytes)
			p.AssignedAccountID = &u
			p.AssignedAccountName = accountName
			p.IsAssigned = true
		}
		days, status := domain.ComputeExpiry(p.ValidUntil, time.Now())
		p.DaysRemaining = days
		p.ExpiryStatus = status
		result = append(result, p)
	}
	return result, rows.Err()
}

// AvailableProxyIPs returns unassigned proxy IPs of the specified type.
// If excludeAccountID is provided, that account's own assigned IP is also treated as available.
func (s *Store) AvailableProxyIPs(ctx context.Context, ipType string, excludeAccountID *uuid.UUID) ([]domain.ProxyIP, error) {
	const query = `
SELECT 
    p.ip_address,
    p.ip_type,
    p.host,
    p.port,
    p.whitelisted_brokers,
    p.valid_from,
    p.valid_until,
    p.plan,
    p.created_at
FROM proxy_ips p
LEFT JOIN accounts a ON a.ip_address = p.ip_address
WHERE (a.id IS NULL OR ($1::uuid IS NOT NULL AND a.id = $1::uuid))
  AND ($2::text = '' OR p.ip_type = $2::text)
ORDER BY p.ip_address ASC;`

	rows, err := s.pool.Query(ctx, query, excludeAccountID, ipType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ProxyIP
	for rows.Next() {
		var (
			p          domain.ProxyIP
			validFrom  pgtype.Timestamptz
			validUntil pgtype.Timestamptz
			createdAt  pgtype.Timestamptz
		)
		if err := rows.Scan(
			&p.IPAddress,
			&p.IPType,
			&p.Host,
			&p.Port,
			&p.WhitelistedBrokers,
			&validFrom,
			&validUntil,
			&p.Plan,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if validFrom.Valid {
			p.ValidFrom = validFrom.Time.In(domain.IST)
		}
		if validUntil.Valid {
			p.ValidUntil = validUntil.Time.In(domain.IST)
		}
		if createdAt.Valid {
			p.CreatedAt = createdAt.Time.In(domain.IST)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ProxyIPByAddress retrieves a single proxy IP with credentials for broker egress.
func (s *Store) ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error) {
	const query = `
SELECT 
    p.ip_address,
    p.ip_type,
    p.host,
    p.port,
    p.username,
    p.password,
    p.whitelisted_brokers,
    p.valid_from,
    p.valid_until,
    p.plan,
    p.created_at
FROM proxy_ips p
WHERE p.ip_address = $1;`

	var (
		p          domain.ProxyIP
		validFrom  pgtype.Timestamptz
		validUntil pgtype.Timestamptz
		createdAt  pgtype.Timestamptz
	)
	err := s.pool.QueryRow(ctx, query, ipAddress).Scan(
		&p.IPAddress,
		&p.IPType,
		&p.Host,
		&p.Port,
		&p.Username,
		&p.Password,
		&p.WhitelistedBrokers,
		&validFrom,
		&validUntil,
		&p.Plan,
		&createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProxyIP{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ProxyIP{}, err
	}
	if validFrom.Valid {
		p.ValidFrom = validFrom.Time.In(domain.IST)
	}
	if validUntil.Valid {
		p.ValidUntil = validUntil.Time.In(domain.IST)
	}
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time.In(domain.IST)
	}
	return p, nil
}

// UpsertProxyIP inserts or updates a static proxy configuration in the proxy_ips table.
func (s *Store) UpsertProxyIP(ctx context.Context, p domain.ProxyIP) error {
	const query = `
INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, whitelisted_brokers, valid_from, valid_until, plan)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (ip_address) DO UPDATE SET
    ip_type = EXCLUDED.ip_type,
    host = EXCLUDED.host,
    port = EXCLUDED.port,
    username = EXCLUDED.username,
    password = EXCLUDED.password,
    whitelisted_brokers = EXCLUDED.whitelisted_brokers,
    valid_from = EXCLUDED.valid_from,
    valid_until = EXCLUDED.valid_until,
    plan = EXCLUDED.plan;`

	port := p.Port
	if port == 0 {
		port = 443
	}
	plan := p.Plan
	if plan == "" {
		plan = "QUARTERLY"
	}
	validFrom := p.ValidFrom
	if validFrom.IsZero() {
		validFrom = time.Now()
	}
	validUntil := p.ValidUntil
	if validUntil.IsZero() {
		validUntil = validFrom.AddDate(0, 3, 0)
	}

	ipType := p.IPType
	if ipType == "" {
		if strings.Contains(p.IPAddress, ":") {
			ipType = "ipv6"
		} else {
			ipType = "ipv4"
		}
	}

	_, err := s.pool.Exec(ctx, query,
		p.IPAddress,
		ipType,
		p.Host,
		port,
		p.Username,
		p.Password,
		p.WhitelistedBrokers,
		validFrom,
		validUntil,
		plan,
	)
	return err
}

// UpsertProxyIPsBatch transactionally inserts or updates multiple proxy IP configurations.
func (s *Store) UpsertProxyIPsBatch(ctx context.Context, ips []domain.ProxyIP) (domain.BulkUploadResult, error) {
	result := domain.BulkUploadResult{
		Total: len(ips),
	}
	if len(ips) == 0 {
		return result, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)

	const checkQuery = `SELECT 1 FROM proxy_ips WHERE ip_address = $1;`
	const upsertQuery = `
INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, whitelisted_brokers, valid_from, valid_until, plan)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (ip_address) DO UPDATE SET
    ip_type = EXCLUDED.ip_type,
    host = EXCLUDED.host,
    port = EXCLUDED.port,
    username = EXCLUDED.username,
    password = EXCLUDED.password,
    whitelisted_brokers = EXCLUDED.whitelisted_brokers,
    valid_from = EXCLUDED.valid_from,
    valid_until = EXCLUDED.valid_until,
    plan = EXCLUDED.plan;`

	for _, p := range ips {
		var dummy int
		err := tx.QueryRow(ctx, checkQuery, p.IPAddress).Scan(&dummy)
		isExisting := (err == nil)

		port := p.Port
		if port == 0 {
			port = 443
		}
		plan := p.Plan
		if plan == "" {
			plan = "QUARTERLY"
		}
		validFrom := p.ValidFrom
		if validFrom.IsZero() {
			validFrom = time.Now()
		}
		validUntil := p.ValidUntil
		if validUntil.IsZero() {
			validUntil = validFrom.AddDate(0, 3, 0)
		}
		ipType := p.IPType
		if ipType == "" {
			if strings.Contains(p.IPAddress, ":") {
				ipType = "ipv6"
			} else {
				ipType = "ipv4"
			}
		}

		if _, err := tx.Exec(ctx, upsertQuery,
			p.IPAddress,
			ipType,
			p.Host,
			port,
			p.Username,
			p.Password,
			p.WhitelistedBrokers,
			validFrom,
			validUntil,
			plan,
		); err != nil {
			return result, err
		}

		if isExisting {
			result.Updated++
		} else {
			result.Inserted++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

// UpdateProxyIP updates validity, password, or plan for an existing proxy IP.
func (s *Store) UpdateProxyIP(ctx context.Context, ipAddress string, validUntil time.Time, password, plan string) error {
	const query = `
UPDATE proxy_ips SET
    valid_until = CASE WHEN $2::timestamptz IS NOT NULL THEN $2::timestamptz ELSE valid_until END,
    password = CASE WHEN $3::text != '' THEN $3::text ELSE password END,
    plan = CASE WHEN $4::text != '' THEN $4::text ELSE plan END
WHERE ip_address = $1;`

	var validUntilParam *time.Time
	if !validUntil.IsZero() {
		validUntilParam = &validUntil
	}

	tag, err := s.pool.Exec(ctx, query, ipAddress, validUntilParam, password, plan)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateProxyIPFull updates all configuration fields of an existing proxy IP in a single SQL operation.
// If the password provided is empty, the existing password in the database is retained.
func (s *Store) UpdateProxyIPFull(ctx context.Context, p domain.ProxyIP) error {
	const query = `
UPDATE proxy_ips SET
    ip_type = $2,
    host = $3,
    port = $4,
    username = $5,
    password = CASE WHEN $6::text != '' THEN $6::text ELSE password END,
    whitelisted_brokers = $7,
    valid_from = $8,
    valid_until = $9,
    plan = $10
WHERE ip_address = $1;`

	port := p.Port
	if port <= 0 {
		port = 443
	}
	plan := p.Plan
	if plan == "" {
		plan = "QUARTERLY"
	}
	validFrom := p.ValidFrom
	if validFrom.IsZero() {
		validFrom = time.Now()
	}
	validUntil := p.ValidUntil
	if validUntil.IsZero() {
		validUntil = validFrom.AddDate(0, 3, 0)
	}

	ipType := p.IPType
	if ipType == "" {
		if strings.Contains(p.IPAddress, ":") {
			ipType = "ipv6"
		} else {
			ipType = "ipv4"
		}
	}

	tag, err := s.pool.Exec(ctx, query,
		p.IPAddress,
		ipType,
		p.Host,
		port,
		p.Username,
		p.Password,
		p.WhitelistedBrokers,
		validFrom,
		validUntil,
		plan,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteProxyIP removes a proxy IP from the pool if it is not currently assigned to an active account.
func (s *Store) DeleteProxyIP(ctx context.Context, ipAddress string) error {
	// 1. Check if assigned to an active, non-deleted account
	const checkQuery = `
SELECT a.id, a.name 
FROM accounts a 
WHERE a.ip_address = $1 AND a.deleted_at IS NULL 
LIMIT 1;`

	var accID uuid.UUID
	var accName string
	err := s.pool.QueryRow(ctx, checkQuery, ipAddress).Scan(&accID, &accName)
	if err == nil {
		// Found active assignment -> conflict!
		return domain.ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// 2. Safe to delete
	const deleteQuery = `DELETE FROM proxy_ips WHERE ip_address = $1;`
	tag, err := s.pool.Exec(ctx, deleteQuery, ipAddress)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ProxyIPStats aggregates inventory and expiration status counts.
func (s *Store) ProxyIPStats(ctx context.Context) (domain.ProxyIPStats, error) {
	const query = `
SELECT
    COUNT(*)::int AS total_count,
    COUNT(a.id)::int AS assigned_count,
    (COUNT(*) - COUNT(a.id))::int AS available_count,
    COUNT(*) FILTER (WHERE p.ip_type = 'ipv4')::int AS ipv4_count,
    COUNT(*) FILTER (WHERE p.ip_type = 'ipv6')::int AS ipv6_count,
    COUNT(*) FILTER (WHERE p.valid_until >= now() AND p.valid_until <= now() + interval '15 days')::int AS expiring_soon_count,
    COUNT(*) FILTER (WHERE p.valid_until < now())::int AS expired_count,
    COUNT(*) FILTER (WHERE a.id IS NOT NULL AND p.valid_until <= now() + interval '15 days')::int AS assigned_expiring
FROM proxy_ips p
LEFT JOIN accounts a ON a.ip_address = p.ip_address AND a.deleted_at IS NULL;`

	var stats domain.ProxyIPStats
	err := s.pool.QueryRow(ctx, query).Scan(
		&stats.TotalCount,
		&stats.AssignedCount,
		&stats.AvailableCount,
		&stats.IPv4Count,
		&stats.IPv6Count,
		&stats.ExpiringSoonCount,
		&stats.ExpiredCount,
		&stats.AssignedExpiring,
	)
	return stats, err
}

