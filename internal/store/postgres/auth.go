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
	row, err := s.queries.GetSessionWithUser(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.SessionWithUser{
		Session: domain.Session{
			ID:         row.SessionID,
			TokenHash:  row.TokenHash,
			UserID:     row.UserID,
			IPAddress:  row.IpAddress,
			UserAgent:  row.UserAgent,
			ExpiresAt:  row.ExpiresAt.Time,
			CreatedAt:  row.SessionCreatedAt.Time,
			LastSeenAt: row.LastSeenAt.Time,
		},
		User: domain.User{
			ID:          row.UserID,
			Email:       row.Email,
			Username:    row.Username,
			Name:        row.Name,
			Phone:       row.Phone,
			Address:     row.Address,
			GSTNumber:   row.GstNumber,
			Role:        row.Role,
			TotpEnabled: row.TotpEnabled,
		},
	}, nil
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
