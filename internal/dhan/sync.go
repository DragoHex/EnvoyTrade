package dhan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite"

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

// PortfolioSyncer orchestrates Dhan account position, holding, and margin sync.
type PortfolioSyncer struct {
	Store      PortfolioStore
	APIFactory func(clientID, token string, proxyCfg *ProxyConfig) (SyncAPI, error)
}

// defaultAPIFactory constructs a real godhan.Client with optional static proxy routing.
func defaultAPIFactory(clientID, token string, proxyCfg *ProxyConfig) (SyncAPI, error) {
	opts := []godhan.Option{}
	if proxyCfg != nil && proxyCfg.Host != "" {
		kpCfg := kite.ProxyConfig{
			Scheme:       proxyCfg.Scheme,
			Host:         proxyCfg.Host,
			Port:         proxyCfg.Port,
			ClientID:     proxyCfg.ClientID,
			ClientSecret: proxyCfg.ClientSecret,
		}
		proxyClient, err := kite.RESTClientFor(kpCfg)
		if err != nil {
			return nil, fmt.Errorf("dhan: proxy client setup failed: %w", err)
		}
		opts = append(opts, godhan.WithHTTPClient(proxyClient))
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
		_ = s.Store.SetAccountAccessToken(ctx, accountID, token, nil, "error", "missing dhan client id or access token")
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
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "401") || strings.Contains(strings.ToLower(msg), "unauthorized") || strings.Contains(strings.ToLower(msg), "token") {
			_ = s.Store.SetAccountAccessToken(ctx, accountID, token, nil, "error", "dhan session token expired or invalid")
			return fmt.Errorf("%w: %v", domain.ErrAuthExpired, err)
		}
		return err
	}

	// 1. Fetch & Sync Positions
	positions, err := api.GetPositions(ctx)
	if err != nil {
		return handleAuthFailure(fmt.Errorf("get dhan positions: %w", err))
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
		return handleAuthFailure(fmt.Errorf("get dhan holdings: %w", err))
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

	// Mark status as authenticated if previously unauthenticated or errored
	if authInfo.AuthStatus != "authenticated" {
		_ = s.Store.SetAccountAccessToken(ctx, accountID, token, nil, "authenticated", "")
	}

	return nil
}
