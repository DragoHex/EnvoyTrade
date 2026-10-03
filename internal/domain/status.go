package domain

import (
	"errors"
	"strings"
)

// AccountStatus represents the operational status of an account.
type AccountStatus string

const (
	AccountStatusActive AccountStatus = "active"
	AccountStatusError  AccountStatus = "error"
)

// ErrInvalidAccountStatus is returned when an unrecognized account status is provided.
var ErrInvalidAccountStatus = errors.New("invalid account status: must be 'active', 'ok', or 'error'")

// ParseAccountStatus parses and validates an account status string.
// Accepts "active", "ok", or "error". Both "active" and "ok" are normalized to AccountStatusActive.
func ParseAccountStatus(s string) (AccountStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "active", "ok":
		return AccountStatusActive, nil
	case "error":
		return AccountStatusError, nil
	default:
		return "", ErrInvalidAccountStatus
	}
}

// IsValid checks if an AccountStatus is a recognized valid status.
func (s AccountStatus) IsValid() bool {
	return s == AccountStatusActive || s == AccountStatusError
}

// ToAPI returns the client-facing API representation ("ok" or "error").
func (s AccountStatus) ToAPI() string {
	if s == AccountStatusActive {
		return "ok"
	}
	return "error"
}
