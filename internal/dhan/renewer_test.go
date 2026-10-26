package dhan_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/dhan"
	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan"
	"github.com/google/uuid"
)

// =============================================================================
// Test Schedule Calculation (NextFixedRunTime)
// Tests positive cases, boundary conditions, edge cases, and timezone handling.
// =============================================================================

func TestNextFixedRunTime_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		// Positive Cases
		{
			name:     "positive: middle of the day schedules 00:15 tomorrow",
			input:    time.Date(2026, 10, 26, 14, 0, 0, 0, domain.IST),
			expected: time.Date(2026, 10, 27, 0, 15, 0, 0, domain.IST),
		},
		{
			name:     "positive: early morning schedules 06:00 today",
			input:    time.Date(2026, 10, 26, 3, 0, 0, 0, domain.IST),
			expected: time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
		},
		{
			name:     "positive: right after midnight schedules 00:15 today",
			input:    time.Date(2026, 10, 26, 0, 5, 0, 0, domain.IST),
			expected: time.Date(2026, 10, 26, 0, 15, 0, 0, domain.IST),
		},

		// Edge Cases: Exact boundary timestamps
		{
			name:     "edge: exactly 00:15:00.000 schedules 06:00 today",
			input:    time.Date(2026, 10, 26, 0, 15, 0, 0, domain.IST),
			expected: time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
		},
		{
			name:     "edge: 00:15:00.001 schedules 06:00 today",
			input:    time.Date(2026, 10, 26, 0, 15, 0, 1000, domain.IST),
			expected: time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
		},
		{
			name:     "edge: 00:14:59.999 schedules 00:15 today",
			input:    time.Date(2026, 10, 26, 0, 14, 59, 999999000, domain.IST),
			expected: time.Date(2026, 10, 26, 0, 15, 0, 0, domain.IST),
		},
		{
			name:     "edge: exactly 06:00:00.000 schedules 00:15 tomorrow",
			input:    time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
			expected: time.Date(2026, 10, 27, 0, 15, 0, 0, domain.IST),
		},
		{
			name:     "edge: 06:00:00.001 schedules 00:15 tomorrow",
			input:    time.Date(2026, 10, 26, 6, 0, 0, 1000, domain.IST),
			expected: time.Date(2026, 10, 27, 0, 15, 0, 0, domain.IST),
		},
		{
			name:     "edge: 05:59:59.999 schedules 06:00 today",
			input:    time.Date(2026, 10, 26, 5, 59, 59, 999999000, domain.IST),
			expected: time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
		},
		{
			name:     "edge: end of day 23:59:59 schedules 00:15 tomorrow",
			input:    time.Date(2026, 10, 26, 23, 59, 59, 999999000, domain.IST),
			expected: time.Date(2026, 10, 27, 0, 15, 0, 0, domain.IST),
		},

		// Edge Cases: Month and Year roll
		{
			name:     "edge: month roll on Oct 31 18:00 schedules Nov 1 00:15",
			input:    time.Date(2026, 10, 31, 18, 0, 0, 0, domain.IST),
			expected: time.Date(2026, 11, 1, 0, 15, 0, 0, domain.IST),
		},
		{
			name:     "edge: year roll on Dec 31 20:00 schedules Jan 1 00:15",
			input:    time.Date(2026, 12, 31, 20, 0, 0, 0, domain.IST),
			expected: time.Date(2027, 1, 1, 0, 15, 0, 0, domain.IST),
		},

		// Timezone normalization (UTC input)
		{
			name: "timezone: UTC input 18:45 UTC (00:15 IST next day) schedules 06:00 IST next day",
			// 2026-10-25 18:45:00 UTC = 2026-10-26 00:15:00 IST
			input:    time.Date(2026, 10, 25, 18, 45, 0, 0, time.UTC),
			expected: time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
		},
		{
			name: "timezone: UTC input 00:00 UTC (05:30 IST) schedules 06:00 IST today",
			// 2026-10-26 00:00:00 UTC = 2026-10-26 05:30:00 IST
			input:    time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
			expected: time.Date(2026, 10, 26, 6, 0, 0, 0, domain.IST),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dhan.NextFixedRunTime(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("NextFixedRunTime(%v) = %v, want %v", tt.input, got, tt.expected)
			}
			if got.Location() != domain.IST {
				t.Errorf("NextFixedRunTime location = %v, want domain.IST", got.Location())
			}
		})
	}
}

// =============================================================================
// Mock Store for Renewer Tests
// =============================================================================

type fakeRenewerStore struct {
	mu           sync.Mutex
	accounts     []domain.Account
	accountsErr  error
	authInfos    map[uuid.UUID]domain.AccountAuthInfo
	authInfoErrs map[uuid.UUID]error
	proxyIPs     map[string]domain.ProxyIP
	proxyIPErrs  map[string]error

	savedTokens  map[uuid.UUID]string
	savedExpirys map[uuid.UUID]*time.Time
	savedStatus  map[uuid.UUID]string
	savedErrors  map[uuid.UUID]string
	saveTokenErr error
}

func newFakeRenewerStore() *fakeRenewerStore {
	return &fakeRenewerStore{
		authInfos:    make(map[uuid.UUID]domain.AccountAuthInfo),
		authInfoErrs: make(map[uuid.UUID]error),
		proxyIPs:     make(map[string]domain.ProxyIP),
		proxyIPErrs:  make(map[string]error),
		savedTokens:  make(map[uuid.UUID]string),
		savedExpirys: make(map[uuid.UUID]*time.Time),
		savedStatus:  make(map[uuid.UUID]string),
		savedErrors:  make(map[uuid.UUID]string),
	}
}

func (s *fakeRenewerStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountsErr != nil {
		return nil, s.accountsErr
	}
	return s.accounts, nil
}

func (s *fakeRenewerStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.authInfoErrs[id]; ok {
		return domain.AccountAuthInfo{}, err
	}
	if info, ok := s.authInfos[id]; ok {
		return info, nil
	}
	return domain.AccountAuthInfo{}, domain.ErrNotFound
}

func (s *fakeRenewerStore) ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.proxyIPErrs[ip]; ok {
		return domain.ProxyIP{}, err
	}
	if pIP, ok := s.proxyIPs[ip]; ok {
		return pIP, nil
	}
	return domain.ProxyIP{}, domain.ErrNotFound
}

func (s *fakeRenewerStore) SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveTokenErr != nil {
		return s.saveTokenErr
	}
	s.savedTokens[id] = token
	s.savedExpirys[id] = expiresAt
	s.savedStatus[id] = authStatus
	s.savedErrors[id] = authError
	return nil
}

// =============================================================================
// Unit Tests: Renewer.RenewAll Positive Scenarios
// =============================================================================

func TestRenewer_RenewAll_Positive_ActiveDhanAccount(t *testing.T) {
	store := newFakeRenewerStore()
	accountID := uuid.New()

	store.accounts = []domain.Account{
		{ID: accountID, Broker: "dhan", Active: true},
	}
	store.authInfos[accountID] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1000000401",
		AccessToken:     "old-jwt-token",
	}

	renewCallCount := 0
	expectedExpiry := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	renewer := dhan.NewRenewer(store, slog.Default())
	renewer.RetryInterval = 1 * time.Millisecond
	renewer.RenewFunc = func(ctx context.Context, clientID, token string, proxyCfg *dhan.ProxyConfig) (string, time.Time, error) {
		renewCallCount++
		if clientID != "1000000401" {
			t.Errorf("RenewFunc clientID = %q, want '1000000401'", clientID)
		}
		if token != "old-jwt-token" {
			t.Errorf("RenewFunc token = %q, want 'old-jwt-token'", token)
		}
		return "new-refreshed-jwt", expectedExpiry, nil
	}

	refreshedNotified := false
	renewer.OnTokenRefreshed = func(ctx context.Context, id uuid.UUID) error {
		if id == accountID {
			refreshedNotified = true
		}
		return nil
	}

	err := renewer.RenewAll(context.Background())
	if err != nil {
		t.Fatalf("RenewAll failed: %v", err)
	}

	if renewCallCount != 1 {
		t.Errorf("expected 1 renew call, got %d", renewCallCount)
	}
	if !refreshedNotified {
		t.Errorf("expected OnTokenRefreshed callback to be called")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.savedTokens[accountID] != "new-refreshed-jwt" {
		t.Errorf("savedToken = %q, want 'new-refreshed-jwt'", store.savedTokens[accountID])
	}
	if store.savedStatus[accountID] != "authenticated" {
		t.Errorf("savedStatus = %q, want 'authenticated'", store.savedStatus[accountID])
	}
	if store.savedErrors[accountID] != "" {
		t.Errorf("savedErrors = %q, want empty", store.savedErrors[accountID])
	}
	if store.savedExpirys[accountID] == nil || !store.savedExpirys[accountID].Equal(expectedExpiry) {
		t.Errorf("savedExpiry = %v, want %v", store.savedExpirys[accountID], expectedExpiry)
	}
}

func TestRenewer_RenewAll_Positive_WithStaticProxy(t *testing.T) {
	store := newFakeRenewerStore()
	accountID := uuid.New()
	proxyIP := "10.0.1.50"

	store.accounts = []domain.Account{
		{ID: accountID, Broker: "dhan", Active: true},
	}
	store.authInfos[accountID] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1000000402",
		AccessToken:     "old-follower-token",
		IPAddress:       proxyIP,
	}
	store.proxyIPs[proxyIP] = domain.ProxyIP{
		IPAddress: proxyIP,
		Host:      "dc46-mum-01.algoip.in",
		Port:      443,
		Username:  "proxy-user",
		Password:  "proxy-password",
	}

	var capturedProxy *dhan.ProxyConfig

	renewer := dhan.NewRenewer(store, slog.Default())
	renewer.RetryInterval = 1 * time.Millisecond
	renewer.RenewFunc = func(ctx context.Context, clientID, token string, proxyCfg *dhan.ProxyConfig) (string, time.Time, error) {
		capturedProxy = proxyCfg
		return "new-proxy-jwt", time.Now().Add(24 * time.Hour), nil
	}

	err := renewer.RenewAll(context.Background())
	if err != nil {
		t.Fatalf("RenewAll failed: %v", err)
	}

	if capturedProxy == nil {
		t.Fatalf("expected proxyConfig to be passed to RenewFunc")
	}
	if capturedProxy.Host != "dc46-mum-01.algoip.in" || capturedProxy.Port != 443 || capturedProxy.ClientID != "proxy-user" || capturedProxy.ClientSecret != "proxy-password" {
		t.Errorf("capturedProxy mismatch: %+v", capturedProxy)
	}
}

// =============================================================================
// Unit Tests: Renewer.RenewAll Negative & Filter Scenarios
// =============================================================================

func TestRenewer_RenewAll_Filters_InactiveAndNonDhanAccounts(t *testing.T) {
	store := newFakeRenewerStore()
	dhanActive := uuid.New()
	dhanInactive := uuid.New()
	kiteActive := uuid.New()
	dhanEmptyToken := uuid.New()

	store.accounts = []domain.Account{
		{ID: dhanActive, Broker: "dhan", Active: true},
		{ID: dhanInactive, Broker: "dhan", Active: false},
		{ID: kiteActive, Broker: "zerodha", Active: true},
		{ID: dhanEmptyToken, Broker: "dhan", Active: true},
	}

	store.authInfos[dhanActive] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1001",
		AccessToken:     "valid-token",
	}
	store.authInfos[dhanInactive] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1002",
		AccessToken:     "inactive-token",
	}
	store.authInfos[kiteActive] = domain.AccountAuthInfo{
		Broker:          "zerodha",
		BrokerAccountID: "AB1234",
		AccessToken:     "kite-token",
	}
	store.authInfos[dhanEmptyToken] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1003",
		AccessToken:     "", // empty token
	}

	var renewedIDs []string

	renewer := dhan.NewRenewer(store, slog.Default())
	renewer.RetryInterval = 1 * time.Millisecond
	renewer.RenewFunc = func(ctx context.Context, clientID, token string, proxyCfg *dhan.ProxyConfig) (string, time.Time, error) {
		renewedIDs = append(renewedIDs, clientID)
		return "new-" + token, time.Now().Add(24 * time.Hour), nil
	}

	err := renewer.RenewAll(context.Background())
	if err != nil {
		t.Fatalf("RenewAll failed: %v", err)
	}

	if len(renewedIDs) != 1 || renewedIDs[0] != "1001" {
		t.Errorf("expected only '1001' to be renewed, got: %v", renewedIDs)
	}
}

func TestRenewer_RenewAll_Negative_AccountsStoreError(t *testing.T) {
	store := newFakeRenewerStore()
	store.accountsErr = errors.New("db connection timeout")

	renewer := dhan.NewRenewer(store, slog.Default())
	err := renewer.RenewAll(context.Background())
	if err == nil || !errors.Is(err, store.accountsErr) && err.Error() != "load accounts: db connection timeout" {
		t.Errorf("expected accounts error, got: %v", err)
	}
}

// =============================================================================
// Unit Tests: Renewer.RenewAll Retry & Resilience Edge Cases
// =============================================================================

func TestRenewer_RenewAll_EdgeCase_TransientFailure_RecoversOnRetry(t *testing.T) {
	store := newFakeRenewerStore()
	accountID := uuid.New()

	store.accounts = []domain.Account{
		{ID: accountID, Broker: "dhan", Active: true},
	}
	store.authInfos[accountID] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1000000403",
		AccessToken:     "flaky-jwt",
	}

	attempts := 0
	renewer := dhan.NewRenewer(store, slog.Default())
	renewer.MaxRetries = 3
	renewer.RetryInterval = 5 * time.Millisecond
	renewer.RenewFunc = func(ctx context.Context, clientID, token string, proxyCfg *dhan.ProxyConfig) (string, time.Time, error) {
		attempts++
		if attempts < 3 {
			return "", time.Time{}, errors.New("upstream gateway timeout 504")
		}
		return "success-jwt-attempt-3", time.Now().Add(24 * time.Hour), nil
	}

	err := renewer.RenewAll(context.Background())
	if err != nil {
		t.Fatalf("RenewAll should recover after retry, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.savedTokens[accountID] != "success-jwt-attempt-3" {
		t.Errorf("savedToken = %q, want 'success-jwt-attempt-3'", store.savedTokens[accountID])
	}
	if store.savedStatus[accountID] != "authenticated" {
		t.Errorf("savedStatus = %q, want 'authenticated'", store.savedStatus[accountID])
	}
}

func TestRenewer_RenewAll_EdgeCase_PermanentFailure_RecordsErrorPreservingToken(t *testing.T) {
	store := newFakeRenewerStore()
	accountID := uuid.New()
	oldExpiry := time.Now().Add(2 * time.Hour)

	store.accounts = []domain.Account{
		{ID: accountID, Broker: "dhan", Active: true},
	}
	store.authInfos[accountID] = domain.AccountAuthInfo{
		Broker:          "dhan",
		BrokerAccountID: "1000000404",
		AccessToken:     "doomed-token",
		TokenExpiresAt:  &oldExpiry,
	}

	attempts := 0
	renewer := dhan.NewRenewer(store, slog.Default())
	renewer.MaxRetries = 3
	renewer.RetryInterval = 2 * time.Millisecond
	renewer.RenewFunc = func(ctx context.Context, clientID, token string, proxyCfg *dhan.ProxyConfig) (string, time.Time, error) {
		attempts++
		return "", time.Time{}, errors.New("401 unauthorized: token revoked")
	}

	callbackCalled := false
	renewer.OnTokenRefreshed = func(ctx context.Context, id uuid.UUID) error {
		callbackCalled = true
		return nil
	}

	err := renewer.RenewAll(context.Background())
	if err != nil {
		t.Fatalf("RenewAll should not return fatal error when single account fails, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if callbackCalled {
		t.Errorf("OnTokenRefreshed should NOT be called on failure")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	// Critical: old token must NOT be blanked out on failure so user still has remaining hours
	if store.savedTokens[accountID] != "doomed-token" {
		t.Errorf("savedToken should preserve existing 'doomed-token', got: %q", store.savedTokens[accountID])
	}
	if store.savedStatus[accountID] != "error" {
		t.Errorf("savedStatus = %q, want 'error'", store.savedStatus[accountID])
	}
	if store.savedErrors[accountID] != "401 unauthorized: token revoked" {
		t.Errorf("savedErrors = %q, want '401 unauthorized: token revoked'", store.savedErrors[accountID])
	}
}

// =============================================================================
// Unit Tests: Renewer.Run Lifecycle & Context Cancellation
// =============================================================================

func TestRenewer_Run_Cancellation(t *testing.T) {
	store := newFakeRenewerStore()
	renewer := dhan.NewRenewer(store, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() {
		runDone <- renewer.Run(ctx)
	}()

	// Immediately cancel context
	cancel()

	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Run did not terminate on context cancellation")
	}
}

// =============================================================================
// Unit Tests: RenewToken HTTP Integration with godhan SDK
// =============================================================================

func TestRenewToken_HTTPIntegration_Positive(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/RenewToken" {
			t.Errorf("expected path /RenewToken, got %s", r.URL.Path)
		}
		if r.Header.Get("access-token") != "current-jwt-token" {
			t.Errorf("expected header access-token 'current-jwt-token', got %s", r.Header.Get("access-token"))
		}
		if r.Header.Get("dhanClientId") != "1000000401" {
			t.Errorf("expected header dhanClientId '1000000401', got %s", r.Header.Get("dhanClientId"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"dhanClientId": "1000000401",
			"dhanClientName": "TRADER JOHN",
			"accessToken": "brand-new-jwt-token",
			"expiryTime": "2026-10-27 06:00:00"
		}`))
	}))
	defer ts.Close()

	newToken, expiresAt, err := dhan.RenewToken(
		context.Background(),
		"1000000401",
		"current-jwt-token",
		nil,
		godhan.WithBaseURI(ts.URL),
	)
	if err != nil {
		t.Fatalf("RenewToken failed: %v", err)
	}

	if newToken != "brand-new-jwt-token" {
		t.Errorf("newToken = %q, want 'brand-new-jwt-token'", newToken)
	}
	expectedExpiry := time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC)
	if !expiresAt.Equal(expectedExpiry) {
		t.Errorf("expiresAt = %v, want %v", expiresAt, expectedExpiry)
	}
}

func TestRenewToken_HTTPIntegration_Negative_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":"failed","remarks":"token expired"}`))
	}))
	defer ts.Close()

	_, _, err := dhan.RenewToken(
		context.Background(),
		"1000000401",
		"expired-jwt",
		nil,
		godhan.WithBaseURI(ts.URL),
	)
	if err == nil {
		t.Fatalf("expected error on 401 unauthorized, got nil")
	}
}

func TestRenewToken_HTTPIntegration_EdgeCase_EmptyToken(t *testing.T) {
	_, _, err := dhan.RenewToken(context.Background(), "1000000401", "   ", nil)
	if err == nil {
		t.Fatalf("expected error when renewing empty token, got nil")
	}
}

