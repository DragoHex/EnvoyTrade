package dhan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan"
	"github.com/DragoHex/godhan/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SyncAPI narrows the Dhan API methods needed for portfolio and balances synchronization.
type SyncAPI interface {
	GetPositions(ctx context.Context) ([]models.Position, error)
	GetHoldings(ctx context.Context) ([]models.Holding, error)
	GetFundLimits(ctx context.Context) (*models.FundLimitResponse, error)
}

// PortfolioStore declares the store methods needed for portfolio and credentials sync.
type PortfolioStore interface {
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error
	SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []domain.HoldingSyncParam) error
	SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []domain.PositionSyncParam) error
	SyncAccountMargins(ctx context.Context, accountID uuid.UUID, m domain.MarginSyncParam) error
	ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error)
}

type accountStatusSetter interface {
	SetAccountStatus(ctx context.Context, id uuid.UUID, status domain.AccountStatus) error
}

// PortfolioSyncer orchestrates Dhan account position, holding, and margin sync.
type PortfolioSyncer struct {
	Store      PortfolioStore
	APIFactory func(clientID, token string, proxyCfg *ProxyConfig) (SyncAPI, error)
}

// defaultAPIFactory constructs a real godhan.Client with optional static proxy routing.
func defaultAPIFactory(clientID, token string, proxyCfg *ProxyConfig) (SyncAPI, error) {
	opts := []godhan.Option{}
	if proxyCfg != nil && proxyCfg.Host != "" {
		opts = append(opts, godhan.WithProxy(*proxyCfg))
	}
	return godhan.NewClient(clientID, token, opts...), nil
}

// SyncAccountPortfolio syncs positions, holdings, and funds limits for a Dhan account.
func (s *PortfolioSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	authInfo, err := s.Store.AccountAuthInfo(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load account auth info: %w", err)
	}

	clientID := strings.TrimSpace(authInfo.BrokerAccountID)
	if clientID == "" {
		clientID = strings.TrimSpace(authInfo.ApiKey)
	}
	token := strings.TrimSpace(authInfo.AccessToken)

	if clientID == "" || token == "" {
		_ = s.Store.SetAccountAccessToken(ctx, accountID, token, nil, "error", "Missing broker client ID or access token")
		if setter, ok := s.Store.(accountStatusSetter); ok {
			_ = setter.SetAccountStatus(ctx, accountID, domain.AccountStatusError)
		}
		return fmt.Errorf("%w: missing dhan client id or access token", domain.ErrAuthExpired)
	}

	var proxyCfg *ProxyConfig
	if authInfo.IPAddress != "" {
		if pIP, err := s.Store.ProxyIPByAddress(ctx, authInfo.IPAddress); err == nil {
			proxyCfg = &ProxyConfig{
				Scheme:       "https",
				Host:         pIP.Host,
				Port:         pIP.Port,
				ClientID:     pIP.Username,
				ClientSecret: pIP.Password,
			}
		}
	}

	factory := s.APIFactory
	if factory == nil {
		factory = defaultAPIFactory
	}

	api, err := factory(clientID, token, proxyCfg)
	if err != nil {
		return fmt.Errorf("create dhan sync client: %w", err)
	}

	handleAuthFailure := func(err error) error {
		authErrMsg := domain.SanitizeAuthError(err.Error())
		if authErrMsg == "" {
			authErrMsg = "Broker session expired or token invalid"
		}
		_ = s.Store.SetAccountAccessToken(ctx, accountID, token, nil, "error", authErrMsg)
		if setter, ok := s.Store.(accountStatusSetter); ok {
			_ = setter.SetAccountStatus(ctx, accountID, domain.AccountStatusError)
		}
		return err
	}

	// 1. Fetch & Sync Positions
	positions, err := api.GetPositions(ctx)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "no positions") || strings.Contains(msg, "no data") || strings.Contains(msg, "dh-1111") {
			positions = []models.Position{}
		} else {
			return handleAuthFailure(fmt.Errorf("get dhan positions: %w", err))
		}
	}

	posParams := make([]domain.PositionSyncParam, 0, len(positions))
	totalNetQty := 0
	totalMtm := decimal.Zero
	totalRealized := decimal.Zero

	for _, p := range positions {
		netQty := int(p.NetQty)
		totalNetQty += netQty
		mtm := p.UnrealizedProfit
		totalMtm = totalMtm.Add(mtm)
		totalRealized = totalRealized.Add(p.RealizedProfit)
		pnl := p.RealizedProfit.Add(p.UnrealizedProfit)

		posParams = append(posParams, domain.PositionSyncParam{
			Product:      p.ProductType,
			Instrument:   p.TradingSymbol,
			Quantity:     netQty,
			BuyPrice:     p.BuyAvg,
			SellPrice:    p.SellAvg,
			BuyQuantity:  int(p.BuyQty),
			SellQuantity: int(p.SellQty),
			Mtm:          mtm,
			Pnl:          pnl,
			Action:       "exit",
		})
	}
	if err := s.Store.SyncAccountPositions(ctx, accountID, posParams); err != nil {
		return fmt.Errorf("sync positions to store: %w", err)
	}

	// 2. Fetch & Sync Holdings
	holdings, err := api.GetHoldings(ctx)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "no holdings") || strings.Contains(msg, "dh-1111") || strings.Contains(msg, "no data") {
			holdings = []models.Holding{}
		} else {
			return handleAuthFailure(fmt.Errorf("get dhan holdings: %w", err))
		}
	}

	holdParams := make([]domain.HoldingSyncParam, 0, len(holdings))
	for _, h := range holdings {
		holdParams = append(holdParams, domain.HoldingSyncParam{
			Instrument:       h.TradingSymbol,
			SellableQuantity: int(h.AvailableQty),
			BuyAveragePrice:  h.AvgCostPrice,
			Action:           "exit",
		})
	}
	if err := s.Store.SyncAccountHoldings(ctx, accountID, holdParams); err != nil {
		return fmt.Errorf("sync holdings to store: %w", err)
	}

	// 3. Fetch & Sync Fund Limits / Margins
	funds, err := api.GetFundLimits(ctx)
	if err != nil {
		return handleAuthFailure(fmt.Errorf("get dhan fund limits: %w", err))
	}

	availCash := funds.AvailableBalance
	availMargin := funds.AvailableBalance.Add(funds.CollateralAmount)
	accountValue := availMargin.Add(totalMtm)

	marginParam := domain.MarginSyncParam{
		NetQty:          totalNetQty,
		TotalMtm:        totalMtm,
		RealizedPnl:     totalRealized,
		AccountValue:    accountValue,
		Status:          "online",
		AvailableCash:   &availCash,
		AvailableMargin: &availMargin,
	}
	if err := s.Store.SyncAccountMargins(ctx, accountID, marginParam); err != nil {
		return fmt.Errorf("sync margins to store: %w", err)
	}

	// Mark status as authenticated and active, ensuring token expiration is set / preserved
	expiresAt := authInfo.TokenExpiresAt
	if expiresAt == nil || !expiresAt.After(time.Now().UTC()) {
		exp := time.Now().UTC().Add(24 * time.Hour)
		expiresAt = &exp
	}
	if authInfo.AuthStatus != "authenticated" || authInfo.TokenExpiresAt == nil {
		_ = s.Store.SetAccountAccessToken(ctx, accountID, token, expiresAt, "authenticated", "")
	}
	if setter, ok := s.Store.(accountStatusSetter); ok {
		_ = setter.SetAccountStatus(ctx, accountID, domain.AccountStatusActive)
	}

	return nil
}
