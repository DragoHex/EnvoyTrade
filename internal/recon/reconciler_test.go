package recon_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/recon"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

type fakeStore struct {
	mu           sync.Mutex
	accounts     []domain.Account
	accountsErr  error
	pending      []domain.FollowerOrder
	pendingErr   error
	updatedOrder map[string]string // brokerOrderID -> status
	events       []domain.OrderEvent
	sweptIDs         []string
	sweepErr         error
	masterFillsExist    map[string]bool
	followerOrdersExist map[string]bool
	failedOrders        map[int64]string
}

func (s *fakeStore) MasterFillExists(ctx context.Context, masterID uuid.UUID, brokerOrderID string, filledQty int, status string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.masterFillsExist == nil {
		return false, nil
	}
	return s.masterFillsExist[masterID.String()+":"+brokerOrderID], nil
}

func (s *fakeStore) FollowerOrderTerminalExists(ctx context.Context, followerID uuid.UUID, brokerOrderID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.followerOrdersExist == nil {
		return false, nil
	}
	return s.followerOrdersExist[followerID.String()+":"+brokerOrderID], nil
}

func (s *fakeStore) UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failedOrders == nil {
		s.failedOrders = make(map[int64]string)
	}
	s.failedOrders[id] = errMsg
	return nil
}

func (s *fakeStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accounts, s.accountsErr
}

func (s *fakeStore) PendingFollowerOrders(ctx context.Context, cutoff time.Time) ([]domain.FollowerOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending, s.pendingErr
}

func (s *fakeStore) UpdateFollowerOrderStatus(ctx context.Context, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.updatedOrder == nil {
		s.updatedOrder = make(map[string]string)
	}
	s.updatedOrder[brokerOrderID] = status
	return 1, nil
}

func (s *fakeStore) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

func (s *fakeStore) SweepPendingOrderUpdates(ctx context.Context, cutoff time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sweptIDs, s.sweepErr
}

type fakeMasterReader struct {
	mu     sync.Mutex
	orders map[uuid.UUID][]kiteconnect.Order
	err    error
}

func (r *fakeMasterReader) GetMasterOrders(ctx context.Context, masterID uuid.UUID) ([]kiteconnect.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return r.orders[masterID], nil
}

type fakeFollowerReader struct {
	mu      sync.Mutex
	history map[string][]kiteconnect.Order
	orders  map[uuid.UUID][]kiteconnect.Order
	err     error
}

func (r *fakeFollowerReader) GetFollowerOrderHistory(ctx context.Context, followerID uuid.UUID, brokerOrderID string) ([]kiteconnect.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return r.history[brokerOrderID], nil
}

func (r *fakeFollowerReader) GetFollowerOrders(ctx context.Context, followerID uuid.UUID) ([]kiteconnect.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return r.orders[followerID], nil
}

type fakeFollowerConsumer struct {
	mu      sync.Mutex
	handled []domain.OrderUpdate
	err     error
}

func (c *fakeFollowerConsumer) Handle(ctx context.Context, upd domain.OrderUpdate) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handled = append(c.handled, upd)
	return c.err
}

type fakeConsumer struct {
	mu      sync.Mutex
	handled []domain.MasterFill
	err     error
}

func (c *fakeConsumer) Handle(ctx context.Context, fill domain.MasterFill) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handled = append(c.handled, fill)
	return c.err
}

type fakeAlerter struct {
	mu     sync.Mutex
	alerts []string
}

func (a *fakeAlerter) Alert(ctx context.Context, msg string, details map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alerts = append(a.alerts, msg)
}

func (a *fakeAlerter) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.alerts)
}

func TestPoller_BackfillsMasterOrderGap(t *testing.T) {
	masterID := uuid.New()
	store := &fakeStore{
		accounts: []domain.Account{
			{ID: masterID, Role: "master", Active: true},
		},
	}
	masterReader := &fakeMasterReader{
		orders: map[uuid.UUID][]kiteconnect.Order{
			masterID: {
				{
					OrderID:        "MO-1",
					Status:         "COMPLETE",
					FilledQuantity: 100,
					TradingSymbol:  "INFY",
				},
			},
		},
	}
	followerReader := &fakeFollowerReader{}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, masterReader, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(consumer.handled) != 1 {
		t.Fatalf("expected 1 fill backfilled, got %d", len(consumer.handled))
	}
	if consumer.handled[0].BrokerOrderID != "MO-1" {
		t.Errorf("BrokerOrderID = %q, want MO-1", consumer.handled[0].BrokerOrderID)
	}
}

func TestPoller_ResolvesStuckFollowerOrder(t *testing.T) {
	followerID := uuid.New()
	brokerOrderID := "FO-101"

	store := &fakeStore{
		pending: []domain.FollowerOrder{
			{
				ID:            1,
				FollowerID:    followerID,
				BrokerOrderID: brokerOrderID,
				IntendedQty:   50,
				CreatedAt:     time.Now().Add(-5 * time.Minute),
			},
		},
	}
	masterReader := &fakeMasterReader{}
	followerReader := &fakeFollowerReader{
		history: map[string][]kiteconnect.Order{
			brokerOrderID: {
				{OrderID: brokerOrderID, Status: "OPEN"},
				{OrderID: brokerOrderID, Status: "COMPLETE", FilledQuantity: 50, AveragePrice: 120.5},
			},
		},
	}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, masterReader, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.updatedOrder[brokerOrderID] != "COMPLETE" {
		t.Errorf("updated status = %q, want COMPLETE", store.updatedOrder[brokerOrderID])
	}
	if len(store.events) != 1 || store.events[0].EventType != "reconciled" {
		t.Errorf("expected 1 reconciled event, got %v", store.events)
	}
	if alerter.count() != 0 {
		t.Errorf("expected 0 alerts, got %d", alerter.count())
	}
}

func TestPoller_AlertsOnDriftAndUnresolved(t *testing.T) {
	followerID := uuid.New()
	store := &fakeStore{
		pending: []domain.FollowerOrder{
			{
				ID:            2,
				FollowerID:    followerID,
				BrokerOrderID: "", // stuck without broker order ID
				CreatedAt:     time.Now().Add(-5 * time.Minute),
			},
			{
				ID:            3,
				FollowerID:    followerID,
				BrokerOrderID: "FO-REJECTED",
				IntendedQty:   100,
				CreatedAt:     time.Now().Add(-5 * time.Minute),
			},
		},
	}
	masterReader := &fakeMasterReader{}
	followerReader := &fakeFollowerReader{
		history: map[string][]kiteconnect.Order{
			"FO-REJECTED": {
				{OrderID: "FO-REJECTED", Status: "REJECTED", FilledQuantity: 0},
			},
		},
	}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, masterReader, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// Should have alert for missing broker_order_id and alert for drift (REJECTED status)
	if alerter.count() != 2 {
		t.Fatalf("expected 2 alerts, got %d: %v", alerter.count(), alerter.alerts)
	}
}

func TestPoller_HandlesReaderErrorsGracefully(t *testing.T) {
	masterID := uuid.New()
	store := &fakeStore{
		accounts: []domain.Account{
			{ID: masterID, Role: "master", Active: true},
		},
	}
	masterReader := &fakeMasterReader{err: errors.New("network down")}
	followerReader := &fakeFollowerReader{}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, masterReader, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	if alerter.count() != 1 {
		t.Fatalf("expected 1 alert for master reader failure, got %d", alerter.count())
	}
}

func TestPoller_SweepsStalePendingOrderUpdatesAndAlerts(t *testing.T) {
	store := &fakeStore{
		sweptIDs: []string{"STALE-ORDER-1", "STALE-ORDER-2"},
	}
	masterReader := &fakeMasterReader{}
	followerReader := &fakeFollowerReader{}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, masterReader, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if alerter.count() != 1 {
		t.Fatalf("expected 1 alert for swept stale orders, got %d", alerter.count())
	}
	if alerter.alerts[0] != "stale unmatched pending order updates swept" {
		t.Errorf("alert msg = %q, want 'stale unmatched pending order updates swept'", alerter.alerts[0])
	}
}

func TestDefaultConfig_HasTightenedThresholds(t *testing.T) {
	cfg := recon.DefaultConfig()
	if cfg.PendingThreshold > 15*time.Second {
		t.Errorf("PendingThreshold = %v, want <= 15s", cfg.PendingThreshold)
	}
	if cfg.PollInterval > 10*time.Second {
		t.Errorf("PollInterval = %v, want <= 10s", cfg.PollInterval)
	}
}

func TestPoller_ReconcileMaster_OnlyReconcilesSpecifiedMaster(t *testing.T) {
	master1 := uuid.New()
	master2 := uuid.New()

	masterReader := &fakeMasterReader{
		orders: map[uuid.UUID][]kiteconnect.Order{
			master1: {
				{OrderID: "M1-ORDER", Status: domain.TerminalComplete, Exchange: "NSE", TradingSymbol: "RELIANCE", FilledQuantity: 100},
			},
			master2: {
				{OrderID: "M2-ORDER", Status: domain.TerminalComplete, Exchange: "NSE", TradingSymbol: "TCS", FilledQuantity: 50},
			},
		},
	}
	followerReader := &fakeFollowerReader{}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}
	store := &fakeStore{}

	poller := recon.NewPoller(store, masterReader, followerReader, consumer, alerter, recon.DefaultConfig(), nil)

	// Reconcile ONLY master1
	if err := poller.ReconcileMaster(context.Background(), master1); err != nil {
		t.Fatalf("ReconcileMaster: %v", err)
	}

	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	if len(consumer.handled) != 1 {
		t.Fatalf("handled fills count = %d, want 1", len(consumer.handled))
	}
	if consumer.handled[0].BrokerOrderID != "M1-ORDER" {
		t.Errorf("handled order ID = %q, want 'M1-ORDER'", consumer.handled[0].BrokerOrderID)
	}
	if consumer.handled[0].MasterID != master1 {
		t.Errorf("handled master ID = %s, want %s", consumer.handled[0].MasterID, master1)
	}
}

func TestPoller_SkipsExistingMasterFill(t *testing.T) {
	masterID := uuid.New()
	store := &fakeStore{
		accounts: []domain.Account{
			{ID: masterID, Role: "master", Active: true},
		},
		masterFillsExist: map[string]bool{
			masterID.String() + ":MO-EXISTS": true,
		},
	}
	masterReader := &fakeMasterReader{
		orders: map[uuid.UUID][]kiteconnect.Order{
			masterID: {
				{
					OrderID:        "MO-EXISTS",
					Status:         "COMPLETE",
					FilledQuantity: 100,
					TradingSymbol:  "INFY",
				},
			},
		},
	}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, masterReader, &fakeFollowerReader{}, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(consumer.handled) != 0 {
		t.Fatalf("expected 0 fills handled since fill already exists, got %d", len(consumer.handled))
	}
}

func TestPoller_BackfillsFollowerOrders(t *testing.T) {
	followerID := uuid.New()
	store := &fakeStore{
		accounts: []domain.Account{
			{ID: followerID, Role: "follower", Active: true},
		},
	}
	followerReader := &fakeFollowerReader{
		orders: map[uuid.UUID][]kiteconnect.Order{
			followerID: {
				{
					OrderID:        "FO-MISSED",
					Status:         "COMPLETE",
					FilledQuantity: 50,
					TradingSymbol:  "TCS",
					Exchange:       "NSE",
					Product:        "CNC",
					TransactionType: "BUY",
				},
			},
		},
	}
	consumer := &fakeConsumer{}
	followerConsumer := &fakeFollowerConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, &fakeMasterReader{}, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	poller.FollowerConsumer = followerConsumer

	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(followerConsumer.handled) != 1 {
		t.Fatalf("expected 1 follower order backfilled, got %d", len(followerConsumer.handled))
	}
	if followerConsumer.handled[0].BrokerOrderID != "FO-MISSED" {
		t.Errorf("BrokerOrderID = %s, want FO-MISSED", followerConsumer.handled[0].BrokerOrderID)
	}
	if followerConsumer.handled[0].FollowerID != followerID {
		t.Errorf("FollowerID = %s, want %s", followerConsumer.handled[0].FollowerID, followerID)
	}
}

func TestPoller_FailsUnresolvableFollowerOrderPast5Minutes(t *testing.T) {
	followerID := uuid.New()
	store := &fakeStore{
		accounts: []domain.Account{
			{ID: followerID, Role: "follower", Active: true},
		},
		pending: []domain.FollowerOrder{
			{
				ID:            101,
				FollowerID:    followerID,
				BrokerOrderID: "FO-STUCK",
				CreatedAt:     time.Now().Add(-6 * time.Minute),
			},
		},
	}
	followerReader := &fakeFollowerReader{
		history: map[string][]kiteconnect.Order{
			"FO-STUCK": {}, // Not found at broker
		},
	}
	consumer := &fakeConsumer{}
	alerter := &fakeAlerter{}

	poller := recon.NewPoller(store, &fakeMasterReader{}, followerReader, consumer, alerter, recon.DefaultConfig(), nil)
	if err := poller.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if store.failedOrders[101] != "order not found at broker past 5m" {
		t.Errorf("failedOrders[101] = %q, want 'order not found at broker past 5m'", store.failedOrders[101])
	}
}


