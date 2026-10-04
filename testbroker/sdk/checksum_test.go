package sdk

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestComputeChecksum(t *testing.T) {
	orderID := "260905000000001"
	orderTimestamp := "2026-09-05 10:30:00"
	apiSecret := "my_test_secret"

	expectedHash := sha256.Sum256([]byte(orderID + orderTimestamp + apiSecret))
	expected := hex.EncodeToString(expectedHash[:])

	actual := ComputeChecksum(orderID, orderTimestamp, apiSecret)
	if actual != expected {
		t.Errorf("ComputeChecksum() = %s, want %s", actual, expected)
	}
}

func TestVerifyChecksum(t *testing.T) {
	orderID := "260905000000001"
	orderTimestamp := "2026-09-05 10:30:00"
	apiSecret := "my_test_secret"

	validChecksum := ComputeChecksum(orderID, orderTimestamp, apiSecret)

	t.Run("Valid Checksum", func(t *testing.T) {
		if !VerifyChecksum(apiSecret, orderID, orderTimestamp, validChecksum) {
			t.Error("VerifyChecksum failed for valid inputs")
		}
	})

	t.Run("Wrong Secret", func(t *testing.T) {
		if VerifyChecksum("wrong_secret", orderID, orderTimestamp, validChecksum) {
			t.Error("VerifyChecksum should fail for wrong secret")
		}
	})

	t.Run("Wrong OrderID", func(t *testing.T) {
		if VerifyChecksum(apiSecret, "different_id", orderTimestamp, validChecksum) {
			t.Error("VerifyChecksum should fail for wrong orderID")
		}
	})

	t.Run("Wrong Timestamp", func(t *testing.T) {
		if VerifyChecksum(apiSecret, orderID, "2026-09-05 10:30:01", validChecksum) {
			t.Error("VerifyChecksum should fail for wrong timestamp")
		}
	})

	t.Run("Tampered Checksum", func(t *testing.T) {
		if VerifyChecksum(apiSecret, orderID, orderTimestamp, "deadbeef12345678") {
			t.Error("VerifyChecksum should fail for tampered checksum")
		}
	})
}
