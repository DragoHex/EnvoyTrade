package dhan

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"envoytrade/internal/domain"
)

// DhanClaims holds essential claims decoded from a Dhan JWT access token.
type DhanClaims struct {
	DhanClientID string    `json:"dhanClientId"`
	ExpiresAt    time.Time `json:"-"`
	Exp          int64     `json:"exp"`
	Iss          string    `json:"iss"`
}

// ParseDhanJWT decodes the unencrypted claims payload from a Dhan access token.
// Does not verify HMAC signature (verified upstream by DhanHQ servers).
func ParseDhanJWT(token string) (*DhanClaims, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid jwt: expected 3 dot-separated segments")
	}

	payloadSegment := parts[1]
	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadSegment)
	if err != nil {
		payloadBytes, err = base64.URLEncoding.DecodeString(payloadSegment)
		if err != nil {
			return nil, fmt.Errorf("decode jwt payload: %w", err)
		}
	}

	var claims DhanClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal jwt payload: %w", err)
	}
	if strings.TrimSpace(claims.DhanClientID) == "" {
		return nil, errors.New("jwt missing dhanClientId claim")
	}
	if claims.Exp > 0 {
		claims.ExpiresAt = time.Unix(claims.Exp, 0).In(domain.IST)
	}
	return &claims, nil
}
