package kotak_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"envoytrade/internal/crypto"
	"envoytrade/internal/domain"
	"envoytrade/internal/kotak"

	"github.com/google/uuid"
)

func TestNormalizeMobileNumber(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"10 digits standard", "9876543210", "+919876543210", false},
		{"10 digits with spaces and dashes", " 9876-543-210 ", "+919876543210", false},
		{"10 digits with parens", "(987) 6543210", "+919876543210", false},
		{"12 digits with 91 prefix", "919876543210", "+919876543210", false},
		{"13 chars with +91 prefix", "+919876543210", "+919876543210", false},
		{"invalid length short", "98765", "", true},
		{"invalid length long", "9876543210123", "", true},
		{"invalid letters", "987654321a", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := kotak.NormalizeMobileNumber(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeMobileNumber(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNextKotakExpiry(t *testing.T) {
	loc := domain.IST
	// Case 1: Before 06:00 AM IST (03:30 AM)
	t1 := time.Date(2026, 10, 29, 3, 30, 0, 0, loc)
	exp1 := kotak.NextKotakExpiry(t1)
	expected1 := time.Date(2026, 10, 29, 6, 0, 0, 0, loc)
	if !exp1.Equal(expected1) {
		t.Errorf("exp1 = %v, want %v", exp1, expected1)
	}

	// Case 2: After 06:00 AM IST (09:30 AM)
	t2 := time.Date(2026, 10, 29, 9, 30, 0, 0, loc)
	exp2 := kotak.NextKotakExpiry(t2)
	expected2 := time.Date(2026, 10, 30, 6, 0, 0, 0, loc)
	if !exp2.Equal(expected2) {
		t.Errorf("exp2 = %v, want %v", exp2, expected2)
	}
}

func TestNextRenewerRunTime(t *testing.T) {
	loc := domain.IST
	// Case 1: Before 06:30 AM IST (05:00 AM)
	t1 := time.Date(2026, 10, 29, 5, 0, 0, 0, loc)
	exp1 := kotak.NextRenewerRunTime(t1)
	expected1 := time.Date(2026, 10, 29, 6, 30, 0, 0, loc)
	if !exp1.Equal(expected1) {
		t.Errorf("exp1 = %v, want %v", exp1, expected1)
	}

	// Case 2: After 06:30 AM IST (07:00 AM)
	t2 := time.Date(2026, 10, 29, 7, 0, 0, 0, loc)
	exp2 := kotak.NextRenewerRunTime(t2)
	expected2 := time.Date(2026, 10, 30, 6, 30, 0, 0, loc)
	if !exp2.Equal(expected2) {
		t.Errorf("exp2 = %v, want %v", exp2, expected2)
	}
}

func TestTradeSessionSerialization(t *testing.T) {
	orig := &kotak.TradeSessionJSON{
		Token:        "tok-123",
		Sid:          "sid-456",
		BaseURL:      "https://tradeapi.kotak.com",
		FeedURL:      "wss://tradeapi.kotak.com/feed",
		RTURL:        "wss://tradeapi.kotak.com/rt",
		GreetingName: "Alice",
		UCC:          "K12345",
	}

	raw, err := kotak.SerializeTradeSession(orig)
	if err != nil {
		t.Fatalf("SerializeTradeSession error: %v", err)
	}

	parsed, err := kotak.DeserializeTradeSession(raw)
	if err != nil {
		t.Fatalf("DeserializeTradeSession error: %v", err)
	}

	if parsed.Token != orig.Token || parsed.Sid != orig.Sid || parsed.BaseURL != orig.BaseURL {
		t.Errorf("parsed session mismatch: %+v vs %+v", parsed, orig)
	}
}

// Mock RenewerStore
type mockRenewerStore struct {
	accounts         map[uuid.UUID]domain.Account
	authInfo         map[uuid.UUID]domain.AccountAuthInfo
	proxyIPs         map[string]domain.ProxyIP
	tokensUpdated    map[uuid.UUID]string
	authStatusRecord map[uuid.UUID]string
	authErrorRecord  map[uuid.UUID]string
}

func newMockRenewerStore() *mockRenewerStore {
	return &mockRenewerStore{
		accounts:         make(map[uuid.UUID]domain.Account),
		authInfo:         make(map[uuid.UUID]domain.AccountAuthInfo),
		proxyIPs:         make(map[string]domain.ProxyIP),
		tokensUpdated:    make(map[uuid.UUID]string),
		authStatusRecord: make(map[uuid.UUID]string),
		authErrorRecord:  make(map[uuid.UUID]string),
	}
}

func (m *mockRenewerStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	var res []domain.Account
	for _, a := range m.accounts {
		res = append(res, a)
	}
	return res, nil
}

func (m *mockRenewerStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	info, ok := m.authInfo[id]
	if !ok {
		return domain.AccountAuthInfo{}, domain.ErrNotFound
	}
	return info, nil
}

func (m *mockRenewerStore) ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error) {
	p, ok := m.proxyIPs[ip]
	if !ok {
		return domain.ProxyIP{}, domain.ErrNotFound
	}
	return p, nil
}

func (m *mockRenewerStore) SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	m.tokensUpdated[id] = token
	m.authStatusRecord[id] = authStatus
	m.authErrorRecord[id] = authError
	return nil
}

func TestRenewer_Execution(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-key-32-bytes-long-for-test!")

	store := newMockRenewerStore()
	accountID := uuid.New()

	encMPIN, err := crypto.Encrypt("123456")
	if err != nil {
		t.Fatalf("encrypt MPIN failed: %v", err)
	}
	encTOTP, err := crypto.Encrypt("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encrypt TOTP failed: %v", err)
	}

	store.accounts[accountID] = domain.Account{
		ID:              accountID,
		Broker:          "kotakneo",
		BrokerAccountID: "UCC123",
		Status:          "active",
	}

	store.authInfo[accountID] = domain.AccountAuthInfo{
		BrokerAccountID:     "UCC123",
		ApiKey:              "9876543210",
		ApiSecret:           "consumer-key-123",
		EncryptedPassword:   encMPIN,
		EncryptedTotpSecret: encTOTP,
		AuthStatus:          "authenticated",
	}

	t.Run("Positive: RenewAccount Success", func(t *testing.T) {
		var tokenRefreshedCalled bool
		renewer := kotak.NewRenewer(store, nil)
		renewer.LoginFunc = func(ctx context.Context, creds kotak.HeadlessLoginCredentials) (*kotak.TradeSessionJSON, error) {
			if creds.UCC != "UCC123" || creds.MPIN != "123456" {
				return nil, errors.New("invalid credentials")
			}
			return &kotak.TradeSessionJSON{
				Token:   "new-trade-token",
				Sid:     "new-trade-sid",
				BaseURL: "https://mock.kotak.com",
			}, nil
		}
		renewer.OnTokenRefreshed = func(ctx context.Context, id uuid.UUID) error {
			if id == accountID {
				tokenRefreshedCalled = true
			}
			return nil
		}

		err := renewer.RenewAccount(context.Background(), accountID)
		if err != nil {
			t.Fatalf("RenewAccount failed: %v", err)
		}

		if !tokenRefreshedCalled {
			t.Errorf("expected OnTokenRefreshed to be called")
		}

		if store.authStatusRecord[accountID] != "authenticated" {
			t.Errorf("auth status = %s, want authenticated", store.authStatusRecord[accountID])
		}

		sess, err := kotak.DeserializeTradeSession(store.tokensUpdated[accountID])
		if err != nil || sess.Token != "new-trade-token" {
			t.Errorf("saved token mismatch: %v", err)
		}
	})

	t.Run("Negative: RenewAccount Missing MPIN/TOTP", func(t *testing.T) {
		badAccID := uuid.New()
		store.authInfo[badAccID] = domain.AccountAuthInfo{
			BrokerAccountID: "BAD1",
		}
		renewer := kotak.NewRenewer(store, nil)
		err := renewer.RenewAccount(context.Background(), badAccID)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if store.authStatusRecord[badAccID] != "unauthenticated" {
			t.Errorf("expected unauthenticated, got %s", store.authStatusRecord[badAccID])
		}
	})

	t.Run("Negative: Permanent Auth Expiry", func(t *testing.T) {
		renewer := kotak.NewRenewer(store, nil)
		renewer.LoginFunc = func(ctx context.Context, creds kotak.HeadlessLoginCredentials) (*kotak.TradeSessionJSON, error) {
			return nil, domain.ErrAuthExpired
		}

		err := renewer.RenewAccount(context.Background(), accountID)
		if err == nil || !errors.Is(err, domain.ErrAuthExpired) {
			t.Fatalf("expected ErrAuthExpired, got %v", err)
		}

		if store.authStatusRecord[accountID] != "unauthenticated" {
			t.Errorf("auth status = %s, want unauthenticated", store.authStatusRecord[accountID])
		}
	})
}
