package kotak_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kotak"

	"github.com/DragoHex/gokotakneo/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type mockPortfolioStore struct {
	authInfo      map[uuid.UUID]domain.AccountAuthInfo
	proxyIPs      map[string]domain.ProxyIP
	tokensUpdated map[uuid.UUID]string
	authStatus    map[uuid.UUID]string
	authError     map[uuid.UUID]string
	accountStatus map[uuid.UUID]domain.AccountStatus
	positions     map[uuid.UUID][]domain.PositionSyncParam
	holdings      map[uuid.UUID][]domain.HoldingSyncParam
	margins       map[uuid.UUID]domain.MarginSyncParam
}

func newMockPortfolioStore() *mockPortfolioStore {
	return &mockPortfolioStore{
		authInfo:      make(map[uuid.UUID]domain.AccountAuthInfo),
		proxyIPs:      make(map[string]domain.ProxyIP),
		tokensUpdated: make(map[uuid.UUID]string),
		authStatus:    make(map[uuid.UUID]string),
		authError:     make(map[uuid.UUID]string),
		accountStatus: make(map[uuid.UUID]domain.AccountStatus),
		positions:     make(map[uuid.UUID][]domain.PositionSyncParam),
		holdings:      make(map[uuid.UUID][]domain.HoldingSyncParam),
		margins:       make(map[uuid.UUID]domain.MarginSyncParam),
	}
}

func (m *mockPortfolioStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	info, ok := m.authInfo[id]
	if !ok {
		return domain.AccountAuthInfo{}, domain.ErrNotFound
	}
	return info, nil
}

func (m *mockPortfolioStore) SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	m.tokensUpdated[id] = token
	m.authStatus[id] = authStatus
	m.authError[id] = authError
	return nil
}

func (m *mockPortfolioStore) SetAccountStatus(ctx context.Context, id uuid.UUID, status domain.AccountStatus) error {
	m.accountStatus[id] = status
	return nil
}

func (m *mockPortfolioStore) SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []domain.HoldingSyncParam) error {
	m.holdings[accountID] = holdings
	return nil
}

func (m *mockPortfolioStore) SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []domain.PositionSyncParam) error {
	m.positions[accountID] = positions
	return nil
}

func (m *mockPortfolioStore) SyncAccountMargins(ctx context.Context, accountID uuid.UUID, marg domain.MarginSyncParam) error {
	m.margins[accountID] = marg
	return nil
}

func (m *mockPortfolioStore) ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error) {
	p, ok := m.proxyIPs[ip]
	if !ok {
		return domain.ProxyIP{}, domain.ErrNotFound
	}
	return p, nil
}

func TestPortfolioSyncer_Execution(t *testing.T) {
	accountID := uuid.New()
	store := newMockPortfolioStore()

	sess := &kotak.TradeSessionJSON{
		Token:   "valid-token",
		Sid:     "valid-sid",
		BaseURL: "https://mock.kotak.com",
	}
	sessRaw, _ := kotak.SerializeTradeSession(sess)
	future := time.Now().Add(24 * time.Hour)

	store.authInfo[accountID] = domain.AccountAuthInfo{
		BrokerAccountID: "UCC123",
		ApiSecret:       "consumer-key",
		AccessToken:     sessRaw,
		TokenExpiresAt:  &future,
		AuthStatus:      "authenticated",
	}

	mockAPI := &mockKotakAPI{
		getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
			return []models.Position{
				{
					Product:         "MIS",
					TradingSymbol:   "TCS",
					Quantity:        10,
					FilledBuyQty:    10,
					BuyAmount:       decimal.NewFromFloat(35000),
					ExchangeSegment: "nse_cm",
				},
			}, nil
		},
		getHoldingsFunc: func(ctx context.Context) ([]models.Holding, error) {
			return []models.Holding{
				{
					CommonScripCode:    "INFY",
					Quantity:           25,
					AveragePrice:       decimal.NewFromFloat(1400),
					ClosingPrice:       decimal.NewFromFloat(1450),
					UnrealisedGainLoss: decimal.NewFromFloat(1250),
				},
			}, nil
		},
		getLimitsFunc: func(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
			return &models.Limits{
				NotionalCash: decimal.NewFromFloat(150000),
				Net:          decimal.NewFromFloat(200000),
				MarginUsed:   decimal.NewFromFloat(50000),
			}, nil
		},
	}

	syncer := &kotak.PortfolioSyncer{
		Store: store,
		APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
			return mockAPI, nil
		},
	}

	t.Run("Positive: SyncAccountPortfolio Success", func(t *testing.T) {
		err := syncer.SyncAccountPortfolio(context.Background(), accountID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio failed: %v", err)
		}

		if len(store.positions[accountID]) != 1 || store.positions[accountID][0].Instrument != "TCS" {
			t.Errorf("positions mismatch: %+v", store.positions[accountID])
		}
		if len(store.holdings[accountID]) != 1 || store.holdings[accountID][0].Instrument != "INFY" {
			t.Errorf("holdings mismatch: %+v", store.holdings[accountID])
		}
		if !store.margins[accountID].AvailableCash.Equal(decimal.NewFromFloat(150000)) {
			t.Errorf("margins mismatch: %+v", store.margins[accountID])
		}
		if store.accountStatus[accountID] != domain.AccountStatusActive {
			t.Errorf("account status = %s, want active", store.accountStatus[accountID])
		}
	})

	t.Run("Negative: Expired Session Missing Renewer", func(t *testing.T) {
		badAccID := uuid.New()
		store.authInfo[badAccID] = domain.AccountAuthInfo{
			BrokerAccountID: "BAD1",
			AccessToken:     "",
		}

		err := syncer.SyncAccountPortfolio(context.Background(), badAccID)
		if err == nil || !errors.Is(err, domain.ErrAuthExpired) {
			t.Fatalf("expected ErrAuthExpired, got %v", err)
		}
		if store.authStatus[badAccID] != "unauthenticated" {
			t.Errorf("auth status = %s, want unauthenticated", store.authStatus[badAccID])
		}
	})

	t.Run("Positive: Empty Positions and Empty Holdings treated as clean empty slices", func(t *testing.T) {
		emptyAccID := uuid.New()
		store.authInfo[emptyAccID] = domain.AccountAuthInfo{
			BrokerAccountID: "EMPTY1",
			ApiSecret:       "consumer-key",
			AccessToken:     sessRaw,
			TokenExpiresAt:  &future,
			AuthStatus:      "authenticated",
		}

		emptyAPI := &mockKotakAPI{
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return []models.Position{}, nil
			},
			getHoldingsFunc: func(ctx context.Context) ([]models.Holding, error) {
				return []models.Holding{}, nil
			},
			getLimitsFunc: func(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
				return &models.Limits{
					NotionalCash: decimal.Zero,
					Net:          decimal.Zero,
				}, nil
			},
		}

		emptySyncer := &kotak.PortfolioSyncer{
			Store: store,
			APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
				return emptyAPI, nil
			},
		}

		err := emptySyncer.SyncAccountPortfolio(context.Background(), emptyAccID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio failed on empty positions/holdings: %v", err)
		}
		if len(store.positions[emptyAccID]) != 0 {
			t.Errorf("expected 0 positions, got %d", len(store.positions[emptyAccID]))
		}
		if len(store.holdings[emptyAccID]) != 0 {
			t.Errorf("expected 0 holdings, got %d", len(store.holdings[emptyAccID]))
		}
		if store.accountStatus[emptyAccID] != domain.AccountStatusActive {
			t.Errorf("account status = %s, want active", store.accountStatus[emptyAccID])
		}
	})
}
