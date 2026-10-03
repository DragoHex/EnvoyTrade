package kite

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"
	"envoytrade/internal/kite/ws"
	"envoytrade/internal/queue"

	"github.com/google/uuid"
)

// TickerStore declares persistence methods needed by TickerManager.
type TickerStore interface {
	Groups(ctx context.Context) ([]domain.GroupSummary, error)
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error)
}

// Reconciler declares scoped reconciliation methods needed on reconnect.
type Reconciler interface {
	ReconcileMaster(ctx context.Context, masterID uuid.UUID) error
}

type managedTicker struct {
	cancel context.CancelFunc
	ticker *callback.MasterTicker
	conn   *ws.Conn
}

// TickerManager dynamically manages isolated WebSocket tickers for multiple master accounts.
type TickerManager struct {
	store      TickerStore
	queue      queue.Publisher[domain.MasterFill]
	reconciler Reconciler
	logger     *slog.Logger
	wsRootURL  string

	mu      sync.RWMutex
	tickers map[uuid.UUID]*managedTicker
}

// NewTickerManager initializes a new multi-master TickerManager.
func NewTickerManager(
	store TickerStore,
	queue queue.Publisher[domain.MasterFill],
	reconciler Reconciler,
	logger *slog.Logger,
) *TickerManager {
	return &TickerManager{
		store:      store,
		queue:      queue,
		reconciler: reconciler,
		logger:     logger,
		tickers:    make(map[uuid.UUID]*managedTicker),
	}
}

// SetWSRootURL overrides the default WebSocket endpoint (used for testing).
func (m *TickerManager) SetWSRootURL(rootURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wsRootURL = rootURL
}

func (m *TickerManager) log() *slog.Logger {
	if m.logger != nil {
		return m.logger
	}
	return slog.Default()
}

// ActiveCount returns the number of currently running master tickers.
func (m *TickerManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tickers)
}

// StartMaster starts an isolated direct WebSocket ticker for masterID.
// Calling StartMaster on an already running master is a safe no-op.
func (m *TickerManager) StartMaster(ctx context.Context, masterID uuid.UUID) error {
	m.mu.Lock()
	if _, exists := m.tickers[masterID]; exists {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	active, err := m.store.MasterActive(ctx, masterID)
	if err != nil {
		return fmt.Errorf("check master active: %w", err)
	}
	if !active {
		m.log().Info("ticker_mgr: master inactive, skipping ticker start", "master_id", masterID)
		return nil
	}

	authInfo, err := m.store.AccountAuthInfo(ctx, masterID)
	if err != nil {
		return fmt.Errorf("load account auth info: %w", err)
	}
	if authInfo.ApiKey == "" || authInfo.AccessToken == "" {
		m.log().Info("ticker_mgr: missing api_key or access_token, skipping ticker start", "master_id", masterID)
		return nil
	}

	m.mu.Lock()
	if _, exists := m.tickers[masterID]; exists {
		m.mu.Unlock()
		return nil
	}

	m.log().Info("ticker_mgr: starting direct master ticker",
		"master_id", masterID,
		"broker_user_id", authInfo.BrokerAccountID,
	)

	conn := ws.NewConn(ws.Config{
		APIKey:      authInfo.ApiKey,
		AccessToken: authInfo.AccessToken,
		RootURL:     m.wsRootURL,
	}, m.logger)

	tickerLogger := m.log().With("component", "master_ticker", "master_id", masterID)
	masterTicker := callback.NewMasterTicker(conn, masterID, m.queue, tickerLogger)

	if m.reconciler != nil {
		masterTicker.SetCatchUpHook(func(catchUpCtx context.Context) error {
			return m.reconciler.ReconcileMaster(catchUpCtx, masterID)
		})
	}

	tickerCtx, cancel := context.WithCancel(context.Background())
	managed := &managedTicker{
		cancel: cancel,
		ticker: masterTicker,
		conn:   conn,
	}
	m.tickers[masterID] = managed
	m.mu.Unlock()

	go func() {
		masterTicker.Start(tickerCtx)
	}()

	return nil
}

// StopMaster stops and removes the ticker for masterID.
func (m *TickerManager) StopMaster(masterID uuid.UUID) error {
	m.mu.Lock()
	managed, exists := m.tickers[masterID]
	if !exists {
		m.mu.Unlock()
		return nil
	}
	delete(m.tickers, masterID)
	m.mu.Unlock()

	m.log().Info("ticker_mgr: stopping master ticker", "master_id", masterID)
	managed.cancel()
	return managed.conn.Close()
}

// RestartMaster restarts the ticker for masterID (e.g. after a token refresh).
func (m *TickerManager) RestartMaster(ctx context.Context, masterID uuid.UUID) error {
	if err := m.StopMaster(masterID); err != nil {
		m.log().Warn("ticker_mgr: error stopping ticker during restart", "master_id", masterID, "error", err)
	}
	return m.StartMaster(ctx, masterID)
}

// SyncActiveMasters scans all master groups from the store and ensures tickers are running for active masters.
func (m *TickerManager) SyncActiveMasters(ctx context.Context) error {
	groups, err := m.store.Groups(ctx)
	if err != nil {
		return fmt.Errorf("load groups: %w", err)
	}

	for _, g := range groups {
		if err := m.StartMaster(ctx, g.MasterID); err != nil {
			m.log().Error("ticker_mgr: failed to start ticker for master group",
				"group_id", g.ID,
				"master_id", g.MasterID,
				"error", err,
			)
		}
	}
	return nil
}

// Shutdown stops all active master tickers cleanly.
func (m *TickerManager) Shutdown() {
	m.mu.Lock()
	tickers := m.tickers
	m.tickers = make(map[uuid.UUID]*managedTicker)
	m.mu.Unlock()

	for masterID, managed := range tickers {
		m.log().Info("ticker_mgr: shutting down ticker", "master_id", masterID)
		managed.cancel()
		_ = managed.conn.Close()
	}
}
