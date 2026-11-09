package kotak_test

import (
	"context"
	"errors"
	"testing"
	"time"

	envoytradecrypto "envoytrade/internal/crypto"
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
	if info, ok := m.authInfo[id]; ok {
		info.AccessToken = token
		info.TokenExpiresAt = expiresAt
		info.AuthStatus = authStatus
		m.authInfo[id] = info
	}
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

func (m *mockPortfolioStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	return nil, nil
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
					Quantity:           decimal.NewFromInt(25),
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

	t.Run("Positive: Kotak Limits Fallback when AvailableCash is Zero but Margin NonZero", func(t *testing.T) {
		cashAccID := uuid.New()
		store.authInfo[cashAccID] = domain.AccountAuthInfo{
			BrokerAccountID: "CASH123",
			ApiSecret:       "consumer-key",
			AccessToken:     sessRaw,
			TokenExpiresAt:  &future,
			AuthStatus:      "authenticated",
		}
		cashMockAPI := &mockKotakAPI{
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return nil, nil
			},
			getHoldingsFunc: func(ctx context.Context) ([]models.Holding, error) {
				return nil, nil
			},
			getLimitsFunc: func(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
				// Models live Kotak Neo API response:
				// CollateralValue is opening funds (766.6), Net is remaining funds (706.6)
				return &models.Limits{
					CollateralValue: decimal.NewFromFloat(766.6),
					Net:             decimal.NewFromFloat(706.6),
					MarginUsed:      decimal.NewFromFloat(60.0),
					NotionalCash:    decimal.Zero,
				}, nil
			},
		}

		cashSyncer := &kotak.PortfolioSyncer{
			Store: store,
			APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
				return cashMockAPI, nil
			},
		}

		err := cashSyncer.SyncAccountPortfolio(context.Background(), cashAccID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio failed: %v", err)
		}

		if store.margins[cashAccID].AvailableCash == nil || !store.margins[cashAccID].AvailableCash.Equal(decimal.NewFromFloat(706.6)) {
			t.Errorf("AvailableCash = %v, want 706.6", store.margins[cashAccID].AvailableCash)
		}
		if store.margins[cashAccID].AvailableMargin == nil || !store.margins[cashAccID].AvailableMargin.Equal(decimal.NewFromFloat(706.6)) {
			t.Errorf("AvailableMargin = %v, want 706.6", store.margins[cashAccID].AvailableMargin)
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

	t.Run("Positive: Kotak Limits Cash Derivation and Position Metrics", func(t *testing.T) {
		calcAccID := uuid.New()
		store.authInfo[calcAccID] = domain.AccountAuthInfo{
			BrokerAccountID: "KOTAK970",
			ApiSecret:       "consumer-key",
			AccessToken:     sessRaw,
			TokenExpiresAt:  &future,
			AuthStatus:      "authenticated",
		}

		calcAPI := &mockKotakAPI{
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return []models.Position{
					{
						Product:         "NRML",
						TradingSymbol:   "CRUDEOILM17NOV265000PE",
						Quantity:        0,
						FilledBuyQty:    10,
						BuyAmount:       decimal.NewFromFloat(1000),
						FilledSellQty:   10,
						SellAmount:      decimal.NewFromFloat(1200),
						ExchangeSegment: "mcx_fo",
					},
					{
						Product:         "MIS",
						TradingSymbol:   "RELIANCE",
						Quantity:        5,
						FilledBuyQty:    5,
						BuyAmount:       decimal.NewFromFloat(12500),
						FilledSellQty:   0,
						ExchangeSegment: "nse_cm",
					},
				}, nil
			},
			getHoldingsFunc: func(ctx context.Context) ([]models.Holding, error) {
				return []models.Holding{}, nil
			},
			getLimitsFunc: func(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
				return &models.Limits{
					NotionalCash: decimal.Zero, // Real Kotak Neo returns 0 NotionalCash
					Net:          decimal.NewFromFloat(970),
				}, nil
			},
		}

		calcSyncer := &kotak.PortfolioSyncer{
			Store: store,
			APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
				return calcAPI, nil
			},
		}

		err := calcSyncer.SyncAccountPortfolio(context.Background(), calcAccID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio failed: %v", err)
		}

		m := store.margins[calcAccID]
		if m.AvailableCash == nil || !m.AvailableCash.Equal(decimal.NewFromFloat(970)) {
			t.Errorf("AvailableCash = %v, want 970", m.AvailableCash)
		}
		if m.AvailableMargin == nil || !m.AvailableMargin.Equal(decimal.NewFromFloat(970)) {
			t.Errorf("AvailableMargin = %v, want 970", m.AvailableMargin)
		}
		if m.NetQty != 5 {
			t.Errorf("NetQty = %d, want 5", m.NetQty)
		}
		if !m.TotalMtm.Equal(decimal.NewFromFloat(200)) {
			t.Errorf("TotalMtm = %s, want 200", m.TotalMtm)
		}
		if !m.RealizedPnl.Equal(decimal.NewFromFloat(200)) {
			t.Errorf("RealizedPnl = %s, want 200", m.RealizedPnl)
		}
		if !m.AccountValue.Equal(decimal.NewFromFloat(1170)) {
			t.Errorf("AccountValue = %s, want 1170", m.AccountValue)
		}
	})

	t.Run("Positive: Successful Login Triggers OnTokenRefreshed", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", "test-key-32-bytes-long-for-test!")
		renewAccID := uuid.New()
		store.authInfo[renewAccID] = domain.AccountAuthInfo{
			BrokerAccountID: "RENEW1",
			ApiSecret:       "consumer-key",
			AccessToken:     "", // empty -> needs login
		}

		var tokenRefreshedCalled bool
		var refreshedAccountID uuid.UUID

		renewer := kotak.NewRenewer(store, nil)
		renewer.LoginFunc = func(ctx context.Context, creds kotak.HeadlessLoginCredentials) (*kotak.TradeSessionJSON, error) {
			return &kotak.TradeSessionJSON{
				Token:   "new-token",
				Sid:     "new-sid",
				BaseURL: "https://api.kotakneo.com",
			}, nil
		}

		renewSyncer := &kotak.PortfolioSyncer{
			Store: store,
			APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
				return mockAPI, nil
			},
			Renewer: renewer,
			OnTokenRefreshed: func(ctx context.Context, accID uuid.UUID) error {
				tokenRefreshedCalled = true
				refreshedAccountID = accID
				return nil
			},
		}

		// Set encrypted credentials in store for renewer
		encMPIN, _ := envoytradecrypto.Encrypt("123456")
		encTOTP, _ := envoytradecrypto.Encrypt("JBSWY3DPEHPK3PXP")
		store.authInfo[renewAccID] = domain.AccountAuthInfo{
			BrokerAccountID:       "RENEW1",
			ApiKey:                "9876543210",
			ApiSecret:             "consumer-key",
			AccessToken:           "",
			EncryptedPassword:     encMPIN,
			EncryptedTotpSecret:   encTOTP,
		}

		err := renewSyncer.SyncAccountPortfolio(context.Background(), renewAccID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio with renewer failed: %v", err)
		}

		if !tokenRefreshedCalled {
			t.Error("expected OnTokenRefreshed to be called, but was not")
		}
		if refreshedAccountID != renewAccID {
			t.Errorf("refreshedAccountID = %s, want %s", refreshedAccountID, renewAccID)
		}
	})

	t.Run("Positive: Fractional and Decimal Holdings correctly floored to int", func(t *testing.T) {
		accID := uuid.New()
		store := newMockPortfolioStore()
		sess := &kotak.TradeSessionJSON{
			Token:   "valid-token",
			Sid:     "valid-sid",
			BaseURL: "https://mock.kotak.com",
		}
		sessRaw, _ := kotak.SerializeTradeSession(sess)
		store.authInfo[accID] = domain.AccountAuthInfo{
			BrokerAccountID: "FRAC01",
			ApiKey:          "9876543210",
			ApiSecret:       "consumer-key",
			AccessToken:     sessRaw,
		}

		mockAPI := &mockKotakAPI{
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return []models.Position{}, nil
			},
			getHoldingsFunc: func(ctx context.Context) ([]models.Holding, error) {
				return []models.Holding{
					{
						CommonScripCode:  "LIQUIDBEES",
						Quantity:         decimal.RequireFromString("0.002"),
						SellableQuantity: decimal.Zero,
						ClosingPrice:     decimal.NewFromInt(1000),
					},
					{
						CommonScripCode:  "PP001ZG-GR",
						Quantity:         decimal.RequireFromString("15456.843"),
						SellableQuantity: decimal.Zero,
						ClosingPrice:     decimal.RequireFromString("93.9483"),
					},
					{
						CommonScripCode:  "LIQUIDPLUS",
						Quantity:         decimal.RequireFromString("559"),
						SellableQuantity: decimal.RequireFromString("559"),
						ClosingPrice:     decimal.RequireFromString("1111.68"),
					},
					{
						CommonScripCode:  "OPTFUTCRUDEOIL",
						Quantity:         decimal.RequireFromString("-100"),
						SellableQuantity: decimal.Zero,
						ClosingPrice:     decimal.RequireFromString("39.4"),
					},
				}, nil
			},
			getLimitsFunc: func(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
				return &models.Limits{
					Net: decimal.NewFromInt(100000),
				}, nil
			},
		}

		syncer := &kotak.PortfolioSyncer{
			Store: store,
			APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
				return mockAPI, nil
			},
		}

		err := syncer.SyncAccountPortfolio(context.Background(), accID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio failed: %v", err)
		}

		synced := store.holdings[accID]
		if len(synced) != 4 {
			t.Fatalf("expected 4 holdings, got %d", len(synced))
		}

		// LIQUIDBEES: 0.002 units -> 0
		if synced[0].Instrument != "LIQUIDBEES" || synced[0].SellableQuantity != 0 {
			t.Errorf("LIQUIDBEES SellableQuantity = %d, want 0", synced[0].SellableQuantity)
		}

		// PP001ZG-GR: 15456.843 units -> 15456
		if synced[1].Instrument != "PP001ZG-GR" || synced[1].SellableQuantity != 15456 {
			t.Errorf("PP001ZG-GR SellableQuantity = %d, want 15456", synced[1].SellableQuantity)
		}

		// LIQUIDPLUS: 559 units -> 559
		if synced[2].Instrument != "LIQUIDPLUS" || synced[2].SellableQuantity != 559 {
			t.Errorf("LIQUIDPLUS SellableQuantity = %d, want 559", synced[2].SellableQuantity)
		}

		// OPTFUTCRUDEOIL: -100 units -> 0 (negative quantity floored to 0)
		if synced[3].Instrument != "OPTFUTCRUDEOIL" || synced[3].SellableQuantity != 0 {
			t.Errorf("OPTFUTCRUDEOIL SellableQuantity = %d, want 0", synced[3].SellableQuantity)
		}
	})

	t.Run("Positive: Open Positions properly synced with non-zero Quantity, totals, and exit Action", func(t *testing.T) {
		accID := uuid.New()
		store := newMockPortfolioStore()
		sess := &kotak.TradeSessionJSON{
			Token:   "valid-token",
			Sid:     "valid-sid",
			BaseURL: "https://mock.kotak.com",
		}
		sessRaw, _ := kotak.SerializeTradeSession(sess)
		store.authInfo[accID] = domain.AccountAuthInfo{
			BrokerAccountID: "POS01",
			ApiKey:          "9876543210",
			ApiSecret:       "consumer-key",
			AccessToken:     sessRaw,
		}

		mockAPI := &mockKotakAPI{
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return []models.Position{
					{
						Product:         "NRML",
						TradingSymbol:   "CRUDEOIL17NOV267300PE",
						ExchangeSegment: "mcx_fo",
						CFSellQty:       100,
						CFSellAmount:    decimal.NewFromFloat(2640.00),
					},
					{
						Product:         "NRML",
						TradingSymbol:   "NIFTY26N0924600PE",
						ExchangeSegment: "nse_fo",
						FilledSellQty:   325,
						SellAmount:      decimal.NewFromFloat(4143.75),
					},
					{
						Product:         "NRML",
						TradingSymbol:   "NIFTY26N0925300CE",
						ExchangeSegment: "nse_fo",
						FilledBuyQty:    325,
						BuyAmount:       decimal.NewFromFloat(1251.25),
						FilledSellQty:   325,
						SellAmount:      decimal.NewFromFloat(5053.75),
					},
				}, nil
			},
			getHoldingsFunc: func(ctx context.Context) ([]models.Holding, error) {
				return []models.Holding{}, nil
			},
			getLimitsFunc: func(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
				return &models.Limits{
					Net: decimal.NewFromInt(100000),
				}, nil
			},
		}

		syncer := &kotak.PortfolioSyncer{
			Store: store,
			APIFactory: func(consumerKey string, sess *kotak.TradeSessionJSON, proxyURL string) (kotak.SyncAPI, error) {
				return mockAPI, nil
			},
		}

		err := syncer.SyncAccountPortfolio(context.Background(), accID)
		if err != nil {
			t.Fatalf("SyncAccountPortfolio failed: %v", err)
		}

		synced := store.positions[accID]
		if len(synced) != 3 {
			t.Fatalf("expected 3 positions, got %d", len(synced))
		}

		// 1. Overnight short CRUDEOIL
		if synced[0].Quantity != -100 {
			t.Errorf("synced[0].Quantity = %d, want -100", synced[0].Quantity)
		}
		if !synced[0].SellPrice.Equal(decimal.NewFromFloat(26.40)) {
			t.Errorf("synced[0].SellPrice = %s, want 26.40", synced[0].SellPrice)
		}
		if synced[0].SellQuantity != 100 {
			t.Errorf("synced[0].SellQuantity = %d, want 100", synced[0].SellQuantity)
		}
		if synced[0].Action != "exit" {
			t.Errorf("synced[0].Action = %s, want 'exit'", synced[0].Action)
		}

		// 2. Day trade open short NIFTY PE
		if synced[1].Quantity != -325 {
			t.Errorf("synced[1].Quantity = %d, want -325", synced[1].Quantity)
		}
		if !synced[1].SellPrice.Equal(decimal.NewFromFloat(12.75)) {
			t.Errorf("synced[1].SellPrice = %s, want 12.75", synced[1].SellPrice)
		}
		if synced[1].SellQuantity != 325 {
			t.Errorf("synced[1].SellQuantity = %d, want 325", synced[1].SellQuantity)
		}
		if synced[1].Action != "exit" {
			t.Errorf("synced[1].Action = %s, want 'exit'", synced[1].Action)
		}

		// 3. Closed day trade NIFTY CE
		if synced[2].Quantity != 0 {
			t.Errorf("synced[2].Quantity = %d, want 0", synced[2].Quantity)
		}
		if !synced[2].BuyPrice.Equal(decimal.NewFromFloat(3.85)) {
			t.Errorf("synced[2].BuyPrice = %s, want 3.85", synced[2].BuyPrice)
		}
		if !synced[2].SellPrice.Equal(decimal.NewFromFloat(15.55)) {
			t.Errorf("synced[2].SellPrice = %s, want 15.55", synced[2].SellPrice)
		}
		if synced[2].BuyQuantity != 325 || synced[2].SellQuantity != 325 {
			t.Errorf("synced[2] BuyQuantity=%d SellQuantity=%d, want 325, 325", synced[2].BuyQuantity, synced[2].SellQuantity)
		}
	})
}
