package sdk

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// ComputeChecksum calculates the Kite-compliant SHA-256 postback checksum:
// hex(sha256(order_id + order_timestamp + api_secret))
func ComputeChecksum(orderID, orderTimestamp, apiSecret string) string {
	sum := sha256.Sum256([]byte(orderID + orderTimestamp + apiSecret))
	return hex.EncodeToString(sum[:])
}

// VerifyChecksum checks whether a received checksum matches the expected SHA-256 hash using constant-time comparison.
func VerifyChecksum(apiSecret, orderID, orderTimestamp, checksum string) bool {
	expected := ComputeChecksum(orderID, orderTimestamp, apiSecret)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(checksum)) == 1
}
