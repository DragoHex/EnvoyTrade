package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// User represents an authenticated platform user.
type User struct {
	ID           uuid.UUID
	Email        string
	Username     string
	Name         string
	Phone        string
	Address      string
	GSTNumber    string
	PasswordHash string
	Role         string
	TotpSecret   *string
	TotpEnabled  bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session represents an active session identified by the SHA-256 hash of its token.
type Session struct {
	ID         int64
	TokenHash  string
	UserID     uuid.UUID
	IPAddress  *string
	UserAgent  *string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// SessionWithUser is a combined view of a session and its owning user.
type SessionWithUser struct {
	Session Session
	User    User
}

type userCtxKey struct{}

// ContextWithUser returns a new context carrying the authenticated domain.User.
func ContextWithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, user)
}

// UserFromContext extracts the authenticated domain.User from context, if present.
func UserFromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(*User)
	return u, ok && u != nil
}

