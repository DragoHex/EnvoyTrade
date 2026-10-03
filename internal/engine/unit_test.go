package engine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/engine"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type mockEngineStore struct {
	mu                         sync.Mutex
	masterActive               bool
	masterActiveErr            error
	followLinks                []domain.FollowLink
	followLinksErr             error
	lotSizes                   map[string]int
	instrumentLookupCalls      int
	lastLookupExchange         string
	lastLookupSymbol           string
	insertedOrders             []domain.FollowerOrder
	failedOrders               map[int64]string
	nextOrderID                int64
	setMasterFillDispatchState []domain.DispatchState
}

func newMockEngineStore() *mockEngineStore {
	return &mockEngineStore{
		masterActive: true,
		lotSizes:     make(map[string]int),
		failedOrders: make(map[int64]string),
	}
}

func (m *mockEngineStore) MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.masterActive, m.masterActiveErr
}

func (m *mockEngineStore) EnabledFollowLinks(ctx context.Context, masterID uuid.UUID) ([]domain.FollowLink, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.followLinks, m.followLinksErr
}

func (m *mockEngineStore) InstrumentLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instrumentLookupCalls++
	m.lastLookupExchange = exchange
	m.lastLookupSymbol = tradingsymbol

	key := exchange + ":" + tradingsymbol
	if size, ok := m.lotSizes[key]; ok {
		return size, nil
	}
	return 0, domain.ErrNotFound
}

func (m *mockEngineStore) InsertFollowerOrder(ctx context.Context, o domain.FollowerOrder) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextOrderID++
	o.ID = m.nextOrderID
	m.insertedOrders = append(m.insertedOrders, o)
	return o.ID, nil
}

func (m *mockEngineStore) SetMasterFillDispatchState(ctx context.Context, id int64, state domain.DispatchState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setMasterFillDispatchState = append(m.setMasterFillDispatchState, state)
	return nil
}

func (m *mockEngineStore) UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedOrders[id] = terminalStatus + ": " + errMsg
	return nil
}

type mockDispatcher struct {
	mu           sync.Mutex
	dispatched   []domain.Job
	refusedUsers map[uuid.UUID]bool
}

func newMockDispatcher() *mockDispatcher {
	return &mockDispatcher{
		refusedUsers: make(map[uuid.UUID]bool),
	}
}

func (d *mockDispatcher) Dispatch(followerID uuid.UUID, job domain.Job) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.refusedUsers[followerID] {
		return false
	}
	d.dispatched = append(d.dispatched, job)
	return true
}

func TestEngine_OneToOneFastPath_DoesNotCallInstrumentLookup(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	followerID := uuid.New()

	// 1:1 follower link
	store.followLinks = []domain.FollowLink{
		{
			FollowerID:      followerID,
			MasterID:        masterID,
			CapitalRatio:    decimal.NewFromInt(1),
			MaxQtyPerOrder:  0,
			Enabled:         true,
		},
	}

	fill := domain.MasterFill{
		ID:              100,
		MasterID:        masterID,
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26OCT11000CE",
		FilledQuantity:  2,
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	err := eng.HandleMasterFill(context.Background(), fill)
	if err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	// 1. Assert InstrumentLotSize was NEVER called (fast-path)
	if store.instrumentLookupCalls != 0 {
		t.Errorf("expected 0 instrument lookup calls for 1:1 follower, got %d", store.instrumentLookupCalls)
	}

	// 2. Assert follower order was inserted with exact master quantity and ReasonOK
	if len(store.insertedOrders) != 1 {
		t.Fatalf("expected 1 follower order inserted, got %d", len(store.insertedOrders))
	}
	fo := store.insertedOrders[0]
	if fo.IntendedQty != 2 {
		t.Errorf("expected IntendedQty = 2, got %d", fo.IntendedQty)
	}
	if fo.SizingReason != domain.ReasonOK {
		t.Errorf("expected SizingReason = ReasonOK, got %v", fo.SizingReason)
	}

	// 3. Assert job was dispatched to worker
	if len(disp.dispatched) != 1 {
		t.Fatalf("expected 1 job dispatched, got %d", len(disp.dispatched))
	}
	job := disp.dispatched[0]
	if job.Quantity != 2 {
		t.Errorf("expected job.Quantity = 2, got %d", job.Quantity)
	}
	if job.Tradingsymbol != "CRUDEOIL26OCT11000CE" {
		t.Errorf("expected job symbol = CRUDEOIL26OCT11000CE, got %s", job.Tradingsymbol)
	}
}

func TestEngine_OneToOneFastPath_WithMaxQtyCap(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	followerID := uuid.New()

	// 1:1 follower with cap of 50
	store.followLinks = []domain.FollowLink{
		{
			FollowerID:     followerID,
			MasterID:       masterID,
			CapitalRatio:   decimal.NewFromInt(1),
			MaxQtyPerOrder: 50,
			Enabled:        true,
		},
	}

	fill := domain.MasterFill{
		ID:              101,
		MasterID:        masterID,
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		FilledQuantity:  100, // exceeds cap of 50
		TransactionType: "BUY",
		Product:         "CNC",
		OrderType:       "MARKET",
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	err := eng.HandleMasterFill(context.Background(), fill)
	if err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	// Should not query instrument master
	if store.instrumentLookupCalls != 0 {
		t.Errorf("expected 0 instrument lookup calls, got %d", store.instrumentLookupCalls)
	}

	if len(store.insertedOrders) != 1 {
		t.Fatalf("expected 1 follower order inserted, got %d", len(store.insertedOrders))
	}
	fo := store.insertedOrders[0]
	if fo.IntendedQty != 50 {
		t.Errorf("expected capped IntendedQty = 50, got %d", fo.IntendedQty)
	}
	if fo.SizingReason != domain.ReasonCapped {
		t.Errorf("expected SizingReason = ReasonCapped, got %v", fo.SizingReason)
	}
}

func TestEngine_NonOneToOne_CallsInstrumentLookupAndSizes(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	followerID := uuid.New()

	// 0.5 ratio follower link (needs lot sizing)
	store.followLinks = []domain.FollowLink{
		{
			FollowerID:     followerID,
			MasterID:       masterID,
			CapitalRatio:   decimal.NewFromFloat(0.5),
			MaxQtyPerOrder: 0,
			Enabled:        true,
		},
	}
	// Seed instrument in mock store with lot size = 25
	store.lotSizes["NFO:NIFTY26OCT25000CE"] = 25

	fill := domain.MasterFill{
		ID:              102,
		MasterID:        masterID,
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCT25000CE",
		FilledQuantity:  50, // 2 lots
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	err := eng.HandleMasterFill(context.Background(), fill)
	if err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	// Instrument lookup MUST be called for ratio != 1
	if store.instrumentLookupCalls != 1 {
		t.Errorf("expected 1 instrument lookup call for ratio 0.5, got %d", store.instrumentLookupCalls)
	}

	if len(store.insertedOrders) != 1 {
		t.Fatalf("expected 1 follower order, got %d", len(store.insertedOrders))
	}
	fo := store.insertedOrders[0]
	// 50 * 0.5 = 25 (1 lot of 25)
	if fo.IntendedQty != 25 {
		t.Errorf("expected IntendedQty = 25, got %d", fo.IntendedQty)
	}
	if fo.LotSize != 25 {
		t.Errorf("expected LotSize = 25, got %d", fo.LotSize)
	}
	if fo.SizingReason != domain.ReasonOK {
		t.Errorf("expected SizingReason = ReasonOK, got %v", fo.SizingReason)
	}
}

func TestEngine_LimitOrder_CarriesMasterLimitPrice(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	followerID := uuid.New()

	store.followLinks = []domain.FollowLink{
		{
			FollowerID:   followerID,
			MasterID:     masterID,
			CapitalRatio: decimal.NewFromInt(1),
			Enabled:      true,
		},
	}

	fill := domain.MasterFill{
		ID:              201,
		MasterID:        masterID,
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26OCT11000CE",
		FilledQuantity:  1,
		TransactionType: "SELL",
		Product:         "NRML",
		OrderType:       "LIMIT",
		Price:           decimal.NewFromFloat(26.5),
		AveragePrice:    decimal.NewFromFloat(27.5),
		TriggerPrice:    decimal.NewFromFloat(25.0),
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	if err := eng.HandleMasterFill(context.Background(), fill); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	if len(disp.dispatched) != 1 {
		t.Fatalf("expected 1 job dispatched, got %d", len(disp.dispatched))
	}
	job := disp.dispatched[0]
	if job.Price != 26.5 {
		t.Errorf("expected job.Price = 26.5, got %v", job.Price)
	}
	if job.TriggerPrice != 25.0 {
		t.Errorf("expected job.TriggerPrice = 25.0, got %v", job.TriggerPrice)
	}
}

func TestEngine_LimitOrder_ResolvesPriceFromAveragePriceWhenZero(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	followerID := uuid.New()

	store.followLinks = []domain.FollowLink{
		{
			FollowerID:   followerID,
			MasterID:     masterID,
			CapitalRatio: decimal.NewFromInt(1),
			Enabled:      true,
		},
	}

	// Price is 0, but AveragePrice is 27.5
	fill := domain.MasterFill{
		ID:              202,
		MasterID:        masterID,
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26OCT11000CE",
		FilledQuantity:  1,
		TransactionType: "SELL",
		Product:         "NRML",
		OrderType:       "LIMIT",
		Price:           decimal.Zero,
		AveragePrice:    decimal.NewFromFloat(27.5),
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	if err := eng.HandleMasterFill(context.Background(), fill); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	if len(disp.dispatched) != 1 {
		t.Fatalf("expected 1 job dispatched, got %d", len(disp.dispatched))
	}
	job := disp.dispatched[0]
	if job.Price != 27.5 {
		t.Errorf("expected job.Price = 27.5 (fell back to AveragePrice), got %v", job.Price)
	}
}

func TestEngine_LimitOrder_ResolvesPriceFromRawPayloadWhenZero(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	followerID := uuid.New()

	store.followLinks = []domain.FollowLink{
		{
			FollowerID:   followerID,
			MasterID:     masterID,
			CapitalRatio: decimal.NewFromInt(1),
			Enabled:      true,
		},
	}

	// fill.Price is 0, AveragePrice is 0, but RawPayload has price: 26.5
	fill := domain.MasterFill{
		ID:              203,
		MasterID:        masterID,
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26OCT11000CE",
		FilledQuantity:  1,
		TransactionType: "SELL",
		Product:         "NRML",
		OrderType:       "LIMIT",
		Price:           decimal.Zero,
		AveragePrice:    decimal.Zero,
		RawPayload:      []byte(`{"price":26.5,"trigger_price":24.5}`),
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	if err := eng.HandleMasterFill(context.Background(), fill); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	if len(disp.dispatched) != 1 {
		t.Fatalf("expected 1 job dispatched, got %d", len(disp.dispatched))
	}
	job := disp.dispatched[0]
	if job.Price != 26.5 {
		t.Errorf("expected job.Price = 26.5 from RawPayload, got %v", job.Price)
	}
	if job.TriggerPrice != 24.5 {
		t.Errorf("expected job.TriggerPrice = 24.5 from RawPayload, got %v", job.TriggerPrice)
	}
}
