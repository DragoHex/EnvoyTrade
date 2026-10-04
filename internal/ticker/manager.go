package ticker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// Ticker represents an active master order stream.
type Ticker interface {
	Start(ctx context.Context)
	Stop() error
}

// Factory creates a Ticker instance for a master account given credentials and catch-up hook.
type Factory interface {
	CreateTicker(ctx context.Context, masterID uuid.UUID, auth domain.AccountAuthInfo, catchUp func(context.Context) error) (Ticker, error)
}

// Store declares persistence methods needed by Manager.
type Store interface {
	Groups(ctx context.Context) ([]domain.GroupSummary, error)
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error)
}

// Reconciler declares scoped reconciliation methods executed on reconnect.
type Reconciler interface {
	ReconcileMaster(ctx context.Context, masterID uuid.UUID) error
}

type managedTicker struct {
	cancel context.CancelFunc
	ticker Ticker
}

// Manager dynamically manages isolated WebSocket tickers across multiple brokers for master accounts.
type Manager struct {
	store      Store
	reconciler Reconciler
	logger     *slog.Logger

	mu        sync.RWMutex
	factories map[string]Factory
	tickers   map[uuid.UUID]*managedTicker
}

// NewManager initializes a new generic multi-broker Manager.
func NewManager(store Store, reconciler Reconciler, logger *slog.Logger) *Manager {
	return &Manager{
		store:      store,
		reconciler: reconciler,
		logger:     logger,
		factories:  make(map[string]Factory),
		tickers:    make(map[uuid.UUID]*managedTicker),
	}
}

// RegisterFactory registers a Ticker factory for a broker identifier (e.g. "kite", "zerodha").
func (m *Manager) RegisterFactory(brokerName string, factory Factory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[strings.ToLower(strings.TrimSpace(brokerName))] = factory
}

func (m *Manager) log() *slog.Logger {
	if m.logger != nil {
		return m.logger
	}
	return slog.Default()
}

// ActiveCount returns the number of currently running master tickers.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tickers)
}

// StartMaster starts an isolated WebSocket ticker for masterID based on the account's configured broker.
// Calling StartMaster on an already running master is a safe idempotent no-op.
func (m *Manager) StartMaster(ctx context.Context, masterID uuid.UUID) error {
	m.mu.RLock()
	if _, exists := m.tickers[masterID]; exists {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()

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

	brokerKey := strings.ToLower(strings.TrimSpace(authInfo.Broker))
	if brokerKey == "" {
		brokerKey = "zerodha"
	}

	m.mu.RLock()
	factory, ok := m.factories[brokerKey]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("ticker_mgr: unsupported broker %q for master %s", brokerKey, masterID)
	}

	var catchUp func(context.Context) error
	if m.reconciler != nil {
		catchUp = func(catchUpCtx context.Context) error {
			return m.reconciler.ReconcileMaster(catchUpCtx, masterID)
		}
	}

	tickerInst, err := factory.CreateTicker(ctx, masterID, authInfo, catchUp)
	if err != nil {
		return fmt.Errorf("create ticker for broker %q: %w", brokerKey, err)
	}
	if tickerInst == nil {
		m.log().Info("ticker_mgr: missing credentials or unconfigured ticker, skipping start",
			"master_id", masterID,
			"broker", brokerKey,
		)
		return nil
	}

	m.mu.Lock()
	if _, exists := m.tickers[masterID]; exists {
		m.mu.Unlock()
		_ = tickerInst.Stop()
		return nil
	}

	tickerCtx, cancel := context.WithCancel(context.Background())
	m.tickers[masterID] = &managedTicker{
		cancel: cancel,
		ticker: tickerInst,
	}
	m.mu.Unlock()

	m.log().Info("ticker_mgr: starting master ticker",
		"master_id", masterID,
		"broker", brokerKey,
		"broker_user_id", authInfo.BrokerAccountID,
	)

	go func() {
		tickerInst.Start(tickerCtx)
	}()

	return nil
}

// StopMaster stops and removes the ticker for masterID.
func (m *Manager) StopMaster(masterID uuid.UUID) error {
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
	return managed.ticker.Stop()
}

// RestartMaster restarts the ticker for masterID (e.g. after a token refresh).
func (m *Manager) RestartMaster(ctx context.Context, masterID uuid.UUID) error {
	if err := m.StopMaster(masterID); err != nil {
		m.log().Warn("ticker_mgr: error stopping ticker during restart", "master_id", masterID, "error", err)
	}
	return m.StartMaster(ctx, masterID)
}

// SyncActiveMasters scans all master groups from the store and ensures tickers are running for active masters.
func (m *Manager) SyncActiveMasters(ctx context.Context) error {
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
func (m *Manager) Shutdown() {
	m.mu.Lock()
	tickers := m.tickers
	m.tickers = make(map[uuid.UUID]*managedTicker)
	m.mu.Unlock()

	for masterID, managed := range tickers {
		m.log().Info("ticker_mgr: shutting down ticker", "master_id", masterID)
		managed.cancel()
		_ = managed.ticker.Stop()
	}
}
