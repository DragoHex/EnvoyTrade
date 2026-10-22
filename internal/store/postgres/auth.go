package postgres

import (
	"context"
	"errors"
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
	row, err := s.queries.GetUserByID(ctx, id)
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
			u         domain.UserSummary
			createdAt pgtype.Timestamptz
			updatedAt pgtype.Timestamptz
		)
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.Username,
			&u.Name,
			&u.Role,
			&u.Active,
			&u.Phone,
			&createdAt,
			&updatedAt,
			&u.AccountCount,
			&u.SessionCount,
		); err != nil {
			return nil, 0, err
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

// UpdateUserRoleAndStatus updates a user's role and/or active status.
func (s *Store) UpdateUserRoleAndStatus(ctx context.Context, userID uuid.UUID, role string, active *bool) error {
	const query = `
UPDATE users SET
    role = CASE WHEN $2::text != '' THEN $2::text ELSE role END,
    active = CASE WHEN $3::boolean IS NOT NULL THEN $3::boolean ELSE active END,
    updated_at = now()
WHERE id = $1;`

	tag, err := s.pool.Exec(ctx, query, userID, role, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
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
