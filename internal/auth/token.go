package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateSessionToken creates a cryptographically secure 32-byte session token
// and returns both the plain hex token (for client cookies) and its SHA-256
// hash (for database storage).
func GenerateSessionToken() (plainToken string, tokenHash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	plainToken = hex.EncodeToString(bytes)
	tokenHash = HashSessionToken(plainToken)
	return plainToken, tokenHash, nil
}

// HashSessionToken computes the SHA-256 hex digest of a plain session token.
func HashSessionToken(plainToken string) string {
	sum := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(sum[:])
}
