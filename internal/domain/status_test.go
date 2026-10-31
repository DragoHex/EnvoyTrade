package domain_test

import (
	"strings"
	"testing"

	"envoytrade/internal/domain"
)

func TestParseAccountStatus(t *testing.T) {
	tests := []struct {
		input       string
		wantStatus  domain.AccountStatus
		wantToAPI   string
		expectError bool
	}{
		{input: "active", wantStatus: domain.AccountStatusActive, wantToAPI: "ok", expectError: false},
		{input: "ACTIVE", wantStatus: domain.AccountStatusActive, wantToAPI: "ok", expectError: false},
		{input: "  active  ", wantStatus: domain.AccountStatusActive, wantToAPI: "ok", expectError: false},
		{input: "ok", wantStatus: domain.AccountStatusActive, wantToAPI: "ok", expectError: false},
		{input: "OK", wantStatus: domain.AccountStatusActive, wantToAPI: "ok", expectError: false},
		{input: "error", wantStatus: domain.AccountStatusError, wantToAPI: "error", expectError: false},
		{input: "ERROR", wantStatus: domain.AccountStatusError, wantToAPI: "error", expectError: false},
		{input: "invalid", expectError: true},
		{input: "", expectError: true},
		{input: "paused", expectError: true},
		{input: "disabled", expectError: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := domain.ParseAccountStatus(tc.input)
			if tc.expectError {
				if err == nil {
					t.Fatalf("ParseAccountStatus(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAccountStatus(%q): unexpected error: %v", tc.input, err)
			}
			if got != tc.wantStatus {
				t.Errorf("ParseAccountStatus(%q) = %q, want %q", tc.input, got, tc.wantStatus)
			}
			if !got.IsValid() {
				t.Errorf("got.IsValid() = false, want true for %q", got)
			}
			if got.ToAPI() != tc.wantToAPI {
				t.Errorf("got.ToAPI() = %q, want %q", got.ToAPI(), tc.wantToAPI)
			}
		})
	}
}

func TestAccountStatus_IsValid(t *testing.T) {
	if !domain.AccountStatusActive.IsValid() {
		t.Errorf("AccountStatusActive.IsValid() = false, want true")
	}
	if !domain.AccountStatusError.IsValid() {
		t.Errorf("AccountStatusError.IsValid() = false, want true")
	}
	if domain.AccountStatus("unknown").IsValid() {
		t.Errorf("AccountStatus('unknown').IsValid() = true, want false")
	}
}

func TestRequiresDailySession(t *testing.T) {
	tests := []struct {
		broker string
		want   bool
	}{
		{"zerodha", true},
		{"ZERODHA", true},
		{"kite", true},
		{"Kite", true},
		{"  kite  ", true},
		{"dhan", false},
		{"DHAN", false},
		{"testbroker", false},
		{"angelone", false},
		{"shoonya", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.broker, func(t *testing.T) {
			got := domain.RequiresDailySession(tc.broker)
			if got != tc.want {
				t.Errorf("RequiresDailySession(%q) = %v, want %v", tc.broker, got, tc.want)
			}
		})
	}
}

func TestSanitizeAuthError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"clean token expired with IST", "Token expired at 06:00 IST", "Token expired at 06:00 IST"},
		{"clean token expired session inactive", "Token expired / session inactive", "Token expired / session inactive"},
		{"clean invalid credentials", "invalid credentials", "invalid credentials"},
		{"dhan DH-1111 error", "dhan sync failed: get dhan holdings: dhan api error [DH-1111] (HTTP 500): No holdings available", "Broker server temporarily unavailable"},
		{"missing request token connect login body", "missing request_token from connect/login (HTTP 500, sess_id=abc, body: \"error\")", "Broker server temporarily unavailable"},
		{"connect login request with URL and dial tcp", "connect login request: Post \"https://kite.zerodha.com/connect/login\": dial tcp 1.2.3.4: connect: connection refused", "Broker server temporarily unavailable"},
		{"session token request with 401", "session token request: Post \"https://api.kite.trade/session/token\": 401 Unauthorized", "Broker session expired or token invalid"},
		{"kite login error with incorrect password", "kite login error: Incorrect password or user id", "Invalid broker credentials or password"},
		{"kite 2fa error with invalid totp", "kite 2fa error: Invalid TOTP code (External TOTP is not enabled)", "Invalid 2FA TOTP code or external TOTP not enabled"},
		{"kotak neo api error user not found for ucc", "kotak-neo api error [424]: HTTP 424: {\"error\":[{\"code\":\"10564\",\"message\":\"User not found for given ucc\"}]}", "Invalid Kotak Neo Client ID (UCC) or Trade API not enabled for this user"},
		{"kotak neo invalid mobile number", "kotak-neo api error [400]: HTTP 400: {\"error\":[{\"code\":\"400\",\"message\":\"Invalid field 'MobileNumber'\"}]}", "Invalid Kotak Neo registered mobile number"},
		{"get dhan fund limits 401", "get dhan fund limits: 401 unauthorized token expired", "Broker session expired or token invalid"},
		{"unknown backend failure with endpoint", "unknown backend failure in POST /api/v2/user/margins", "Authentication failed with broker"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.SanitizeAuthError(tc.input)
			if got != tc.want {
				t.Errorf("SanitizeAuthError(%q) = %q, want %q", tc.input, got, tc.want)
			}
			// Invariant: No backend API calls or URLs allowed in sanitized output
			lower := strings.ToLower(got)
			if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") ||
				strings.Contains(lower, "post ") || strings.Contains(lower, "get ") ||
				strings.Contains(lower, "/api/") || strings.Contains(lower, "connect/login") ||
				strings.Contains(lower, "session/token") || strings.Contains(lower, "dial tcp") {
				t.Errorf("SanitizeAuthError(%q) exposed backend API details: %q", tc.input, got)
			}
		})
	}
}
