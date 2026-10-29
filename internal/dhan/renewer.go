package dhan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan"
	"github.com/google/uuid"
)

// NextFixedRunTime calculates the next fixed run time between 12:15 AM (00:15 IST) and 6:00 AM (06:00 IST).
func NextFixedRunTime(now time.Time) time.Time {
	nowIST := now.In(domain.IST)
	today0015 := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 0, 15, 0, 0, domain.IST)
	today0600 := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 6, 0, 0, 0, domain.IST)

	if nowIST.Before(today0015) {
		return today0015
	}
	if nowIST.Before(today0600) {
		return today0600
	}
	return today0015.AddDate(0, 0, 1)
}

// RenewerStore declares the store interface needed for token renewal (PLAN.md §1 seam rule).
type RenewerStore interface {
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error)
	SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrAuthExpired) {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "dh-901") || strings.Contains(msg, "dh-906") ||
		strings.Contains(msg, "invalid token") || strings.Contains(msg, "token expired") ||
		strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") {
		return false
	}
	return true
}

// Renewer handles scheduled 24h JWT token renewals for Dhan accounts.
type Renewer struct {
	Store            RenewerStore
	RenewFunc        func(ctx context.Context, clientID, token string, proxyCfg *ProxyConfig) (string, time.Time, error)
	OnTokenRefreshed func(ctx context.Context, accountID uuid.UUID) error
	Logger           *slog.Logger
	MaxRetries       int
	RetryInterval    time.Duration
}

// RenewToken calls DhanHQ API via godhan.Client to refresh an active access token.
func RenewToken(ctx context.Context, clientID, token string, proxyCfg *ProxyConfig, extraOpts ...godhan.Option) (string, time.Time, error) {
	clientID = strings.TrimSpace(clientID)
	token = strings.TrimSpace(token)
	if clientID == "" {
		return "", time.Time{}, fmt.Errorf("%w: missing client id", domain.ErrAuthExpired)
	}
	if token == "" {
		return "", time.Time{}, fmt.Errorf("%w: missing access token", domain.ErrAuthExpired)
	}

	opts := BuildClientOptions(proxyCfg, extraOpts...)
	client := godhan.NewClient(clientID, token, opts...)
	resp, err := client.RenewToken(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	if resp == nil {
		return "", time.Time{}, fmt.Errorf("%w: nil response from broker", domain.ErrBrokerUnreachable)
	}

	newToken := strings.TrimSpace(resp.AccessToken)
	if newToken == "" {
		newToken = strings.TrimSpace(resp.Token)
	}
	if newToken == "" {
		return "", time.Time{}, fmt.Errorf("%w: broker renewal returned empty access token", domain.ErrAuthExpired)
	}

	var claims *DhanClaims
	if strings.Count(newToken, ".") == 2 {
		var claimErr error
		claims, claimErr = ParseDhanJWT(newToken)
		if claimErr != nil {
			return "", time.Time{}, fmt.Errorf("%w: invalid renewed jwt: %v", domain.ErrAuthExpired, claimErr)
		}
		if clientID != "" && claims.DhanClientID != "" && claims.DhanClientID != clientID {
			return "", time.Time{}, fmt.Errorf("%w: renewed token client id mismatch (got %s, want %s)", domain.ErrAuthExpired, claims.DhanClientID, clientID)
		}
	}

	expiresAt := resp.ExpiryTime.Time
	if !expiresAt.IsZero() {
		expiresAt = expiresAt.In(domain.IST)
	} else if claims != nil && !claims.ExpiresAt.IsZero() {
		expiresAt = claims.ExpiresAt.In(domain.IST)
	} else {
		expiresAt = domain.NowIST().Add(24 * time.Hour)
	}
	return newToken, expiresAt, nil
}

// NewRenewer constructs a Renewer with default settings.
func NewRenewer(store RenewerStore, logger *slog.Logger) *Renewer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Renewer{
		Store: store,
		RenewFunc: func(ctx context.Context, clientID, token string, proxyCfg *ProxyConfig) (string, time.Time, error) {
			return RenewToken(ctx, clientID, token, proxyCfg)
		},
		Logger:        logger,
		MaxRetries:    3,
		RetryInterval: 2 * time.Minute,
	}
}

// RenewAll runs a renewal cycle across all active Dhan accounts.
func (r *Renewer) RenewAll(ctx context.Context) error {
	accs, err := r.Store.Accounts(ctx, nil)
	if err != nil {
		return fmt.Errorf("load accounts: %w", err)
	}

	for _, a := range accs {
		if !a.Active || !strings.EqualFold(a.Broker, "dhan") {
			continue
		}

		authInfo, err := r.Store.AccountAuthInfo(ctx, a.ID)
		if err != nil {
			r.Logger.Error("dhan renew: load auth info failed", "account_id", a.ID, "error", err)
			continue
		}
		if strings.TrimSpace(authInfo.AccessToken) == "" {
			continue
		}

		var proxyCfg *ProxyConfig
		if authInfo.IPAddress != "" {
			if pIP, err := r.Store.ProxyIPByAddress(ctx, authInfo.IPAddress); err == nil {
				proxyCfg = &ProxyConfig{
					Scheme:       "https",
					Host:         pIP.Host,
					Port:         pIP.Port,
					ClientID:     pIP.Username,
					ClientSecret: pIP.Password,
				}
			}
		}

		clientID := strings.TrimSpace(authInfo.BrokerAccountID)
		if clientID == "" {
			clientID = strings.TrimSpace(authInfo.ApiKey)
		}
		if clientID == "" {
			r.Logger.Error("dhan renew: missing client id", "account_id", a.ID)
			_ = r.Store.SetAccountAccessToken(ctx, a.ID, authInfo.AccessToken, authInfo.TokenExpiresAt, "error", "Missing broker client ID")
			if setter, ok := r.Store.(accountStatusSetter); ok {
				_ = setter.SetAccountStatus(ctx, a.ID, domain.AccountStatusError)
			}
			continue
		}

		var lastErr error
		for attempt := 1; attempt <= r.MaxRetries; attempt++ {
			newToken, expiresAt, err := r.RenewFunc(ctx, clientID, authInfo.AccessToken, proxyCfg)
			newToken = strings.TrimSpace(newToken)
			if err == nil && newToken != "" {
				if saveErr := r.Store.SetAccountAccessToken(ctx, a.ID, newToken, &expiresAt, "authenticated", ""); saveErr != nil {
					r.Logger.Error("dhan renew: save token failed", "account_id", a.ID, "error", saveErr)
					lastErr = saveErr
					break
				}
				if setter, ok := r.Store.(accountStatusSetter); ok {
					_ = setter.SetAccountStatus(ctx, a.ID, domain.AccountStatusActive)
				}
				r.Logger.Info("dhan renew: token renewed successfully", "account_id", a.ID, "expires_at", expiresAt)
				if r.OnTokenRefreshed != nil {
					_ = r.OnTokenRefreshed(ctx, a.ID)
				}
				lastErr = nil
				break
			}

			if err == nil && newToken == "" {
				err = fmt.Errorf("%w: renewal returned empty token", domain.ErrAuthExpired)
			}

			lastErr = err
			r.Logger.Warn("dhan renew: attempt failed", "account_id", a.ID, "attempt", attempt, "error", err)

			if !isRetryable(err) {
				break
			}

			if attempt < r.MaxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(r.RetryInterval):
				}
			}
		}

		if lastErr != nil {
			_ = r.Store.SetAccountAccessToken(ctx, a.ID, authInfo.AccessToken, authInfo.TokenExpiresAt, "error", domain.SanitizeAuthError(lastErr.Error()))
			if setter, ok := r.Store.(accountStatusSetter); ok {
				_ = setter.SetAccountStatus(ctx, a.ID, domain.AccountStatusError)
			}
		}
	}
	return nil
}

// Run executes the renewal loop at 00:15 IST and 06:00 IST until ctx is canceled.
func (r *Renewer) Run(ctx context.Context) error {
	for {
		nextRun := NextFixedRunTime(time.Now())
		waitDur := time.Until(nextRun)
		r.Logger.Info("dhan renew: scheduled next run", "target", nextRun.Format("2006-01-02 15:04:05 IST"), "wait", waitDur)

		timer := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			if err := r.RenewAll(ctx); err != nil && !errors.Is(err, context.Canceled) {
				r.Logger.Error("dhan renew: cycle failed", "error", err)
			}
		}
	}
}
