package kite

import (
	"context"
	"log/slog"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"
	"envoytrade/internal/kite/ws"
	"envoytrade/internal/queue"
	"envoytrade/internal/ticker"

	"github.com/google/uuid"
)

// TickerFactory produces master WebSocket ticker instances for Zerodha Kite Connect.
type TickerFactory struct {
	queue     queue.Publisher[domain.MasterFill]
	logger    *slog.Logger
	wsRootURL string
}

// NewTickerFactory creates a new Zerodha Kite TickerFactory.
func NewTickerFactory(queue queue.Publisher[domain.MasterFill], logger *slog.Logger) *TickerFactory {
	return &TickerFactory{
		queue:  queue,
		logger: logger,
	}
}

// SetWSRootURL overrides the default WebSocket endpoint (used for testing or custom gateways).
func (f *TickerFactory) SetWSRootURL(rootURL string) {
	f.wsRootURL = rootURL
}

func (f *TickerFactory) log() *slog.Logger {
	if f.logger != nil {
		return f.logger
	}
	return slog.Default()
}

// CreateTicker creates an isolated direct WebSocket ticker for a Zerodha master account.
// If auth credentials (APIKey or AccessToken) are missing, it returns nil, nil to skip.
func (f *TickerFactory) CreateTicker(
	ctx context.Context,
	masterID uuid.UUID,
	auth domain.AccountAuthInfo,
	catchUp func(context.Context) error,
) (ticker.Ticker, error) {
	if auth.ApiKey == "" || auth.AccessToken == "" {
		return nil, nil
	}

	conn := ws.NewConn(ws.Config{
		APIKey:      auth.ApiKey,
		AccessToken: auth.AccessToken,
		RootURL:     f.wsRootURL,
	}, f.logger)

	tickerLogger := f.log().With("component", "master_ticker", "master_id", masterID)
	masterTicker := callback.NewMasterTicker(conn, masterID, f.queue, tickerLogger)

	if catchUp != nil {
		masterTicker.SetCatchUpHook(catchUp)
	}

	return masterTicker, nil
}
