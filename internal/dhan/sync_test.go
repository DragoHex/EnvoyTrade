package dhan_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"envoytrade/internal/dhan"
	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeSyncStore struct {
	authInfo             domain.AccountAuthInfo
	authInfoErr          error
	syncedPos            []domain.PositionSyncParam
	syncedHold           []domain.HoldingSyncParam
	syncedMargin         domain.MarginSyncParam
	statusUpdated        string
	errorUpdated         string
	expiresAtUpdated     *time.Time
	accountStatusUpdated domain.AccountStatus
}

func (s *fakeSyncStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	if s.authInfoErr != nil {
		return domain.AccountAuthInfo{}, s.authInfoErr
	}
	return s.authInfo, nil
}

func (s *fakeSyncStore) SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	s.statusUpdated = authStatus
	s.errorUpdated = authError
	s.expiresAtUpdated = expiresAt
	return nil
}

func (s *fakeSyncStore) SetAccountStatus(ctx context.Context, id uuid.UUID, status domain.AccountStatus) error {
	s.accountStatusUpdated = status
	return nil
}

func (s *fakeSyncStore) SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []domain.HoldingSyncParam) error {
	s.syncedHold = holdings
	return nil
}

func (s *fakeSyncStore) SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []domain.PositionSyncParam) error {
	s.syncedPos = positions
	return nil
}

func (s *fakeSyncStore) SyncAccountMargins(ctx context.Context, accountID uuid.UUID, m domain.MarginSyncParam) error {
	s.syncedMargin = m
	return nil
}

func (s *fakeSyncStore) ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error) {
	return domain.ProxyIP{}, domain.ErrNotFound
}

type fakeDhanSyncAPI struct {
	positions []models.Position
	holdings  []models.Holding
	funds     models.FundLimitResponse
	err       error
}

func (f *fakeDhanSyncAPI) GetPositions(ctx context.Context) ([]models.Position, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.positions, nil
}

func (f *fakeDhanSyncAPI) GetHoldings(ctx context.Context) ([]models.Holding, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.holdings, nil
}

func (f *fakeDhanSyncAPI) GetFundLimits(ctx context.Context) (*models.FundLimitResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &f.funds, nil
}

func TestPortfolioSyncer_SyncAccountPortfolio_Positive(t *testing.T) {
	accountID := uuid.New()
	store := &fakeSyncStore{
		authInfo: domain.AccountAuthInfo{
			ID:              accountID,
			Broker:          "dhan",
			BrokerAccountID: "1000000001",
			AccessToken:     "valid-jwt-token",
			AuthStatus:      "unauthenticated",
		},
	}

	api := &fakeDhanSyncAPI{
		positions: []models.Position{
			{
				TradingSymbol:    "TATASTEEL",
				ProductType:      "CNC",
				NetQty:           50,
				BuyAvg:           decimal.NewFromFloat(120.0),
				RealizedProfit:   decimal.NewFromFloat(100.0),
				UnrealizedProfit: decimal.NewFromFloat(50.0),
			},
		},
		holdings: []models.Holding{
			{
				TradingSymbol: "RELIANCE",
				AvailableQty:  10,
				AvgCostPrice:  decimal.NewFromFloat(2800.0),
			},
		},
		funds: models.FundLimitResponse{
			AvailableBalance: decimal.NewFromFloat(50000.0),
			CollateralAmount: decimal.NewFromFloat(25000.0),
		},
	}

	syncer := &dhan.PortfolioSyncer{
		Store: store,
		APIFactory: func(clientID, token string, proxyCfg *dhan.ProxyConfig) (dhan.SyncAPI, error) {
			return api, nil
		},
	}

	err := syncer.SyncAccountPortfolio(context.Background(), accountID)
	if err != nil {
		t.Fatalf("SyncAccountPortfolio failed: %v", err)
	}

	// Verify positions synced
	if len(store.syncedPos) != 1 || store.syncedPos[0].Instrument != "TATASTEEL" || store.syncedPos[0].Quantity != 50 {
		t.Errorf("positions not synced properly: %+v", store.syncedPos)
	}

	// Verify holdings synced
	if len(store.syncedHold) != 1 || store.syncedHold[0].Instrument != "RELIANCE" || store.syncedHold[0].SellableQuantity != 10 {
		t.Errorf("holdings not synced properly: %+v", store.syncedHold)
	}

	// Verify margins synced
	if !store.syncedMargin.AvailableCash.Equal(decimal.NewFromFloat(50000.0)) {
		t.Errorf("margins cash = %s, want 50000.0", store.syncedMargin.AvailableCash)
	}

	// Verify auth status updated to authenticated
	if store.statusUpdated != "authenticated" {
		t.Errorf("statusUpdated = %s, want authenticated", store.statusUpdated)
	}
}

func TestPortfolioSyncer_SyncAccountPortfolio_Negative_MissingToken(t *testing.T) {
	accountID := uuid.New()
	store := &fakeSyncStore{
		authInfo: domain.AccountAuthInfo{
			ID:              accountID,
			Broker:          "dhan",
			BrokerAccountID: "1000000001",
			AccessToken:     "", // Missing
		},
	}

	syncer := &dhan.PortfolioSyncer{
		Store: store,
	}

	err := syncer.SyncAccountPortfolio(context.Background(), accountID)
	if err == nil {
		t.Errorf("expected error for missing access token, got nil")
	}
	if store.statusUpdated != "error" {
		t.Errorf("statusUpdated = %s, want error", store.statusUpdated)
	}
}

func TestPortfolioSyncer_SyncAccountPortfolio_Negative_AuthError(t *testing.T) {
	accountID := uuid.New()
	store := &fakeSyncStore{
		authInfo: domain.AccountAuthInfo{
			ID:              accountID,
			Broker:          "dhan",
			BrokerAccountID: "1000000001",
			AccessToken:     "expired-token",
		},
	}

	api := &fakeDhanSyncAPI{
		err: errors.New("401 Unauthorized: token has expired"),
	}

	syncer := &dhan.PortfolioSyncer{
		Store: store,
		APIFactory: func(clientID, token string, proxyCfg *dhan.ProxyConfig) (dhan.SyncAPI, error) {
			return api, nil
		},
	}

	err := syncer.SyncAccountPortfolio(context.Background(), accountID)
	if err == nil {
		t.Errorf("expected error for 401, got nil")
	}
	if store.statusUpdated != "error" {
		t.Errorf("statusUpdated = %s, want error", store.statusUpdated)
	}
}

func TestPortfolioSyncer_SyncAccountPortfolio_PreservesOrComputesExpiry(t *testing.T) {
	// Case 1: nil TokenExpiresAt -> computes 24h expiry
	t.Run("nil TokenExpiresAt computes 24h expiry", func(t *testing.T) {
		accountID := uuid.New()
		store := &fakeSyncStore{
			authInfo: domain.AccountAuthInfo{
				ID:              accountID,
				Broker:          "dhan",
				BrokerAccountID: "1000000001",
				AccessToken:     "valid-token",
				AuthStatus:      "pending",
				TokenExpiresAt:  nil,
			},
		}

		api := &fakeDhanSyncAPI{
			funds: models.FundLimitResponse{
				AvailableBalance: decimal.NewFromInt(10000),
			},
		}

		syncer := &dhan.PortfolioSyncer{
			Store: store,
			APIFactory: func(clientID, token string, proxyCfg *dhan.ProxyConfig) (dhan.SyncAPI, error) {
				return api, nil
			},
		}

		err := syncer.SyncAccountPortfolio(context.Background(), accountID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.statusUpdated != "authenticated" {
			t.Errorf("statusUpdated = %s, want authenticated", store.statusUpdated)
		}
		if store.accountStatusUpdated != domain.AccountStatusActive {
			t.Errorf("accountStatusUpdated = %v, want active", store.accountStatusUpdated)
		}
		if store.expiresAtUpdated == nil {
			t.Fatalf("expiresAtUpdated is nil, want non-nil 24h expiry")
		}
		if !store.expiresAtUpdated.After(time.Now().Add(23 * time.Hour)) {
			t.Errorf("expected expiresAtUpdated to be ~24h in future, got %v", store.expiresAtUpdated)
		}
	})

	// Case 2: existing future TokenExpiresAt -> preserved
	t.Run("existing future TokenExpiresAt is preserved", func(t *testing.T) {
		accountID := uuid.New()
		futureExpiry := time.Now().UTC().Add(18 * time.Hour).Truncate(time.Second)
		store := &fakeSyncStore{
			authInfo: domain.AccountAuthInfo{
				ID:              accountID,
				Broker:          "dhan",
				BrokerAccountID: "1000000001",
				AccessToken:     "valid-token",
				AuthStatus:      "pending",
				TokenExpiresAt:  &futureExpiry,
			},
		}

		api := &fakeDhanSyncAPI{
			funds: models.FundLimitResponse{
				AvailableBalance: decimal.NewFromInt(10000),
			},
		}

		syncer := &dhan.PortfolioSyncer{
			Store: store,
			APIFactory: func(clientID, token string, proxyCfg *dhan.ProxyConfig) (dhan.SyncAPI, error) {
				return api, nil
			},
		}

		err := syncer.SyncAccountPortfolio(context.Background(), accountID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.statusUpdated != "authenticated" {
			t.Errorf("statusUpdated = %s, want authenticated", store.statusUpdated)
		}
		if store.accountStatusUpdated != domain.AccountStatusActive {
			t.Errorf("accountStatusUpdated = %v, want active", store.accountStatusUpdated)
		}
		if store.expiresAtUpdated == nil || !store.expiresAtUpdated.Equal(futureExpiry) {
			t.Errorf("expiresAtUpdated = %v, want %v", store.expiresAtUpdated, futureExpiry)
		}
	})
}
