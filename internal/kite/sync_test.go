package kite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"envoytrade/internal/crypto"
	"envoytrade/internal/domain"
	"envoytrade/internal/kite"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

type mockSyncStore struct {
	authInfo      domain.AccountAuthInfo
	authInfoErr   error
	setTokenCalls []struct {
		Token      string
		ExpiresAt  *time.Time
		AuthStatus string
		AuthError  string
	}
	syncedHoldings  []domain.HoldingSyncParam
	syncedPositions []domain.PositionSyncParam
	syncedMargins   *domain.MarginSyncParam
}

func (m *mockSyncStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	if m.authInfoErr != nil {
		return domain.AccountAuthInfo{}, m.authInfoErr
	}
	return m.authInfo, nil
}

func (m *mockSyncStore) SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	m.setTokenCalls = append(m.setTokenCalls, struct {
		Token      string
		ExpiresAt  *time.Time
		AuthStatus string
		AuthError  string
	}{
		Token:      token,
		ExpiresAt:  expiresAt,
		AuthStatus: authStatus,
		AuthError:  authError,
	})
	m.authInfo.AccessToken = token
	m.authInfo.TokenExpiresAt = expiresAt
	m.authInfo.AuthStatus = authStatus
	m.authInfo.AuthError = authError
	return nil
}

func (m *mockSyncStore) SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []domain.HoldingSyncParam) error {
	m.syncedHoldings = holdings
	return nil
}

func (m *mockSyncStore) SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []domain.PositionSyncParam) error {
	m.syncedPositions = positions
	return nil
}

func (m *mockSyncStore) SyncAccountMargins(ctx context.Context, accountID uuid.UUID, margin domain.MarginSyncParam) error {
	m.syncedMargins = &margin
	return nil
}

func (m *mockSyncStore) ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error) {
	return domain.ProxyIP{}, domain.ErrNotFound
}

func TestSyncAccountPortfolio_WithValidAccessToken(t *testing.T) {
	accID := uuid.New()
	validExpiry := time.Now().Add(2 * time.Hour)
	store := &mockSyncStore{
		authInfo: domain.AccountAuthInfo{
			ID:              accID,
			Role:            "master",
			Broker:          "kite",
			BrokerAccountID: "TEST01",
			ApiKey:          "test_api_key",
			ApiSecret:       "test_api_secret",
			AccessToken:     "valid_token_123",
			TokenExpiresAt:  &validExpiry,
			AuthStatus:      "authenticated",
		},
	}

	fakeAPI := &fakeKiteAPI{
		holdings: kiteconnect.Holdings{
			{
				Tradingsymbol: "INFY",
				Quantity:      10,
				UsedQuantity:  0,
				AveragePrice:  1500.0,
				LastPrice:     1550.0,
				PnL:           500.0,
			},
		},
		positions: kiteconnect.Positions{
			Net: []kiteconnect.Position{
				{
					Product:       "NRML",
					Tradingsymbol: "NIFTY26SEPFUT",
					Exchange:      "NFO",
					Quantity:      50,
					BuyPrice:      24000.0,
					LastPrice:     24100.0,
					M2M:           5000.0,
					Realised:      0.0,
				},
			},
		},
		margins: kiteconnect.AllMargins{
			Equity: kiteconnect.Margins{
				Available: kiteconnect.AvailableMargins{
					Cash:        100000.0,
					LiveBalance: 95000.0,
				},
			},
		},
	}

	syncer := &kite.PortfolioSyncer{
		Store: store,
		APIFactory: func(apiKey, accessToken string) (kite.API, error) {
			if accessToken != "valid_token_123" {
				t.Fatalf("APIFactory received token %q, want valid_token_123", accessToken)
			}
			return fakeAPI, nil
		},
		LoginFunc: func(ctx context.Context, userID, password, totpSecret, apiKey, apiSecret string) (string, error) {
			t.Fatalf("LoginFunc should not be called when token is valid")
			return "", nil
		},
	}

	err := syncer.SyncAccountPortfolio(context.Background(), accID)
	if err != nil {
		t.Fatalf("SyncAccountPortfolio failed: %v", err)
	}

	if len(store.syncedHoldings) != 1 || store.syncedHoldings[0].Instrument != "INFY" {
		t.Fatalf("syncedHoldings = %+v, want 1 INFY holding", store.syncedHoldings)
	}
	if len(store.syncedPositions) != 1 || store.syncedPositions[0].Instrument != "NIFTY26SEPFUT" {
		t.Fatalf("syncedPositions = %+v, want 1 position", store.syncedPositions)
	}
	if store.syncedMargins == nil || !store.syncedMargins.AccountValue.Equal(decimal.NewFromFloat(95000.0)) {
		t.Fatalf("syncedMargins = %+v", store.syncedMargins)
	}
}

func TestSyncAccountPortfolio_ExpiredToken_PerformsLoginAndSyncs(t *testing.T) {
	accID := uuid.New()
	expired := time.Now().Add(-1 * time.Hour)

	encPass, _ := crypto.Encrypt("mypassword")
	encTotp, _ := crypto.Encrypt("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")

	store := &mockSyncStore{
		authInfo: domain.AccountAuthInfo{
			ID:                  accID,
			Role:                "master",
			Broker:              "kite",
			BrokerAccountID:     "TEST01",
			ApiKey:              "test_api_key",
			ApiSecret:           "test_api_secret",
			EncryptedPassword:   encPass,
			EncryptedTotpSecret: encTotp,
			AccessToken:         "expired_token",
			TokenExpiresAt:      &expired,
			AuthStatus:          "unauthenticated",
		},
	}

	fakeAPI := &fakeKiteAPI{
		holdings: kiteconnect.Holdings{},
	}

	loginCalled := false
	syncer := &kite.PortfolioSyncer{
		Store: store,
		APIFactory: func(apiKey, accessToken string) (kite.API, error) {
			if accessToken != "new_token_777" {
				t.Fatalf("APIFactory got accessToken %q, want new_token_777", accessToken)
			}
			return fakeAPI, nil
		},
		LoginFunc: func(ctx context.Context, userID, password, totpSecret, apiKey, apiSecret string) (string, error) {
			loginCalled = true
			if userID != "TEST01" || password != "mypassword" {
				t.Errorf("login params: user=%s, pass=%s", userID, password)
			}
			return "new_token_777", nil
		},
	}

	refreshedAccountID := uuid.Nil
	syncer.OnTokenRefreshed = func(ctx context.Context, accountID uuid.UUID) error {
		refreshedAccountID = accountID
		return nil
	}

	err := syncer.SyncAccountPortfolio(context.Background(), accID)
	if err != nil {
		t.Fatalf("SyncAccountPortfolio failed: %v", err)
	}

	if !loginCalled {
		t.Fatalf("expected LoginFunc to be called")
	}
	if refreshedAccountID != accID {
		t.Errorf("refreshedAccountID = %s, want %s", refreshedAccountID, accID)
	}
	if len(store.setTokenCalls) != 1 || store.setTokenCalls[0].Token != "new_token_777" {
		t.Fatalf("setTokenCalls = %+v, want token new_token_777", store.setTokenCalls)
	}
	if store.setTokenCalls[0].AuthStatus != "authenticated" {
		t.Errorf("AuthStatus = %s, want authenticated", store.setTokenCalls[0].AuthStatus)
	}
}

func TestSyncAccountPortfolio_LoginFailure_RecordsAuthError(t *testing.T) {
	accID := uuid.New()
	encPass, _ := crypto.Encrypt("wrongpass")
	encTotp, _ := crypto.Encrypt("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")

	store := &mockSyncStore{
		authInfo: domain.AccountAuthInfo{
			ID:                  accID,
			Role:                "master",
			Broker:              "kite",
			BrokerAccountID:     "TEST01",
			ApiKey:              "test_api_key",
			ApiSecret:           "test_api_secret",
			EncryptedPassword:   encPass,
			EncryptedTotpSecret: encTotp,
			AccessToken:         "",
			AuthStatus:          "unauthenticated",
		},
	}

	syncer := &kite.PortfolioSyncer{
		Store: store,
		LoginFunc: func(ctx context.Context, userID, password, totpSecret, apiKey, apiSecret string) (string, error) {
			return "", errors.New("invalid credentials")
		},
	}

	err := syncer.SyncAccountPortfolio(context.Background(), accID)
	if err == nil {
		t.Fatalf("expected error on login failure, got nil")
	}

	if len(store.setTokenCalls) != 1 {
		t.Fatalf("expected 1 setTokenCall recording failure, got %d", len(store.setTokenCalls))
	}
	if store.setTokenCalls[0].AuthStatus != "error" {
		t.Errorf("AuthStatus = %s, want error", store.setTokenCalls[0].AuthStatus)
	}
	if store.setTokenCalls[0].AuthError != "invalid credentials" {
		t.Errorf("AuthError = %s, want invalid credentials", store.setTokenCalls[0].AuthError)
	}
}
