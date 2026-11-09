package kotak

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"envoytrade/internal/crypto"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

// RenewerStore defines the store interface needed for Kotak Neo session renewal.
type RenewerStore interface {
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error)
	SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error
}

// Renewer handles scheduled 06:30 AM IST daily session renewals for Kotak Neo accounts.
type Renewer struct {
	Store            RenewerStore
	LoginFunc        func(ctx context.Context, creds HeadlessLoginCredentials) (*TradeSessionJSON, error)
	OnTokenRefreshed func(ctx context.Context, accountID uuid.UUID) error
	Logger           *slog.Logger

	flight singleflight.Group
}

// NewRenewer constructs a Renewer for Kotak Neo accounts.
func NewRenewer(store RenewerStore, logger *slog.Logger) *Renewer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Renewer{
		Store:     store,
		LoginFunc: HeadlessLogin,
		Logger:    logger.With("component", "kotak.renewer"),
	}
}

// Start spawns the daily renewal background loop.
func (r *Renewer) Start(ctx context.Context) {
	go func() {
		for {
			nextRun := NextRenewerRunTime(time.Now())
			wait := time.Until(nextRun)
			r.Logger.Info("scheduled next Kotak Neo renewal run", "at", nextRun.Format(time.RFC3339), "wait", wait.Round(time.Second))

			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				if err := r.RenewAll(ctx); err != nil {
					r.Logger.Error("Kotak Neo renewal run failed", "error", err)
				}
			}
		}
	}()
}

// RenewAll iterates over all active Kotak Neo accounts and renews their sessions.
func (r *Renewer) RenewAll(ctx context.Context) error {
	accounts, err := r.Store.Accounts(ctx, nil)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}

	for _, acc := range accounts {
		if strings.ToLower(acc.Broker) != "kotakneo" && strings.ToLower(acc.Broker) != "kotak" {
			continue
		}
		status := strings.ToLower(acc.Status)
		if status != "active" && status != "ok" && status != "pending" {
			continue
		}

		if err := r.RenewAccount(ctx, acc.ID); err != nil {
			r.Logger.Error("failed to renew Kotak Neo account", "account_id", acc.ID, "account", acc.BrokerAccountID, "error", err)
		}
	}

	return nil
}

// RenewAccount renews session tokens for a single Kotak Neo account with singleflight deduplication.
func (r *Renewer) RenewAccount(ctx context.Context, accountID uuid.UUID) error {
	key := accountID.String()
	_, err, _ := r.flight.Do(key, func() (interface{}, error) {
		return nil, r.renewAccountInternal(ctx, accountID)
	})
	return err
}

func (r *Renewer) renewAccountInternal(ctx context.Context, accountID uuid.UUID) error {
	authInfo, err := r.Store.AccountAuthInfo(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get account auth info: %w", err)
	}

	if len(authInfo.EncryptedPassword) == 0 || len(authInfo.EncryptedTotpSecret) == 0 {
		_ = r.Store.SetAccountAccessToken(ctx, accountID, "", nil, "unauthenticated", "missing MPIN or TOTP secret")
		return fmt.Errorf("%w: missing MPIN or TOTP secret", ErrInvalidInput)
	}

	mpin, err := crypto.Decrypt(authInfo.EncryptedPassword)
	if err != nil {
		return fmt.Errorf("decrypt MPIN: %w", err)
	}

	totpSecret, err := crypto.Decrypt(authInfo.EncryptedTotpSecret)
	if err != nil {
		return fmt.Errorf("decrypt TOTP secret: %w", err)
	}

	var proxyURL string
	if authInfo.IPAddress != "" {
		if proxy, err := r.Store.ProxyIPByAddress(ctx, authInfo.IPAddress); err == nil {
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

	creds := HeadlessLoginCredentials{
		ConsumerKey: authInfo.ApiSecret,
		Mobile:      authInfo.ApiKey,
		UCC:         authInfo.BrokerAccountID,
		MPIN:        mpin,
		TOTPSecret:  totpSecret,
		ProxyURL:    proxyURL,
	}

	var session *TradeSessionJSON
	var loginErr error

	// Retry up to 3 times for transient network errors
	for attempt := 1; attempt <= 3; attempt++ {
		session, loginErr = r.LoginFunc(ctx, creds)
		if loginErr == nil {
			break
		}
		if errors.Is(loginErr, domain.ErrAuthExpired) || errors.Is(loginErr, ErrInvalidInput) {
			break // Non-retryable
		}
		if attempt < 3 {
			time.Sleep(5 * time.Second)
		}
	}

	if loginErr != nil {
		authStatus := "unauthenticated"
		if errors.Is(loginErr, domain.ErrBrokerUnreachable) {
			authStatus = authInfo.AuthStatus // Retain status on broker unreachable
		}
		_ = r.Store.SetAccountAccessToken(ctx, accountID, "", nil, authStatus, loginErr.Error())
		return fmt.Errorf("login failed: %w", loginErr)
	}

	sessionRaw, err := SerializeTradeSession(session)
	if err != nil {
		return fmt.Errorf("serialize session: %w", err)
	}

	expiry := NextKotakExpiry(time.Now())
	if err := r.Store.SetAccountAccessToken(ctx, accountID, sessionRaw, &expiry, "authenticated", ""); err != nil {
		return fmt.Errorf("save account session: %w", err)
	}

	if r.OnTokenRefreshed != nil {
		if err := r.OnTokenRefreshed(ctx, accountID); err != nil {
			r.Logger.Warn("OnTokenRefreshed callback error", "account_id", accountID, "error", err)
		}
	}

	r.Logger.Info("Kotak Neo account session renewed successfully", "account_id", accountID, "expires_at", expiry.Format(time.RFC3339))
	return nil
}
