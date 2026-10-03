package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// HashPassword hashes a raw password using bcrypt with cost 12.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword verifies that a raw password matches a stored bcrypt hash.
func CheckPassword(password, hash string) bool {
	if password == "" || hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
