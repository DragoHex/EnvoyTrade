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
