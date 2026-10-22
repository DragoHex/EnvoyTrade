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

func (m *mockEngineStore) InsertFollowerOrdersBatch(ctx context.Context, orders []domain.FollowerOrder) (map[string]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make(map[string]int64, len(orders))
	for _, o := range orders {
		isDup := false
		for _, prev := range m.insertedOrders {
			if prev.IdempotencyTag == o.IdempotencyTag {
				isDup = true
				break
			}
		}
		if isDup {
			continue
		}
		m.nextOrderID++
		o.ID = m.nextOrderID
		m.insertedOrders = append(m.insertedOrders, o)
		res[o.IdempotencyTag] = o.ID
	}
	return res, nil
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
			CloneFactor:    decimal.NewFromInt(1),
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
			CloneFactor:   decimal.NewFromInt(1),
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
			CloneFactor:   decimal.NewFromFloat(0.5),
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
			CloneFactor: decimal.NewFromInt(1),
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
			CloneFactor: decimal.NewFromInt(1),
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
			CloneFactor: decimal.NewFromInt(1),
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

func TestHandleMasterFill_BatchDispatchAllFollowers(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	f1 := uuid.New()
	f2 := uuid.New()
	f3 := uuid.New()

	store.lotSizes["NSE:RELIANCE"] = 1
	store.followLinks = []domain.FollowLink{
		{
			FollowerID:     f1,
			MasterID:       masterID,
			CloneFactor:    decimal.NewFromInt(1),
			MaxQtyPerOrder: 0,
			Enabled:        true,
		},
		{
			FollowerID:     f2,
			MasterID:       masterID,
			CloneFactor:    decimal.NewFromFloat(2.0),
			MaxQtyPerOrder: 0,
			Enabled:        true,
		},
		{
			FollowerID:     f3,
			MasterID:       masterID,
			CloneFactor:    decimal.NewFromFloat(0.5),
			MaxQtyPerOrder: 40,
			Enabled:        true,
		},
	}

	fill := domain.MasterFill{
		ID:              501,
		MasterID:        masterID,
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		FilledQuantity:  100,
		TransactionType: "BUY",
		Product:         "CNC",
		OrderType:       "LIMIT",
		Price:           decimal.NewFromInt(2500),
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	if err := eng.HandleMasterFill(context.Background(), fill); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	if len(disp.dispatched) != 3 {
		t.Fatalf("expected 3 jobs dispatched, got %d", len(disp.dispatched))
	}

	jobMap := make(map[uuid.UUID]domain.Job)
	for _, j := range disp.dispatched {
		jobMap[j.FollowerID] = j
	}

	if jobMap[f1].Quantity != 100 {
		t.Errorf("f1 qty = %d, want 100", jobMap[f1].Quantity)
	}
	if jobMap[f2].Quantity != 200 {
		t.Errorf("f2 qty = %d, want 200", jobMap[f2].Quantity)
	}
	if jobMap[f3].Quantity != 40 {
		t.Errorf("f3 qty = %d, want 40 (capped from 50)", jobMap[f3].Quantity)
	}
}

func TestEngine_LotSizeCacheHitAndClear(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	masterID := uuid.New()
	f1 := uuid.New()
	store.lotSizes["NFO:NIFTY26OCTFUT"] = 75
	store.followLinks = []domain.FollowLink{
		{
			FollowerID:     f1,
			MasterID:       masterID,
			CloneFactor:    decimal.NewFromFloat(2.0),
			MaxQtyPerOrder: 0,
			Enabled:        true,
		},
	}

	fill1 := domain.MasterFill{
		ID:              601,
		MasterID:        masterID,
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		FilledQuantity:  75,
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	// First fill: should query store.InstrumentLotSize once
	if err := eng.HandleMasterFill(context.Background(), fill1); err != nil {
		t.Fatalf("fill1 failed: %v", err)
	}
	if store.instrumentLookupCalls != 1 {
		t.Fatalf("expected 1 instrument lookup call, got %d", store.instrumentLookupCalls)
	}

	// Second fill with same symbol: should hit cache, instrumentLookupCalls remains 1!
	fill2 := fill1
	fill2.ID = 602
	if err := eng.HandleMasterFill(context.Background(), fill2); err != nil {
		t.Fatalf("fill2 failed: %v", err)
	}
	if store.instrumentLookupCalls != 1 {
		t.Errorf("expected instrumentLookupCalls to remain 1 (cache hit), got %d", store.instrumentLookupCalls)
	}

	// Clear cache: next fill should query store again (instrumentLookupCalls becomes 2)
	eng.ClearLotSizeCache()
	fill3 := fill1
	fill3.ID = 603
	if err := eng.HandleMasterFill(context.Background(), fill3); err != nil {
		t.Fatalf("fill3 failed: %v", err)
	}
	if store.instrumentLookupCalls != 2 {
		t.Errorf("expected instrumentLookupCalls to be 2 after cache clear, got %d", store.instrumentLookupCalls)
	}
}

func TestEngine_KillSwitch(t *testing.T) {
	store := newMockEngineStore()
	masterID := uuid.New()
	followerID := uuid.New()
	store.followLinks = []domain.FollowLink{
		{
			FollowerID:     followerID,
			MasterID:       masterID,
			CloneFactor:    decimal.NewFromInt(1),
			MaxQtyPerOrder: 0,
			Enabled:        true,
		},
	}
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	// Initially not engaged
	if eng.IsKillSwitchEngaged() {
		t.Fatal("expected kill switch to be initially disengaged")
	}
	st := eng.KillSwitchStatus()
	if st.Engaged {
		t.Fatal("expected kill switch status to be false")
	}

	// Engage kill switch
	eng.SetKillSwitch(true, "Emergency circuit breaker triggered", "admin@envoytrade.com")
	if !eng.IsKillSwitchEngaged() {
		t.Fatal("expected kill switch to be engaged")
	}
	st = eng.KillSwitchStatus()
	if !st.Engaged || st.Reason != "Emergency circuit breaker triggered" || st.EngagedBy != "admin@envoytrade.com" {
		t.Fatalf("unexpected kill switch status: %+v", st)
	}

	fill1 := domain.MasterFill{
		ID:              101,
		MasterID:        masterID,
		BrokerOrderID:   "BO-KILL-1",
		Tradingsymbol:   "RELIANCE",
		Exchange:        "NSE",
		TransactionType: "BUY",
		FilledQuantity:  10,
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	// Should drop fan-out without dispatching to followers
	if err := eng.HandleMasterFill(context.Background(), fill1); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}
	if len(disp.dispatched) != 0 {
		t.Fatalf("expected 0 dispatched jobs when kill switch engaged, got %d", len(disp.dispatched))
	}
	if len(store.setMasterFillDispatchState) != 1 || store.setMasterFillDispatchState[0] != domain.DispatchDispatched {
		t.Fatalf("expected master fill 101 to be marked dispatched, got %+v", store.setMasterFillDispatchState)
	}

	// Disengage kill switch
	eng.SetKillSwitch(false, "", "admin@envoytrade.com")
	if eng.IsKillSwitchEngaged() {
		t.Fatal("expected kill switch to be disengaged")
	}

	fill2 := domain.MasterFill{
		ID:              102,
		MasterID:        masterID,
		BrokerOrderID:   "BO-KILL-2",
		Tradingsymbol:   "RELIANCE",
		Exchange:        "NSE",
		TransactionType: "BUY",
		FilledQuantity:  10,
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}
	if err := eng.HandleMasterFill(context.Background(), fill2); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}
	if len(disp.dispatched) != 1 {
		t.Fatalf("expected 1 dispatched job after kill switch disengaged, got %d", len(disp.dispatched))
	}
}

type fakeExecutionRecorder struct {
	mu     sync.Mutex
	events []domain.ExecutionEvent
}

func (f *fakeExecutionRecorder) Record(event domain.ExecutionEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func TestHandleMasterFill_RecordsExecutionEvent(t *testing.T) {
	store := newMockEngineStore()
	disp := newMockDispatcher()
	eng := engine.New(store, disp)

	rec := &fakeExecutionRecorder{}
	eng.ExecutionRecorder = rec

	masterID := uuid.New()
	follower1 := uuid.New()
	follower2 := uuid.New()

	store.followLinks = []domain.FollowLink{
		{
			MasterID:    masterID,
			FollowerID:  follower1,
			CloneFactor: decimal.NewFromInt(1),
			Enabled:     true,
		},
		{
			MasterID:    masterID,
			FollowerID:  follower2,
			CloneFactor: decimal.NewFromFloat(0.1), // 10 qty * 0.1 = 1, but lot size is 50 -> 0 qty (skipped)
			Enabled:     true,
		},
	}
	store.lotSizes["NSE:NIFTY"] = 50

	fill := domain.MasterFill{
		ID:              999,
		MasterID:        masterID,
		BrokerOrderID:   "BO-999",
		Tradingsymbol:   "NIFTY",
		Exchange:        "NSE",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Price:           decimal.NewFromInt(0),
		AveragePrice:    decimal.NewFromFloat(22500.50),
		FilledQuantity:  10,
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
	}

	if err := eng.HandleMasterFill(context.Background(), fill); err != nil {
		t.Fatalf("HandleMasterFill failed: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 recorded event, got %d", len(rec.events))
	}

	ev := rec.events[0]
	if ev.Tradingsymbol != "NIFTY" || ev.Exchange != "NSE" || ev.TransactionType != "BUY" {
		t.Errorf("unexpected event symbol/exchange: %+v", ev)
	}
	if ev.MasterQty != 10 {
		t.Errorf("expected masterQty 10, got %d", ev.MasterQty)
	}
	if ev.Price != 22500.50 {
		t.Errorf("expected price 22500.50, got %f", ev.Price)
	}
	if ev.FollowersCount != 1 {
		t.Errorf("expected followersCount 1, got %d", ev.FollowersCount)
	}
	if ev.SkippedCount != 1 {
		t.Errorf("expected skippedCount 1, got %d", ev.SkippedCount)
	}
	if ev.LatencyMs < 0 {
		t.Errorf("expected latencyMs >= 0, got %d", ev.LatencyMs)
	}
}



