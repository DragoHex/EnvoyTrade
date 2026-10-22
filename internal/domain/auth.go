package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Platform user roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
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
	Active       bool
	TotpSecret   *string
	TotpEnabled  bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserSummary provides an administrative overview of a platform user with related stats.
type UserSummary struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Active       bool      `json:"active"`
	Phone        string    `json:"phone,omitempty"`
	AccountCount int       `json:"accountCount"`
	SessionCount int       `json:"sessionCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Session represents an active session identified by the SHA-256 hash of its token.
type Session struct {
	ID         int64     `json:"id"`
	TokenHash  string    `json:"-"`
	UserID     uuid.UUID `json:"userId"`
	IPAddress  *string   `json:"ipAddress,omitempty"`
	UserAgent  *string   `json:"userAgent,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
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

