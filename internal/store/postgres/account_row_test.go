package postgres

import (
	"strings"
	"testing"
	"time"

	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAccountFromRow_MultiBrokerTokenExpiration(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	future := pgtype.Timestamptz{Time: now.Add(4 * time.Hour), Valid: true}
	past := pgtype.Timestamptz{Time: now.Add(-2 * time.Hour), Valid: true}
	nullExpiry := pgtype.Timestamptz{Valid: false}

	tests := []struct {
		name           string
		broker         string
		authStatus     string
		authError      string
		tokenExpiresAt pgtype.Timestamptz
		accountStatus  string
		wantAuthStatus string
		wantAuthError  string
		wantStatus     string
	}{
		{
			name:           "zerodha with active future token",
			broker:         "zerodha",
			authStatus:     "authenticated",
			tokenExpiresAt: future,
			accountStatus:  "active",
			wantAuthStatus: "authenticated",
			wantAuthError:  "",
			wantStatus:     "ok",
		},
		{
			name:           "zerodha with expired token",
			broker:         "zerodha",
			authStatus:     "authenticated",
			tokenExpiresAt: past,
			accountStatus:  "active",
			wantAuthStatus: "error",
			wantAuthError:  "Token expired at",
			wantStatus:     "error",
		},
		{
			name:           "zerodha with null token expiry (requires daily session)",
			broker:         "zerodha",
			authStatus:     "authenticated",
			tokenExpiresAt: nullExpiry,
			accountStatus:  "active",
			wantAuthStatus: "error",
			wantAuthError:  "Token expired / session inactive",
			wantStatus:     "error",
		},
		{
			name:           "kite alias with null token expiry (requires daily session)",
			broker:         "kite",
			authStatus:     "authenticated",
			tokenExpiresAt: nullExpiry,
			accountStatus:  "active",
			wantAuthStatus: "error",
			wantAuthError:  "Token expired / session inactive",
			wantStatus:     "error",
		},
		{
			name:           "dhan with active future token",
			broker:         "dhan",
			authStatus:     "authenticated",
			tokenExpiresAt: future,
			accountStatus:  "active",
			wantAuthStatus: "authenticated",
			wantAuthError:  "",
			wantStatus:     "ok",
		},
		{
			name:           "dhan with expired token",
			broker:         "dhan",
			authStatus:     "authenticated",
			tokenExpiresAt: past,
			accountStatus:  "active",
			wantAuthStatus: "error",
			wantAuthError:  "Token expired at",
			wantStatus:     "error",
		},
		{
			name:           "dhan with null token expiry remains authenticated",
			broker:         "dhan",
			authStatus:     "authenticated",
			tokenExpiresAt: nullExpiry,
			accountStatus:  "active",
			wantAuthStatus: "authenticated",
			wantAuthError:  "",
			wantStatus:     "ok",
		},
		{
			name:           "testbroker with null token expiry remains authenticated",
			broker:         "testbroker",
			authStatus:     "authenticated",
			tokenExpiresAt: nullExpiry,
			accountStatus:  "active",
			wantAuthStatus: "authenticated",
			wantAuthError:  "",
			wantStatus:     "ok",
		},
		{
			name:           "any broker in pending state reflects pending status",
			broker:         "dhan",
			authStatus:     "pending",
			tokenExpiresAt: future,
			accountStatus:  "pending",
			wantAuthStatus: "pending",
			wantAuthError:  "",
			wantStatus:     "pending",
		},
		{
			name:           "any broker in error state preserves existing error",
			broker:         "dhan",
			authStatus:     "error",
			authError:      "invalid api key",
			tokenExpiresAt: future,
			accountStatus:  "error",
			wantAuthStatus: "error",
			wantAuthError:  "invalid api key",
			wantStatus:     "error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := sqlcgen.AccountsRow{
				ID:             uuid.New(),
				Name:           "Test Account",
				Role:           "master",
				Broker:         tc.broker,
				BrokerUserID:   "USER01",
				Status:         tc.accountStatus,
				AuthStatus:     tc.authStatus,
				AuthError:      tc.authError,
				TokenExpiresAt: tc.tokenExpiresAt,
				Active:         true,
			}

			account := accountFromRow(row, now)

			if account.AuthStatus != tc.wantAuthStatus {
				t.Errorf("AuthStatus = %q, want %q", account.AuthStatus, tc.wantAuthStatus)
			}
			if tc.wantAuthError != "" && !strings.Contains(account.AuthError, tc.wantAuthError) {
				t.Errorf("AuthError = %q, want containing %q", account.AuthError, tc.wantAuthError)
			}
			if tc.wantAuthError == "" && account.AuthError != "" {
				t.Errorf("AuthError = %q, want empty", account.AuthError)
			}
			if account.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", account.Status, tc.wantStatus)
			}
		})
	}
}
