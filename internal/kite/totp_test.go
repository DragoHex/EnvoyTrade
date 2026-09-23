package kite_test

import (
	"testing"
	"time"

	"envoytrade/internal/kite"
)

func TestGenerateTOTP_RFC6238Vectors(t *testing.T) {
	// Base32 representation of ASCII "12345678901234567890"
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	tests := []struct {
		unixSeconds int64
		expected    string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}

	for _, tc := range tests {
		code, err := kite.GenerateTOTP(secret, time.Unix(tc.unixSeconds, 0))
		if err != nil {
			t.Fatalf("unix %d: unexpected error: %v", tc.unixSeconds, err)
		}
		if code != tc.expected {
			t.Errorf("unix %d: got %s, want %s", tc.unixSeconds, code, tc.expected)
		}
	}
}

func TestGenerateTOTP_FormattingAndPadding(t *testing.T) {
	// Secret with spaces, lowercase, and missing padding
	rawSecret := "gez dgn bvg y3t qoj qgez dgn bvg y3t qoj q"
	code, err := kite.GenerateTOTP(rawSecret, time.Unix(1234567890, 0))
	if err != nil {
		t.Fatalf("unexpected error with unformatted secret: %v", err)
	}
	if code != "005924" {
		t.Errorf("got %s, want 005924", code)
	}
}

func TestGenerateTOTP_InvalidSecret(t *testing.T) {
	_, err := kite.GenerateTOTP("invalid!secret!123", time.Now())
	if err == nil {
		t.Fatalf("expected error on invalid base32 secret, got nil")
	}
}
