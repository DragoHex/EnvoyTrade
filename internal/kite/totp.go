package kite

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// GenerateTOTP generates a standard 6-digit TOTP code for the given base32 secret and time,
// complying with RFC 6238 and RFC 4226 (30s time step, HMAC-SHA1).
func GenerateTOTP(base32Secret string, t time.Time) (string, error) {
	// Normalize secret: remove spaces, hyphens, and convert to upper case
	cleaned := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(base32Secret, " ", ""), "-", ""))
	if cleaned == "" {
		return "", fmt.Errorf("empty totp secret")
	}

	// Add Base32 padding if missing
	if rem := len(cleaned) % 8; rem != 0 {
		cleaned += strings.Repeat("=", 8-rem)
	}

	key, err := base32.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("base32 decode secret: %w", err)
	}

	// Calculate 30-second time steps since Unix epoch
	step := uint64(t.Unix() / 30)

	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, step)

	mac := hmac.New(sha1.New, key)
	mac.Write(msg)
	digest := mac.Sum(nil)

	// Dynamic truncation per RFC 4226 §5.4
	offset := digest[len(digest)-1] & 0x0f
	code := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff

	// Truncate to 6 digits
	otp := code % 1000000

	return fmt.Sprintf("%06d", otp), nil
}
