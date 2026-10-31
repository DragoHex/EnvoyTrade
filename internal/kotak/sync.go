package kotak

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"envoytrade/internal/domain"

	gokotak "github.com/DragoHex/gokotakneo"
	"github.com/DragoHex/gokotakneo/models"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SyncAPI narrows the Kotak Neo API methods needed for portfolio and balances synchronization.
type SyncAPI interface {
	GetPositions(ctx context.Context) ([]models.Position, error)
	GetHoldings(ctx context.Context) ([]models.Holding, error)
	GetLimits(ctx context.Context, seg, exch, prod string) (*models.Limits, error)
}

// PortfolioStore declares the store methods needed for portfolio synchronization.
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

// PortfolioSyncer orchestrates Kotak Neo account positions, holdings, and limits sync.
type PortfolioSyncer struct {
	Store            PortfolioStore
	APIFactory       func(consumerKey string, sess *TradeSessionJSON, proxyURL string) (SyncAPI, error)
	Renewer          *Renewer
	OnTokenRefreshed func(ctx context.Context, accountID uuid.UUID) error
}

func defaultSyncAPIFactory(consumerKey string, sess *TradeSessionJSON, proxyURL string) (SyncAPI, error) {
	var opts []gokotak.Option
	if strings.TrimSpace(proxyURL) != "" {
		opts = append(opts, gokotak.WithProxyURL(strings.TrimSpace(proxyURL)))
	}
	client := gokotak.New(consumerKey, opts...)
	if sess != nil {
		client.SetGateways(sess.BaseURL, sess.FeedURL, sess.RTURL)
		client.SetSessionTokens(sess.Token, sess.Sid)
	}
	return client, nil
}

// SyncAccountPortfolio syncs positions, holdings, and limits for a Kotak Neo account.
func (s *PortfolioSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	authInfo, err := s.Store.AccountAuthInfo(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load account auth info: %w", err)
	}

	consumerKey := strings.TrimSpace(authInfo.ApiSecret)
	tokenRaw := strings.TrimSpace(authInfo.AccessToken)

	var sess *TradeSessionJSON
	if tokenRaw != "" {
		sess, _ = DeserializeTradeSession(tokenRaw)
	}

	// Trigger headless login if session is absent, expired, or invalid
	needsLogin := sess == nil || sess.Token == "" || (authInfo.TokenExpiresAt != nil && time.Now().After(*authInfo.TokenExpiresAt))
	var renewErr error
	if needsLogin && s.Renewer != nil {
		if renewErr = s.Renewer.RenewAccount(ctx, accountID); renewErr == nil {
			if freshAuth, err := s.Store.AccountAuthInfo(ctx, accountID); err == nil {
				authInfo = freshAuth
				sess, _ = DeserializeTradeSession(authInfo.AccessToken)
			}
		}
	}

	if sess == nil || sess.Token == "" {
		authErrMsg := "Missing or expired Kotak Neo session"
		if renewErr != nil {
			authErrMsg = domain.SanitizeAuthError(renewErr.Error())
		}
		_ = s.Store.SetAccountAccessToken(ctx, accountID, "", nil, "unauthenticated", authErrMsg)
		if setter, ok := s.Store.(accountStatusSetter); ok {
			_ = setter.SetAccountStatus(ctx, accountID, domain.AccountStatusError)
		}
		if renewErr != nil {
			return fmt.Errorf("%w: %s", domain.ErrAuthExpired, renewErr.Error())
		}
		return fmt.Errorf("%w: missing or expired session for account %s", domain.ErrAuthExpired, accountID)
	}

	var proxyURL string
	if authInfo.IPAddress != "" {
		if proxy, err := s.Store.ProxyIPByAddress(ctx, authInfo.IPAddress); err == nil {
			u := &url.URL{
				Scheme: "https",
				Host:   proxy.Host + ":" + strconv.Itoa(proxy.Port),
			}
			if proxy.Username != "" {
				u.User = url.UserPassword(proxy.Username, proxy.Password)
			}
			proxyURL = u.String()
		}
	}

	factory := s.APIFactory
	if factory == nil {
		factory = defaultSyncAPIFactory
	}

	api, err := factory(consumerKey, sess, proxyURL)
	if err != nil {
		return fmt.Errorf("create sync api client: %w", err)
	}

	// 1. Sync Positions
	kPositions, err := api.GetPositions(ctx)
	if err != nil {
		// Attempt transparent re-login on 401
		if strings.Contains(strings.ToLower(err.Error()), "401") || strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
			if s.Renewer != nil {
				if rErr := s.Renewer.RenewAccount(ctx, accountID); rErr == nil {
					return s.SyncAccountPortfolio(ctx, accountID) // retry once with renewed credentials
				}
			}
		}
		s.handleSyncFailure(ctx, accountID, err)
		return fmt.Errorf("get positions: %w", translateBrokerError(err))
	}

	posParams := make([]domain.PositionSyncParam, 0, len(kPositions))
	for _, p := range kPositions {
		buyPrice := decimal.Zero
		if p.FilledBuyQty > 0 && !p.BuyAmount.IsZero() {
			buyPrice = p.BuyAmount.Div(decimal.NewFromInt(int64(p.FilledBuyQty)))
		}
		sellPrice := decimal.Zero
		if p.FilledSellQty > 0 && !p.SellAmount.IsZero() {
			sellPrice = p.SellAmount.Div(decimal.NewFromInt(int64(p.FilledSellQty)))
		}

		posParams = append(posParams, domain.PositionSyncParam{
			Product:    p.Product,
			Instrument: p.TradingSymbol,
			Quantity:   p.Quantity,
			BuyPrice:   buyPrice,
			SellPrice:  sellPrice,
			Ltp:        decimal.Zero,
			Mtm:        decimal.Zero,
			Pnl:        decimal.Zero,
		})
	}

	if err := s.Store.SyncAccountPositions(ctx, accountID, posParams); err != nil {
		return fmt.Errorf("sync positions to store: %w", err)
	}

	// 2. Sync Holdings
	kHoldings, err := api.GetHoldings(ctx)
	if err != nil {
		s.handleSyncFailure(ctx, accountID, err)
		return fmt.Errorf("get holdings: %w", translateBrokerError(err))
	}

	holdParams := make([]domain.HoldingSyncParam, 0, len(kHoldings))
	for _, h := range kHoldings {
		symbol := h.CommonScripCode
		if symbol == "" {
			symbol = h.InstrumentName
		}
		holdParams = append(holdParams, domain.HoldingSyncParam{
			Instrument:       symbol,
			SellableQuantity: h.Quantity,
			BuyAveragePrice:  h.AveragePrice,
			Ltp:              h.ClosingPrice,
			Pnl:              h.UnrealisedGainLoss,
		})
	}

	if err := s.Store.SyncAccountHoldings(ctx, accountID, holdParams); err != nil {
		return fmt.Errorf("sync holdings to store: %w", err)
	}

	// 3. Sync Limits & Margins
	kLimits, err := api.GetLimits(ctx, "ALL", "ALL", "ALL")
	if err != nil {
		s.handleSyncFailure(ctx, accountID, err)
		return fmt.Errorf("get limits: %w", translateBrokerError(err))
	}

	availCash := kLimits.NotionalCash
	availMargin := kLimits.Net

	marginParam := domain.MarginSyncParam{
		AvailableCash:   &availCash,
		AvailableMargin: &availMargin,
		TotalMtm:        decimal.Zero,
		Status:          "online",
	}

	if err := s.Store.SyncAccountMargins(ctx, accountID, marginParam); err != nil {
		return fmt.Errorf("sync margins to store: %w", err)
	}

	// Mark account active and authenticated on success
	_ = s.Store.SetAccountAccessToken(ctx, accountID, authInfo.AccessToken, authInfo.TokenExpiresAt, "authenticated", "")
	if setter, ok := s.Store.(accountStatusSetter); ok {
		_ = setter.SetAccountStatus(ctx, accountID, domain.AccountStatusActive)
	}

	return nil
}

func (s *PortfolioSyncer) handleSyncFailure(ctx context.Context, accountID uuid.UUID, err error) {
	authStatus := "error"
	if strings.Contains(strings.ToLower(err.Error()), "401") || strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
		authStatus = "unauthenticated"
	}
	cleanMsg := domain.SanitizeAuthError(err.Error())
	_ = s.Store.SetAccountAccessToken(ctx, accountID, "", nil, authStatus, cleanMsg)
	if setter, ok := s.Store.(accountStatusSetter); ok {
		_ = setter.SetAccountStatus(ctx, accountID, domain.AccountStatusError)
	}
}
