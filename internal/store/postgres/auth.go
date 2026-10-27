package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateUser inserts a new user record.
func (s *Store) CreateUser(ctx context.Context, id uuid.UUID, email, username, name, passwordHash, role string) error {
	err := s.queries.CreateUser(ctx, sqlcgen.CreateUserParams{
		ID:           id,
		Email:        email,
		Username:     username,
		Name:         name,
		PasswordHash: passwordHash,
		Role:         role,
	})
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	return err
}

// GetUserByEmail finds a user by their email address (case-insensitive).
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.User{
		ID:           row.ID,
		Email:        row.Email,
		Username:     row.Username,
		Name:         row.Name,
		Phone:        row.Phone,
		Address:      row.Address,
		GSTNumber:    row.GstNumber,
		PasswordHash: row.PasswordHash,
		Role:         row.Role,
		Active:       row.Active,
		TotpSecret:   row.TotpSecret,
		TotpEnabled:  row.TotpEnabled,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

// GetUserByUsername finds a user by their username (case-insensitive).
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	row, err := s.queries.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.User{
		ID:           row.ID,
		Email:        row.Email,
		Username:     row.Username,
		Name:         row.Name,
		Phone:        row.Phone,
		Address:      row.Address,
		GSTNumber:    row.GstNumber,
		PasswordHash: row.PasswordHash,
		Role:         row.Role,
		Active:       row.Active,
		TotpSecret:   row.TotpSecret,
		TotpEnabled:  row.TotpEnabled,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}, nil
}

// GetUserByID finds a user by their primary key UUID.
func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	const query = `
SELECT id, email, username, name, phone, address, gst_number, password_hash,
       totp_secret, totp_enabled, role, active, max_ips, broker_ip_limits,
       created_at, updated_at
FROM users
WHERE id = $1;`

	var (
		u                 domain.User
		brokerLimitsBytes []byte
		createdAt         pgtype.Timestamptz
		updatedAt         pgtype.Timestamptz
	)
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Email,
		&u.Username,
		&u.Name,
		&u.Phone,
		&u.Address,
		&u.GSTNumber,
		&u.PasswordHash,
		&u.TotpSecret,
		&u.TotpEnabled,
		&u.Role,
		&u.Active,
		&u.MaxIPs,
		&brokerLimitsBytes,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if len(brokerLimitsBytes) > 0 {
		var limits map[string]int
		if err := json.Unmarshal(brokerLimitsBytes, &limits); err == nil {
			u.BrokerIPLimits = limits
		}
	}
	if createdAt.Valid {
		u.CreatedAt = createdAt.Time.In(domain.IST)
	}
	if updatedAt.Valid {
		u.UpdatedAt = updatedAt.Time.In(domain.IST)
	}
	return &u, nil
}

// UpdateUserProfile updates a user's editable profile information.
func (s *Store) UpdateUserProfile(ctx context.Context, id uuid.UUID, email, username, name, phone, address, gstNumber string) error {
	err := s.queries.UpdateUserProfile(ctx, sqlcgen.UpdateUserProfileParams{
		ID:        id,
		Email:     email,
		Username:  username,
		Name:      name,
		Phone:     phone,
		Address:   address,
		GstNumber: gstNumber,
	})
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	return err
}

// UpdateUserPassword updates the password hash for a given user.
func (s *Store) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return s.queries.UpdateUserPassword(ctx, sqlcgen.UpdateUserPasswordParams{
		ID:           id,
		PasswordHash: passwordHash,
	})
}

// CountUsers returns the total count of users registered.
func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	return s.queries.CountUsers(ctx)
}

// CreateSession creates a new session associated with a user.
func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID uuid.UUID, ip, userAgent string, expiresAt time.Time) error {
	var ipPtr, uaPtr *string
	if ip != "" {
		ipPtr = &ip
	}
	if userAgent != "" {
		uaPtr = &userAgent
	}
	return s.queries.CreateSession(ctx, sqlcgen.CreateSessionParams{
		TokenHash: tokenHash,
		UserID:    userID,
		IpAddress: ipPtr,
		UserAgent: uaPtr,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// GetSessionWithUser retrieves an active, unexpired session and its associated user by token hash.
func (s *Store) GetSessionWithUser(ctx context.Context, tokenHash string) (*domain.SessionWithUser, error) {
	const query = `
SELECT s.id, s.token_hash, s.user_id, s.ip_address, s.user_agent,
       s.expires_at, s.created_at, s.last_seen_at,
       u.email, u.username, u.name, u.phone, u.address, u.gst_number, u.role, u.active, u.totp_enabled
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();`

	var (
		sessWithUser domain.SessionWithUser
		expiresAt    pgtype.Timestamptz
		sessCreated  pgtype.Timestamptz
		lastSeenAt   pgtype.Timestamptz
	)

	err := s.pool.QueryRow(ctx, query, tokenHash).Scan(
		&sessWithUser.Session.ID,
		&sessWithUser.Session.TokenHash,
		&sessWithUser.Session.UserID,
		&sessWithUser.Session.IPAddress,
		&sessWithUser.Session.UserAgent,
		&expiresAt,
		&sessCreated,
		&lastSeenAt,
		&sessWithUser.User.Email,
		&sessWithUser.User.Username,
		&sessWithUser.User.Name,
		&sessWithUser.User.Phone,
		&sessWithUser.User.Address,
		&sessWithUser.User.GSTNumber,
		&sessWithUser.User.Role,
		&sessWithUser.User.Active,
		&sessWithUser.User.TotpEnabled,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sessWithUser.User.ID = sessWithUser.Session.UserID
	if expiresAt.Valid {
		sessWithUser.Session.ExpiresAt = expiresAt.Time
	}
	if sessCreated.Valid {
		sessWithUser.Session.CreatedAt = sessCreated.Time
	}
	if lastSeenAt.Valid {
		sessWithUser.Session.LastSeenAt = lastSeenAt.Time
	}
	return &sessWithUser, nil
}

// TouchSession updates the session's last_seen_at and sets a new expiration timestamp.
func (s *Store) TouchSession(ctx context.Context, tokenHash string, newExpiry time.Time) error {
	_, err := s.queries.TouchSession(ctx, sqlcgen.TouchSessionParams{
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: newExpiry, Valid: true},
	})
	return err
}

// DeleteSession revokes a session by its token hash.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.queries.DeleteSession(ctx, tokenHash)
	return err
}

// DeleteOtherSessions invalidates all sessions for a user except the given token hash.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	_, err := s.queries.DeleteOtherSessions(ctx, sqlcgen.DeleteOtherSessionsParams{
		UserID:    userID,
		TokenHash: tokenHash,
	})
	return err
}

// DeleteExpiredSessions cleans up sessions that have passed their expiration time.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	return s.queries.DeleteExpiredSessions(ctx)
}

// ListUsers returns a paginated summary of platform users and total user count.
func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]domain.UserSummary, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	const countQuery = `SELECT COUNT(*)::int FROM users;`
	var total int
	if err := s.pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	const listQuery = `
SELECT 
    u.id, u.email, u.username, u.name, u.role, u.active, u.phone,
    u.max_ips, u.broker_ip_limits,
    u.created_at, u.updated_at,
    COUNT(DISTINCT a.id)::int AS account_count,
    COUNT(DISTINCT s.id)::int AS session_count
FROM users u
LEFT JOIN accounts a ON a.user_id = u.id AND a.deleted_at IS NULL
LEFT JOIN sessions s ON s.user_id = u.id AND s.expires_at > now()
GROUP BY u.id
ORDER BY u.created_at DESC
LIMIT $1 OFFSET $2;`

	rows, err := s.pool.Query(ctx, listQuery, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []domain.UserSummary
	for rows.Next() {
		var (
			u                 domain.UserSummary
			brokerLimitsBytes []byte
			createdAt         pgtype.Timestamptz
			updatedAt         pgtype.Timestamptz
		)
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.Username,
			&u.Name,
			&u.Role,
			&u.Active,
			&u.Phone,
			&u.MaxIPs,
			&brokerLimitsBytes,
			&createdAt,
			&updatedAt,
			&u.AccountCount,
			&u.SessionCount,
		); err != nil {
			return nil, 0, err
		}
		if len(brokerLimitsBytes) > 0 {
			var limits map[string]int
			if err := json.Unmarshal(brokerLimitsBytes, &limits); err == nil {
				u.BrokerIPLimits = limits
			}
		}
		if createdAt.Valid {
			u.CreatedAt = createdAt.Time.In(domain.IST)
		}
		if updatedAt.Valid {
			u.UpdatedAt = updatedAt.Time.In(domain.IST)
		}
		result = append(result, u)
	}
	return result, total, rows.Err()
}

// UpdateUserRoleAndStatus updates a user's role, active status, and/or account/IP quotas.
func (s *Store) UpdateUserRoleAndStatus(ctx context.Context, userID uuid.UUID, role string, active *bool, maxIPs *int, brokerIPLimits map[string]int) error {
	var brokerLimitsJSON *string
	if brokerIPLimits != nil {
		data, err := json.Marshal(brokerIPLimits)
		if err != nil {
			return err
		}
		str := string(data)
		brokerLimitsJSON = &str
	}

	const query = `
UPDATE users SET
    role = CASE WHEN $2::text != '' THEN $2::text ELSE role END,
    active = CASE WHEN $3::boolean IS NOT NULL THEN $3::boolean ELSE active END,
    max_ips = CASE WHEN $4::int IS NOT NULL THEN $4::int ELSE max_ips END,
    broker_ip_limits = CASE WHEN $5::text IS NOT NULL THEN $5::jsonb ELSE broker_ip_limits END,
    updated_at = now()
WHERE id = $1;`

	tag, err := s.pool.Exec(ctx, query, userID, role, active, maxIPs, brokerLimitsJSON)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// CheckAccountQuota checks whether user has exceeded global max_ips or broker-specific account quota.
func (s *Store) CheckAccountQuota(ctx context.Context, userID uuid.UUID, broker string) error {
	const userQuery = `
SELECT 
    u.max_ips,
    u.broker_ip_limits,
    COUNT(a.id)::int AS total_used
FROM users u
LEFT JOIN accounts a 
       ON a.user_id = u.id 
      AND a.deleted_at IS NULL
WHERE u.id = $1
GROUP BY u.id, u.max_ips, u.broker_ip_limits;`

	var (
		maxIPs          int
		brokerLimitsRaw []byte
		totalUsed       int
	)
	err := s.pool.QueryRow(ctx, userQuery, userID).Scan(&maxIPs, &brokerLimitsRaw, &totalUsed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	// -1 on max_ips unrestricts all types.
	if maxIPs == -1 {
		return nil
	}

	// Any value >= 0 enforces totalUsed < maxIPs.
	if totalUsed >= maxIPs {
		return domain.ErrAccountQuotaExceeded
	}

	var limits map[string]int
	if len(brokerLimitsRaw) > 0 {
		_ = json.Unmarshal(brokerLimitsRaw, &limits)
	}

	targetBroker := strings.ToLower(strings.TrimSpace(broker))
	if targetBroker == "" {
		targetBroker = "kite"
	}

	brokerLimit := limits[targetBroker]
	generalLimit := limits["general"]

	// For individual types, 0 is 0 IP, not infinity.
	if brokerLimit <= 0 && generalLimit <= 0 {
		return domain.ErrBrokerAccountQuotaExceeded
	}

	const brokerCountsQuery = `
SELECT 
    LOWER(COALESCE(a.broker, 'kite')) AS broker,
    COUNT(a.id)::int AS count
FROM accounts a
WHERE a.user_id = $1 AND a.deleted_at IS NULL
GROUP BY LOWER(COALESCE(a.broker, 'kite'));`

	rows, err := s.pool.Query(ctx, brokerCountsQuery, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var b string
		var cnt int
		if err := rows.Scan(&b, &cnt); err != nil {
			return err
		}
		counts[b] = cnt
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if counts[targetBroker] < brokerLimit {
		return nil
	}

	excessUsed := 0
	for b, count := range counts {
		dedicated := limits[b]
		if count > dedicated {
			excessUsed += (count - dedicated)
		}
	}

	if excessUsed >= generalLimit {
		return domain.ErrBrokerAccountQuotaExceeded
	}

	return nil
}

// ListUserSessions returns active unexpired sessions for a specific user.
func (s *Store) ListUserSessions(ctx context.Context, userID uuid.UUID) ([]domain.Session, error) {
	const query = `
SELECT id, token_hash, user_id, ip_address, user_agent, expires_at, created_at, last_seen_at
FROM sessions
WHERE user_id = $1 AND expires_at > now()
ORDER BY last_seen_at DESC;`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []domain.Session
	for rows.Next() {
		var (
			sess       domain.Session
			expiresAt  pgtype.Timestamptz
			createdAt  pgtype.Timestamptz
			lastSeenAt pgtype.Timestamptz
		)
		if err := rows.Scan(
			&sess.ID,
			&sess.TokenHash,
			&sess.UserID,
			&sess.IPAddress,
			&sess.UserAgent,
			&expiresAt,
			&createdAt,
			&lastSeenAt,
		); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			sess.ExpiresAt = expiresAt.Time.In(domain.IST)
		}
		if createdAt.Valid {
			sess.CreatedAt = createdAt.Time.In(domain.IST)
		}
		if lastSeenAt.Valid {
			sess.LastSeenAt = lastSeenAt.Time.In(domain.IST)
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// RevokeAllUserSessions revokes all sessions for a specific user.
func (s *Store) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) (int64, error) {
	const query = `DELETE FROM sessions WHERE user_id = $1;`
	tag, err := s.pool.Exec(ctx, query, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
