package domain_test

import (
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
