package ticker_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/ticker"

	"github.com/google/uuid"
	"go.uber.org/goleak"
)

type fakeStore struct {
	mu       sync.Mutex
	groups   []domain.GroupSummary
	authInfo map[uuid.UUID]domain.AccountAuthInfo
	active   map[uuid.UUID]bool
	err      error
}

func (s *fakeStore) Groups(ctx context.Context) ([]domain.GroupSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.groups, nil
}

func (s *fakeStore) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.AccountAuthInfo{}, s.err
	}
	info, ok := s.authInfo[id]
	if !ok {
		return domain.AccountAuthInfo{}, domain.ErrNotFound
	}
	return info, nil
}

func (s *fakeStore) MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false, s.err
	}
	return s.active[masterID], nil
}

type fakeReconciler struct {
	mu         sync.Mutex
	reconciled []uuid.UUID
}

func (r *fakeReconciler) ReconcileMaster(ctx context.Context, masterID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconciled = append(r.reconciled, masterID)
	return nil
}

type fakeTicker struct {
	mu      sync.Mutex
	started chan struct{}
	stopped chan struct{}
	closed  bool
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (t *fakeTicker) Start(ctx context.Context) {
	close(t.started)
	<-ctx.Done()
	t.mu.Lock()
	if !t.closed {
		t.closed = true
		close(t.stopped)
	}
	t.mu.Unlock()
}

func (t *fakeTicker) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		close(t.stopped)
	}
	return nil
}

type fakeFactory struct {
	mu        sync.Mutex
	calls     []uuid.UUID
	tickers   map[uuid.UUID]*fakeTicker
	catchUps  map[uuid.UUID]func(context.Context) error
	returnNil bool
	err       error
}

func newFakeFactory() *fakeFactory {
	return &fakeFactory{
		tickers:  make(map[uuid.UUID]*fakeTicker),
		catchUps: make(map[uuid.UUID]func(context.Context) error),
	}
}

func (f *fakeFactory) CreateTicker(
	ctx context.Context,
	masterID uuid.UUID,
	auth domain.AccountAuthInfo,
	catchUp func(context.Context) error,
) (ticker.Ticker, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, masterID)
	if f.err != nil {
		return nil, f.err
	}
	if f.returnNil {
		return nil, nil
	}
	t := newFakeTicker()
	f.tickers[masterID] = t
	if catchUp != nil {
		f.catchUps[masterID] = catchUp
	}
	return t, nil
}

func TestManager_Lifecycle(t *testing.T) {
	defer goleak.VerifyNone(t)

	master1 := uuid.New()
	master2 := uuid.New()

	store := &fakeStore{
		authInfo: map[uuid.UUID]domain.AccountAuthInfo{
			master1: {Broker: "zerodha", BrokerAccountID: "M1", ApiKey: "k1", AccessToken: "tok1"},
			master2: {Broker: "zerodha", BrokerAccountID: "M2", ApiKey: "k2", AccessToken: "tok2"},
		},
		active: map[uuid.UUID]bool{
			master1: true,
			master2: false, // inactive master
		},
		groups: []domain.GroupSummary{
			{ID: uuid.New(), Name: "G1", MasterID: master1},
			{ID: uuid.New(), Name: "G2", MasterID: master2},
		},
	}

	factory := newFakeFactory()
	reconciler := &fakeReconciler{}

	mgr := ticker.NewManager(store, reconciler, nil)
	mgr.RegisterFactory("zerodha", factory)

	ctx := context.Background()

	// 1. Start active master1
	if err := mgr.StartMaster(ctx, master1); err != nil {
		t.Fatalf("StartMaster(master1): %v", err)
	}

	// Wait for ticker Start() to be invoked
	factory.mu.Lock()
	ticker1 := factory.tickers[master1]
	factory.mu.Unlock()
	if ticker1 == nil {
		t.Fatal("expected ticker to be created for master1")
	}
	select {
	case <-ticker1.started:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for ticker1 to start")
	}

	// Idempotent start: calling again should be a no-op
	if err := mgr.StartMaster(ctx, master1); err != nil {
		t.Fatalf("StartMaster duplicate: %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount = %d, want 1", count)
	}

	// 2. Start inactive master2: should be skipped
	if err := mgr.StartMaster(ctx, master2); err != nil {
		t.Fatalf("StartMaster(master2): %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount after inactive master = %d, want 1", count)
	}

	// 3. Restart master1 with refreshed token
	store.mu.Lock()
	store.authInfo[master1] = domain.AccountAuthInfo{Broker: "zerodha", BrokerAccountID: "M1", ApiKey: "k1", AccessToken: "tok1_new"}
	store.mu.Unlock()

	if err := mgr.RestartMaster(ctx, master1); err != nil {
		t.Fatalf("RestartMaster(master1): %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount after restart = %d, want 1", count)
	}

	// 4. Stop master1
	if err := mgr.StopMaster(master1); err != nil {
		t.Fatalf("StopMaster(master1): %v", err)
	}
	if count := mgr.ActiveCount(); count != 0 {
		t.Fatalf("ActiveCount after stop = %d, want 0", count)
	}

	// 5. SyncActiveMasters: should start master1 only
	if err := mgr.SyncActiveMasters(ctx); err != nil {
		t.Fatalf("SyncActiveMasters: %v", err)
	}
	if count := mgr.ActiveCount(); count != 1 {
		t.Fatalf("ActiveCount after SyncActiveMasters = %d, want 1", count)
	}

	// 6. Shutdown
	mgr.Shutdown()
	if count := mgr.ActiveCount(); count != 0 {
		t.Fatalf("ActiveCount after Shutdown = %d, want 0", count)
	}
}

func TestManager_MultiBrokerRouting(t *testing.T) {
	defer goleak.VerifyNone(t)

	kiteMaster := uuid.New()
	customMaster := uuid.New()
	unknownMaster := uuid.New()
	emptyBrokerMaster := uuid.New()

	store := &fakeStore{
		authInfo: map[uuid.UUID]domain.AccountAuthInfo{
			kiteMaster:        {Broker: "Kite", BrokerAccountID: "KM", ApiKey: "k", AccessToken: "t"},
			customMaster:      {Broker: "custom_broker", BrokerAccountID: "CM", ApiKey: "k2", AccessToken: "t2"},
			unknownMaster:     {Broker: "unknown_broker", BrokerAccountID: "UM", ApiKey: "k3", AccessToken: "t3"},
			emptyBrokerMaster: {Broker: "", BrokerAccountID: "EM", ApiKey: "k4", AccessToken: "t4"},
		},
		active: map[uuid.UUID]bool{
			kiteMaster:        true,
			customMaster:      true,
			unknownMaster:     true,
			emptyBrokerMaster: true,
		},
	}

	kiteFactory := newFakeFactory()
	customFactory := newFakeFactory()

	mgr := ticker.NewManager(store, nil, nil)
	mgr.RegisterFactory("kite", kiteFactory)
	mgr.RegisterFactory("zerodha", kiteFactory)
	mgr.RegisterFactory("custom_broker", customFactory)

	ctx := context.Background()

	// 1. Kite Master with casing "Kite" -> routes to kiteFactory
	if err := mgr.StartMaster(ctx, kiteMaster); err != nil {
		t.Fatalf("StartMaster(kiteMaster): %v", err)
	}
	kiteFactory.mu.Lock()
	if len(kiteFactory.calls) != 1 || kiteFactory.calls[0] != kiteMaster {
		t.Fatalf("expected kiteFactory to be called for kiteMaster, got: %v", kiteFactory.calls)
	}
	kiteFactory.mu.Unlock()

	// 2. Custom Master -> routes to customFactory
	if err := mgr.StartMaster(ctx, customMaster); err != nil {
		t.Fatalf("StartMaster(customMaster): %v", err)
	}
	customFactory.mu.Lock()
	if len(customFactory.calls) != 1 || customFactory.calls[0] != customMaster {
		t.Fatalf("expected customFactory to be called for customMaster, got: %v", customFactory.calls)
	}
	customFactory.mu.Unlock()

	// 3. Unknown Broker -> returns error
	err := mgr.StartMaster(ctx, unknownMaster)
	if err == nil {
		t.Fatal("expected error for unknown_broker, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported broker") {
		t.Fatalf("expected error mentioning unsupported broker, got: %v", err)
	}

	// 4. Empty Broker -> defaults to zerodha (which is kiteFactory)
	if err := mgr.StartMaster(ctx, emptyBrokerMaster); err != nil {
		t.Fatalf("StartMaster(emptyBrokerMaster): %v", err)
	}
	kiteFactory.mu.Lock()
	if len(kiteFactory.calls) != 2 || kiteFactory.calls[1] != emptyBrokerMaster {
		t.Fatalf("expected kiteFactory to be called for emptyBrokerMaster default, got: %v", kiteFactory.calls)
	}
	kiteFactory.mu.Unlock()

	if count := mgr.ActiveCount(); count != 3 {
		t.Fatalf("ActiveCount = %d, want 3", count)
	}

	mgr.Shutdown()
}

func TestManager_CatchUpHook(t *testing.T) {
	defer goleak.VerifyNone(t)

	masterID := uuid.New()
	store := &fakeStore{
		authInfo: map[uuid.UUID]domain.AccountAuthInfo{
			masterID: {Broker: "zerodha", ApiKey: "k", AccessToken: "t"},
		},
		active: map[uuid.UUID]bool{
			masterID: true,
		},
	}

	factory := newFakeFactory()
	reconciler := &fakeReconciler{}

	mgr := ticker.NewManager(store, reconciler, nil)
	mgr.RegisterFactory("zerodha", factory)

	ctx := context.Background()
	if err := mgr.StartMaster(ctx, masterID); err != nil {
		t.Fatalf("StartMaster: %v", err)
	}

	factory.mu.Lock()
	hook := factory.catchUps[masterID]
	factory.mu.Unlock()

	if hook == nil {
		t.Fatal("expected catchUp hook to be registered")
	}

	// Trigger hook
	if err := hook(context.Background()); err != nil {
		t.Fatalf("catchUp hook error: %v", err)
	}

	reconciler.mu.Lock()
	if len(reconciler.reconciled) != 1 || reconciler.reconciled[0] != masterID {
		t.Fatalf("expected ReconcileMaster to be called for masterID, got: %v", reconciler.reconciled)
	}
	reconciler.mu.Unlock()

	mgr.Shutdown()
}

func TestManager_MissingCredentialsSkipped(t *testing.T) {
	defer goleak.VerifyNone(t)

	masterID := uuid.New()
	store := &fakeStore{
		authInfo: map[uuid.UUID]domain.AccountAuthInfo{
			masterID: {Broker: "zerodha"}, // missing keys
		},
		active: map[uuid.UUID]bool{
			masterID: true,
		},
	}

	factory := newFakeFactory()
	factory.returnNil = true // simulates unconfigured credentials

	mgr := ticker.NewManager(store, nil, nil)
	mgr.RegisterFactory("zerodha", factory)

	if err := mgr.StartMaster(context.Background(), masterID); err != nil {
		t.Fatalf("expected nil error on missing credentials, got: %v", err)
	}

	if count := mgr.ActiveCount(); count != 0 {
		t.Fatalf("ActiveCount = %d, want 0", count)
	}
}
