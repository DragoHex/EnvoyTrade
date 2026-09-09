package callback_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"envoytrade/internal/kite/callback"
)

func sumOf(orderID, orderTimestamp, apiSecret string) string {
	sum := sha256.Sum256([]byte(orderID + orderTimestamp + apiSecret))
	return hex.EncodeToString(sum[:])
}

func TestVerifyChecksum_MatchingChecksumIsValid(t *testing.T) {
	got := sumOf("151220000000000", "2021-01-01 12:00:00", "secret123")
	if !callback.VerifyChecksum("secret123", "151220000000000", "2021-01-01 12:00:00", got) {
		t.Fatal("want valid, got invalid")
	}
}

func TestVerifyChecksum_WrongChecksumIsInvalid(t *testing.T) {
	if callback.VerifyChecksum("secret123", "151220000000000", "2021-01-01 12:00:00", "deadbeef") {
		t.Fatal("want invalid, got valid")
	}
}

func TestVerifyChecksum_WrongSecretIsInvalid(t *testing.T) {
	got := sumOf("151220000000000", "2021-01-01 12:00:00", "secret123")
	if callback.VerifyChecksum("wrong-secret", "151220000000000", "2021-01-01 12:00:00", got) {
		t.Fatal("want invalid, got valid")
	}
}

func TestVerifyChecksum_EmptyChecksumIsInvalid(t *testing.T) {
	if callback.VerifyChecksum("secret123", "151220000000000", "2021-01-01 12:00:00", "") {
		t.Fatal("want invalid, got valid")
	}
}
