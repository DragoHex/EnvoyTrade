package testbroker

import (
	"context"
	"log/slog"
	"os"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"
	"envoytrade/internal/kite/ws"
	"envoytrade/internal/queue"
	"envoytrade/internal/ticker"

	"github.com/google/uuid"
)

const defaultWSRootURL = "ws://localhost:8089"

// TickerFactory implements ticker.Factory for testbroker master accounts.
type TickerFactory struct {
	queue     queue.Publisher[domain.MasterFill]
	logger    *slog.Logger
	wsRootURL string
}

// NewTickerFactory creates a new testbroker TickerFactory.
func NewTickerFactory(q queue.Publisher[domain.MasterFill], logger *slog.Logger) *TickerFactory {
	rootURL := os.Getenv("TESTBROKER_WS_URL")
	if rootURL == "" {
		rootURL = defaultWSRootURL
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TickerFactory{
		queue:     q,
		logger:    logger,
		wsRootURL: rootURL,
	}
}

// SetWSRootURL overrides the default WebSocket endpoint (used for testing).
func (f *TickerFactory) SetWSRootURL(rootURL string) {
	f.wsRootURL = rootURL
}

// WSRootURL returns the currently configured WebSocket endpoint.
func (f *TickerFactory) WSRootURL() string {
	return f.wsRootURL
}

// CreateTicker instantiates an isolated master ticker streaming fills from the testbroker.
func (f *TickerFactory) CreateTicker(
	ctx context.Context,
	masterID uuid.UUID,
	auth domain.AccountAuthInfo,
	catchUp func(context.Context) error,
) (ticker.Ticker, error) {
	if auth.ApiKey == "" || auth.AccessToken == "" {
		return nil, nil // skip unconfigured credentials gracefully
	}

	conn := ws.NewConn(ws.Config{
		APIKey:      auth.ApiKey,
		AccessToken: auth.AccessToken,
		RootURL:     f.wsRootURL,
	}, f.logger)

	tickerLogger := f.logger.With("component", "testbroker_ticker", "master_id", masterID)
	masterTicker := callback.NewMasterTicker(conn, masterID, f.queue, tickerLogger)
	if catchUp != nil {
		masterTicker.SetCatchUpHook(catchUp)
	}

	return masterTicker, nil
}
