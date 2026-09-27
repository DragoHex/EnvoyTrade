package auth_test

import (
	"testing"

	"envoytrade/internal/auth"
)

func TestPasswordHashing(t *testing.T) {
	t.Run("hash and verify positive", func(t *testing.T) {
		password := "SuperSecret123!"
		hash, err := auth.HashPassword(password)
		if err != nil {
			t.Fatalf("unexpected error hashing password: %v", err)
		}
		if hash == "" || hash == password {
			t.Fatalf("expected valid non-empty hash different from password")
		}

		if !auth.CheckPassword(password, hash) {
			t.Fatalf("expected CheckPassword to return true for matching password")
		}
	})

	t.Run("wrong password returns false", func(t *testing.T) {
		password := "CorrectHorseBatteryStaple"
		hash, err := auth.HashPassword(password)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if auth.CheckPassword("WrongPassword", hash) {
			t.Fatalf("expected CheckPassword to return false for wrong password")
		}
	})

	t.Run("empty password or empty hash returns false", func(t *testing.T) {
		if auth.CheckPassword("", "$2a$12$somevalidlengthmockhashvaluehere") {
			t.Fatalf("expected false for empty password")
		}
		if auth.CheckPassword("somepassword", "") {
			t.Fatalf("expected false for empty hash")
		}
	})

	t.Run("malformed hash returns false", func(t *testing.T) {
		if auth.CheckPassword("somepassword", "not_a_bcrypt_hash") {
			t.Fatalf("expected false for malformed hash")
		}
	})

	t.Run("seeded mock user hash matches password123", func(t *testing.T) {
		seededHash := "$2a$12$Ut7Sf7HsyDpwWGURAB4zUOBDdeYELVYMybmk6cCNrUD3nOD5o68ci"
		if !auth.CheckPassword("password123", seededHash) {
			t.Fatalf("expected seeded hash to match password123")
		}
		if auth.CheckPassword("wrongpassword", seededHash) {
			t.Fatalf("expected seeded hash to reject wrong password")
		}
	})
}
