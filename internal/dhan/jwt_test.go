package dhan_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"envoytrade/internal/dhan"
)

func makeTestJWT(header map[string]any, payload map[string]any) string {
	hBytes, _ := json.Marshal(header)
	pBytes, _ := json.Marshal(payload)
	hB64 := base64.RawURLEncoding.EncodeToString(hBytes)
	pB64 := base64.RawURLEncoding.EncodeToString(pBytes)
	return hB64 + "." + pB64 + ".dummy-signature"
}

func TestParseDhanJWT_Positive_ValidToken(t *testing.T) {
	expTime := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	token := makeTestJWT(
		map[string]any{"typ": "JWT", "alg": "HS512"},
		map[string]any{
			"dhanClientId": "1114425611",
			"exp":          expTime.Unix(),
			"iss":          "dhan",
		},
	)

	claims, err := dhan.ParseDhanJWT(token)
	if err != nil {
		t.Fatalf("unexpected error parsing valid Dhan JWT: %v", err)
	}

	if claims.DhanClientID != "1114425611" {
		t.Errorf("DhanClientID = %q, want '1114425611'", claims.DhanClientID)
	}
	if claims.Exp != expTime.Unix() {
		t.Errorf("Exp = %d, want %d", claims.Exp, expTime.Unix())
	}
	if !claims.ExpiresAt.Equal(expTime.UTC()) {
		t.Errorf("ExpiresAt = %v, want %v", claims.ExpiresAt, expTime.UTC())
	}
}

func TestParseDhanJWT_Negative_MalformedTokens(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"single segment", "notajwt"},
		{"two segments only", "header.payload"},
		{"four segments", "a.b.c.d"},
		{"invalid base64 payload", "header.!!!invalid-base64!!!.sig"},
		{"invalid json payload", "header." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".sig"},
		{"missing dhanClientId", makeTestJWT(map[string]any{"typ": "JWT"}, map[string]any{"exp": 1234567890})},
		{"empty dhanClientId", makeTestJWT(map[string]any{"typ": "JWT"}, map[string]any{"dhanClientId": "   "})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := dhan.ParseDhanJWT(tc.token)
			if err == nil {
				t.Fatalf("expected error for token %q, got nil with claims: %+v", tc.token, claims)
			}
		})
	}
}
