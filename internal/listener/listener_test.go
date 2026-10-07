package listener_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestMasterFillConsumer_Handle_TriggersPortfolioSync(t *testing.T) {
	masterID := uuid.New()
	store := &fakeMasterFillStore{}
	engine := &fakeEngine{}
	syncer := &fakePortfolioSyncer{syncedID: make(chan uuid.UUID, 1)}
	c := &listener.MasterFillConsumer{Store: store, Engine: engine, Syncer: syncer}

	fill := domain.MasterFill{MasterID: masterID, BrokerOrderID: "101"}
	if err := c.Handle(context.Background(), fill); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	select {
	case id := <-syncer.syncedID:
		if id != masterID {
			t.Errorf("syncedID = %s, want %s", id, masterID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("portfolio sync was not triggered within 1s")
	}
}

type fakeFollowerStatusStore struct {
	followerID      uuid.UUID
	updateErr       error
	updateByTagErr  error
	insertDirectErr error
	stashErr        error
	getErr          error
	appendErr       error
	updatedID       int64
	appendedEvent   *domain.OrderEvent
	stashedBrokerID string
	stashedStatus   string
	stashedQty      int
	stashedPrice    decimal.Decimal
	stashedPayload  []byte
	directInserted  *domain.OrderUpdate
	tagUpdatedTag   string
}

func (f *fakeFollowerStatusStore) UpdateFollowerOrderStatus(ctx context.Context, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal) (int64, error) {
	if f.updateErr != nil {
		return 0, f.updateErr
	}
	return f.updatedID, nil
}

func (f *fakeFollowerStatusStore) UpdateFollowerOrderByTag(ctx context.Context, followerID uuid.UUID, tag, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal) (int64, error) {
	if f.updateByTagErr != nil {
		return 0, f.updateByTagErr
	}
	f.tagUpdatedTag = tag
	return f.updatedID, nil
}

func (f *fakeFollowerStatusStore) InsertDirectFollowerOrder(ctx context.Context, upd domain.OrderUpdate) (int64, error) {
	if f.insertDirectErr != nil {
		return 0, f.insertDirectErr
	}
	f.directInserted = &upd
	return f.updatedID, nil
}

func (f *fakeFollowerStatusStore) StashPendingOrderUpdate(ctx context.Context, brokerOrderID, status string, filledQty int, averagePrice decimal.Decimal, rawPayload []byte) error {
	if f.stashErr != nil {
		return f.stashErr
	}
	f.stashedBrokerID = brokerOrderID
	f.stashedStatus = status
	f.stashedQty = filledQty
	f.stashedPrice = averagePrice
	f.stashedPayload = rawPayload
	return nil
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

type fakePortfolioSyncer struct {
	syncedID chan uuid.UUID
}

func (s *fakePortfolioSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	s.syncedID <- accountID
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

func TestFollowerStatusConsumer_Handle_DirectFollowerOrder_InsertsAndSyncs(t *testing.T) {
	followerID := uuid.New()
	store := &fakeFollowerStatusStore{
		followerID: followerID,
		updateErr:  domain.ErrNotFound,
		updatedID:  88,
	}
	syncer := &fakePortfolioSyncer{syncedID: make(chan uuid.UUID, 1)}
	c := &listener.FollowerStatusConsumer{Store: store, Syncer: syncer}

	upd := domain.OrderUpdate{
		FollowerID:      followerID,
		BrokerOrderID:   "DIRECT-001",
		Tradingsymbol:   "RELIANCE",
		Exchange:        "NSE",
		Product:         "CNC",
		TransactionType: "BUY",
		Status:          domain.TerminalComplete,
		FilledQuantity:  5,
		AveragePrice:    decimal.NewFromFloat(2450.0),
		RawPayload:      []byte(`{}`),
	}

	if err := c.Handle(context.Background(), upd); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if store.directInserted == nil {
		t.Fatal("expected InsertDirectFollowerOrder to be called")
	}
	if store.directInserted.BrokerOrderID != "DIRECT-001" {
		t.Errorf("BrokerOrderID = %q, want DIRECT-001", store.directInserted.BrokerOrderID)
	}
	if store.appendedEvent == nil {
		t.Fatal("expected order event to be appended")
	}

	select {
	case gotID := <-syncer.syncedID:
		if gotID != followerID {
			t.Errorf("syncedID = %v, want %v", gotID, followerID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for portfolio sync on direct follower order complete")
	}
}

func TestFollowerStatusConsumer_Handle_InFlightTagMatching_ResolvesOrder(t *testing.T) {
	followerID := uuid.New()
	store := &fakeFollowerStatusStore{
		followerID: followerID,
		updateErr:  domain.ErrNotFound,
		updatedID:  99,
	}
	c := &listener.FollowerStatusConsumer{Store: store}

	upd := domain.OrderUpdate{
		FollowerID:     followerID,
		BrokerOrderID:  "BROKER-123",
		Tag:            "race-tag-123",
		Status:         domain.TerminalComplete,
		FilledQuantity: 10,
	}

	if err := c.Handle(context.Background(), upd); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if store.tagUpdatedTag != "race-tag-123" {
		t.Errorf("tagUpdatedTag = %q, want race-tag-123", store.tagUpdatedTag)
	}
	if store.directInserted != nil {
		t.Fatal("did not expect direct insert when tag matches in-flight placement")
	}
}

func TestFollowerStatusConsumer_Handle_DuplicateDirectOrder_Ignored(t *testing.T) {
	followerID := uuid.New()
	store := &fakeFollowerStatusStore{
		followerID:      followerID,
		updateErr:       domain.ErrNotFound,
		insertDirectErr: domain.ErrDuplicate,
	}
	c := &listener.FollowerStatusConsumer{Store: store}

	upd := domain.OrderUpdate{
		FollowerID:    followerID,
		BrokerOrderID: "DUP-001",
		Status:        domain.TerminalComplete,
	}

	if err := c.Handle(context.Background(), upd); err != nil {
		t.Fatalf("Handle duplicate should return nil (no-op), got: %v", err)
	}
}

func TestFollowerStatusConsumer_Handle_UnknownBrokerOrderIDFallbackStash(t *testing.T) {
	store := &fakeFollowerStatusStore{
		updateErr:       domain.ErrNotFound,
		insertDirectErr: errors.New("temporary db failure"),
	}
	c := &listener.FollowerStatusConsumer{Store: store}

	upd := domain.OrderUpdate{
		BrokerOrderID:  "EARLY-ORDER",
		Status:         domain.TerminalComplete,
		FilledQuantity: 25,
		AveragePrice:   decimal.NewFromFloat(150.25),
		RawPayload:     []byte(`{"order_id":"EARLY-ORDER"}`),
	}
	err := c.Handle(context.Background(), upd)
	if err == nil {
		t.Fatal("Handle: want error from insert, got nil")
	}
	if store.stashedBrokerID != "EARLY-ORDER" {
		t.Errorf("stashedBrokerID = %q, want EARLY-ORDER", store.stashedBrokerID)
	}
}

func TestFollowerStatusConsumer_Handle_TriggersPortfolioSyncOnComplete(t *testing.T) {
	followerID := uuid.New()
	store := &fakeFollowerStatusStore{followerID: followerID, updatedID: 42}
	syncer := &fakePortfolioSyncer{syncedID: make(chan uuid.UUID, 1)}
	c := &listener.FollowerStatusConsumer{Store: store, Syncer: syncer}

	upd := domain.OrderUpdate{BrokerOrderID: "F-1", Status: domain.TerminalComplete, FilledQuantity: 50, RawPayload: []byte(`{}`)}
	if err := c.Handle(context.Background(), upd); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	select {
	case gotID := <-syncer.syncedID:
		if gotID != followerID {
			t.Errorf("syncedID = %v, want %v", gotID, followerID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for portfolio sync to be called for follower on COMPLETE")
	}
}

func TestFollowerStatusConsumer_Handle_UpdateErrorPropagates(t *testing.T) {
	store := &fakeFollowerStatusStore{updateErr: errors.New("boom")}
	c := &listener.FollowerStatusConsumer{Store: store}

	if err := c.Handle(context.Background(), domain.OrderUpdate{BrokerOrderID: "F-1"}); err == nil {
		t.Fatal("Handle: want error, got nil")
	}
}

