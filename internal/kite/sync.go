package kite

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"envoytrade/internal/crypto"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// PortfolioStore declares the store methods needed for portfolio and credentials sync.
type PortfolioStore interface {
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error
	SyncAccountHoldings(ctx context.Context, accountID uuid.UUID, holdings []domain.HoldingSyncParam) error
	SyncAccountPositions(ctx context.Context, accountID uuid.UUID, positions []domain.PositionSyncParam) error
	SyncAccountMargins(ctx context.Context, accountID uuid.UUID, m domain.MarginSyncParam) error
	ProxyIPByAddress(ctx context.Context, ipAddress string) (domain.ProxyIP, error)
}

// PortfolioSyncer orchestrates login, token verification, and data fetching.
type PortfolioSyncer struct {
	Store            PortfolioStore
	APIFactory       func(apiKey, accessToken string) (API, error)
	LoginFunc        func(ctx context.Context, userID, password, totpSecret, apiKey, apiSecret string) (string, error)
	OnTokenRefreshed func(ctx context.Context, accountID uuid.UUID) error
}

// NextKiteExpiry returns the next 06:00 AM IST cutoff for Kite access tokens.
func NextKiteExpiry(now time.Time) time.Time {
	ist := time.FixedZone("IST", 5*3600+1800)
	nowIST := now.In(ist)
	target := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 6, 0, 0, 0, ist)
	if !nowIST.Before(target) {
		target = target.AddDate(0, 0, 1)
	}
	return target
}

// SyncAccountPortfolio syncs holdings, positions, and margins for an account,
// performing headless login if the access token is missing or expired.
func (s *PortfolioSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	authInfo, err := s.Store.AccountAuthInfo(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load account auth info: %w", err)
	}

	var proxyHTTPClient *http.Client
	if authInfo.IPAddress != "" {
		pIP, err := s.Store.ProxyIPByAddress(ctx, authInfo.IPAddress)
		if err == nil {
			proxyCfg := ProxyConfig{
				Scheme:       "https",
				Host:         pIP.Host,
				Port:         pIP.Port,
				ClientID:     pIP.Username,
				ClientSecret: pIP.Password,
			}
			if client, err := RESTClientFor(proxyCfg); err == nil {
				proxyHTTPClient = client
			}
		}
	}

	accessToken := authInfo.AccessToken
	tokenExpired := accessToken == "" || (authInfo.TokenExpiresAt != nil && !time.Now().Before(*authInfo.TokenExpiresAt))

	if tokenExpired {
		if authInfo.EncryptedPassword == "" || authInfo.EncryptedTotpSecret == "" {
			return errors.New("no active access token or stored credentials")
		}

		password, err := crypto.Decrypt(authInfo.EncryptedPassword)
		if err != nil {
			return fmt.Errorf("decrypt password: %w", err)
		}
		totpSecret, err := crypto.Decrypt(authInfo.EncryptedTotpSecret)
		if err != nil {
			return fmt.Errorf("decrypt totp secret: %w", err)
		}

		loginFn := s.LoginFunc
		if loginFn == nil {
			loginFn = func(ctx context.Context, u, p, t, k, sec string) (string, error) {
				return HeadlessLogin(ctx, proxyHTTPClient, u, p, t, k, sec)
			}
		}

		newToken, loginErr := loginFn(ctx, authInfo.BrokerAccountID, password, totpSecret, authInfo.ApiKey, authInfo.ApiSecret)
		if loginErr != nil {
			_ = s.Store.SetAccountAccessToken(ctx, accountID, "", nil, "error", loginErr.Error())
			return fmt.Errorf("headless login: %w", loginErr)
		}

		exp := NextKiteExpiry(time.Now())
		if err := s.Store.SetAccountAccessToken(ctx, accountID, newToken, &exp, "authenticated", ""); err != nil {
			return fmt.Errorf("persist new access token: %w", err)
		}
		accessToken = newToken

		if s.OnTokenRefreshed != nil {
			_ = s.OnTokenRefreshed(ctx, accountID)
		}
	}

	apiFactory := s.APIFactory
	if apiFactory == nil {
		apiFactory = func(apiKey, token string) (API, error) {
			kc := kiteconnect.New(apiKey)
			kc.SetAccessToken(token)
			if proxyHTTPClient != nil {
				kc.SetHTTPClient(proxyHTTPClient)
			}
			return kc, nil
		}
	}

	client, err := apiFactory(authInfo.ApiKey, accessToken)
	if err != nil {
		return fmt.Errorf("create api client: %w", err)
	}

	holdings, err := client.GetHoldings()
	if err != nil {
		return fmt.Errorf("fetch holdings: %w", err)
	}

	positions, err := client.GetPositions()
	if err != nil {
		return fmt.Errorf("fetch positions: %w", err)
	}

	margins, err := client.GetUserMargins()
	if err != nil {
		return fmt.Errorf("fetch user margins: %w", err)
	}

	// Convert and persist holdings
	holdingParams := make([]domain.HoldingSyncParam, 0, len(holdings))
	for _, h := range holdings {
		conv := ConvertHolding(accountID, h)
		holdingParams = append(holdingParams, domain.HoldingSyncParam{
			Instrument:       conv.Instrument,
			SellableQuantity: conv.SellableQuantity,
			BuyAveragePrice:  conv.BuyAveragePrice,
			Ltp:              conv.Ltp,
			Pnl:              conv.Pnl,
			Action:           conv.Action,
		})
	}
	if err := s.Store.SyncAccountHoldings(ctx, accountID, holdingParams); err != nil {
		return fmt.Errorf("sync holdings: %w", err)
	}

	// Convert and persist positions
	posParams := make([]domain.PositionSyncParam, 0, len(positions.Net))
	for _, p := range positions.Net {
		conv := ConvertPosition(accountID, p)
		posParams = append(posParams, domain.PositionSyncParam{
			Product:      conv.Product,
			Instrument:   conv.Instrument,
			Quantity:     conv.Quantity,
			BuyPrice:     conv.BuyPrice,
			SellPrice:    conv.SellPrice,
			BuyQuantity:  conv.BuyQuantity,
			SellQuantity: conv.SellQuantity,
			Ltp:          conv.Ltp,
			Mtm:          conv.Mtm,
			Pnl:          conv.Pnl,
			Action:       conv.Action,
		})
	}
	if err := s.Store.SyncAccountPositions(ctx, accountID, posParams); err != nil {
		return fmt.Errorf("sync positions: %w", err)
	}

	// Calculate and persist margin metrics
	summary := CalculatePositionMetrics(positions.Net, margins)
	accValue := summary.AvailableMargin
	if accValue.IsZero() && !summary.AvailableCash.IsZero() {
		accValue = summary.AvailableCash
	}
	marginParam := domain.MarginSyncParam{
		NetQty:          summary.NetQty,
		TotalMtm:        summary.TotalMtm,
		RealizedPnl:     summary.RealizedPnl,
		AccountValue:    accValue,
		AvailableCash:   &summary.AvailableCash,
		AvailableMargin: &summary.AvailableMargin,
		Status:          "online",
	}
	if err := s.Store.SyncAccountMargins(ctx, accountID, marginParam); err != nil {
		return fmt.Errorf("sync margins: %w", err)
	}

	return nil
}
