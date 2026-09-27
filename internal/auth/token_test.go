package auth_test

import (
	"encoding/hex"
	"testing"

	"envoytrade/internal/auth"
)

func TestSessionTokens(t *testing.T) {
	t.Run("generate token produces valid hex and distinct hash", func(t *testing.T) {
		plainToken, tokenHash, err := auth.GenerateSessionToken()
		if err != nil {
			t.Fatalf("unexpected error generating token: %v", err)
		}

		if len(plainToken) != 64 { // 32 bytes hex encoded = 64 characters
			t.Fatalf("expected 64 characters plain token, got %d", len(plainToken))
		}
		if len(tokenHash) != 64 { // SHA-256 hex encoded = 64 characters
			t.Fatalf("expected 64 characters token hash, got %d", len(tokenHash))
		}
		if plainToken == tokenHash {
			t.Fatalf("plain token and token hash should not be equal")
		}

		if _, err := hex.DecodeString(plainToken); err != nil {
			t.Fatalf("plain token must be valid hex: %v", err)
		}
		if _, err := hex.DecodeString(tokenHash); err != nil {
			t.Fatalf("token hash must be valid hex: %v", err)
		}
	})

	t.Run("hash function is deterministic", func(t *testing.T) {
		plainToken, expectedHash, err := auth.GenerateSessionToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		computedHash := auth.HashSessionToken(plainToken)
		if computedHash != expectedHash {
			t.Fatalf("HashSessionToken(%q) = %q; want %q", plainToken, computedHash, expectedHash)
		}
	})

	t.Run("empty token returns empty or valid hash without crashing", func(t *testing.T) {
		hash := auth.HashSessionToken("")
		if hash == "" {
			t.Fatalf("hash of empty string should be SHA-256 of empty string")
		}
	})

	t.Run("consecutive tokens are unique", func(t *testing.T) {
		token1, hash1, err := auth.GenerateSessionToken()
		if err != nil {
			t.Fatalf("token1 err: %v", err)
		}
		token2, hash2, err := auth.GenerateSessionToken()
		if err != nil {
			t.Fatalf("token2 err: %v", err)
		}

		if token1 == token2 {
			t.Fatalf("tokens must be cryptographically random and unique")
		}
		if hash1 == hash2 {
			t.Fatalf("hashes must be distinct for distinct tokens")
		}
	})
}
