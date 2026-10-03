package postgres

import (
	"context"
	"errors"

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
			p.ValidFrom = validFrom.Time
		}
		if validUntil.Valid {
			p.ValidUntil = validUntil.Time
		}
		if createdAt.Valid {
			p.CreatedAt = createdAt.Time
		}
		if accountID.Valid {
			u := uuid.UUID(accountID.Bytes)
			p.AssignedAccountID = &u
			p.AssignedAccountName = accountName
			p.IsAssigned = true
		}
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
			&validFrom,
			&validUntil,
			&p.Plan,
			&createdAt,
		); err != nil {
			return nil, err
		}
		if validFrom.Valid {
			p.ValidFrom = validFrom.Time
		}
		if validUntil.Valid {
			p.ValidUntil = validUntil.Time
		}
		if createdAt.Valid {
			p.CreatedAt = createdAt.Time
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
		p.ValidFrom = validFrom.Time
	}
	if validUntil.Valid {
		p.ValidUntil = validUntil.Time
	}
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	}
	return p, nil
}
