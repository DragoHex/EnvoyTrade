package listener_test

import (
	"context"
	"errors"
	"testing"

	"envoytrade/internal/domain"
	"envoytrade/internal/listener"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeMasterFillStore struct {
	nextID  int64
	err     error
	inserts []domain.MasterFill
}

func (f *fakeMasterFillStore) InsertMasterFill(ctx context.Context, fill domain.MasterFill) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.inserts = append(f.inserts, fill)
	f.nextID++
	return f.nextID, nil
}

type fakeEngine struct {
	handled []domain.MasterFill
	err     error
}

func (f *fakeEngine) HandleMasterFill(ctx context.Context, fill domain.MasterFill) error {
	f.handled = append(f.handled, fill)
	return f.err
}

func TestMasterFillConsumer_Handle_InsertsAndDispatches(t *testing.T) {
	store := &fakeMasterFillStore{}
	engine := &fakeEngine{}
	c := &listener.MasterFillConsumer{Store: store, Engine: engine}

	fill := domain.MasterFill{BrokerOrderID: "1"}
	if err := c.Handle(context.Background(), fill); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(engine.handled) != 1 {
		t.Fatalf("engine handled %d fills, want 1", len(engine.handled))
	}
	if engine.handled[0].ID != 1 {
		t.Errorf("handled fill ID = %d, want 1 (from InsertMasterFill)", engine.handled[0].ID)
	}
}

func TestMasterFillConsumer_Handle_DuplicateIsNotDispatchedAgain(t *testing.T) {
	store := &fakeMasterFillStore{err: domain.ErrDuplicate}
	engine := &fakeEngine{}
	c := &listener.MasterFillConsumer{Store: store, Engine: engine}

	if err := c.Handle(context.Background(), domain.MasterFill{BrokerOrderID: "1"}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(engine.handled) != 0 {
		t.Fatalf("engine handled %d fills, want 0 (duplicate must not dispatch)", len(engine.handled))
	}
}

func TestMasterFillConsumer_Handle_StoreErrorPropagatesAndSkipsEngine(t *testing.T) {
	store := &fakeMasterFillStore{err: errors.New("boom")}
	engine := &fakeEngine{}
	c := &listener.MasterFillConsumer{Store: store, Engine: engine}

	if err := c.Handle(context.Background(), domain.MasterFill{BrokerOrderID: "1"}); err == nil {
		t.Fatal("Handle: want error, got nil")
	}
	if len(engine.handled) != 0 {
		t.Fatalf("engine handled %d fills, want 0", len(engine.handled))
	}
}

type fakeFollowerStatusStore struct {
	followerID    uuid.UUID
	updateErr     error
	getErr        error
	appendErr     error
	updatedID     int64
	appendedEvent *domain.OrderEvent
}

func (f *fakeFollowerStatusStore) UpdateFollowerOrderStatus(ctx context.Context, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal) (int64, error) {
	if f.updateErr != nil {
		return 0, f.updateErr
	}
	return f.updatedID, nil
}

func (f *fakeFollowerStatusStore) GetFollowerOrder(ctx context.Context, id int64) (domain.FollowerOrder, error) {
	if f.getErr != nil {
		return domain.FollowerOrder{}, f.getErr
	}
	return domain.FollowerOrder{ID: id, FollowerID: f.followerID}, nil
}

func (f *fakeFollowerStatusStore) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.appendedEvent = &ev
	return nil
}

func TestFollowerStatusConsumer_Handle_UpdatesAndAppendsEvent(t *testing.T) {
	followerID := uuid.New()
	store := &fakeFollowerStatusStore{followerID: followerID, updatedID: 42}
	c := &listener.FollowerStatusConsumer{Store: store}

	upd := domain.OrderUpdate{BrokerOrderID: "F-1", Status: domain.TerminalComplete, FilledQuantity: 50, RawPayload: []byte(`{}`)}
	if err := c.Handle(context.Background(), upd); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if store.appendedEvent == nil {
		t.Fatal("no order event appended")
	}
	if *store.appendedEvent.FollowerOrderID != 42 {
		t.Errorf("FollowerOrderID = %d, want 42", *store.appendedEvent.FollowerOrderID)
	}
	if store.appendedEvent.AccountID != followerID {
		t.Errorf("AccountID = %v, want %v", store.appendedEvent.AccountID, followerID)
	}
}

func TestFollowerStatusConsumer_Handle_UnknownBrokerOrderIDIsDroppedNotError(t *testing.T) {
	store := &fakeFollowerStatusStore{updateErr: domain.ErrNotFound}
	c := &listener.FollowerStatusConsumer{Store: store}

	err := c.Handle(context.Background(), domain.OrderUpdate{BrokerOrderID: "GHOST"})
	if err != nil {
		t.Fatalf("Handle: %v, want nil (dropped, not an error)", err)
	}
	if store.appendedEvent != nil {
		t.Fatal("appended an event for an unknown broker_order_id")
	}
}

func TestFollowerStatusConsumer_Handle_UpdateErrorPropagates(t *testing.T) {
	store := &fakeFollowerStatusStore{updateErr: errors.New("boom")}
	c := &listener.FollowerStatusConsumer{Store: store}

	if err := c.Handle(context.Background(), domain.OrderUpdate{BrokerOrderID: "F-1"}); err == nil {
		t.Fatal("Handle: want error, got nil")
	}
}
