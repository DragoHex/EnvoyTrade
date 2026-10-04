package kite

import (
	"log/slog"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"
	"envoytrade/internal/ticker"
)

// TickerStore declares persistence methods needed by TickerManager.
type TickerStore = ticker.Store

// Reconciler declares scoped reconciliation methods needed on reconnect.
type Reconciler = ticker.Reconciler

// TickerManager wraps the generic ticker.Manager preconfigured for Zerodha Kite Connect.
type TickerManager struct {
	*ticker.Manager
	factory *TickerFactory
}

// NewTickerManager initializes a multi-master TickerManager wired with Zerodha Kite Connect support.
func NewTickerManager(
	store TickerStore,
	queue queue.Publisher[domain.MasterFill],
	reconciler Reconciler,
	logger *slog.Logger,
) *TickerManager {
	mgr := ticker.NewManager(store, reconciler, logger)
	factory := NewTickerFactory(queue, logger)
	mgr.RegisterFactory("kite", factory)
	mgr.RegisterFactory("zerodha", factory)

	return &TickerManager{
		Manager: mgr,
		factory: factory,
	}
}

// SetWSRootURL overrides the default WebSocket endpoint (used for testing).
func (m *TickerManager) SetWSRootURL(rootURL string) {
	m.factory.SetWSRootURL(rootURL)
}
