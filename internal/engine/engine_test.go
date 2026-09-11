//go:build integration

// Requires Docker: go test -tags integration ./internal/engine/...
package engine_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/engine"
	"envoytrade/internal/store/postgres"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// fakeDispatcher records every dispatched job and can be told to refuse
// dispatch for specific followers, simulating a full worker channel.
type fakeDispatcher struct {
	mu       sync.Mutex
	refuse   map[uuid.UUID]bool
	jobs     []domain.Job
	dispatch map[uuid.UUID]int
}

func newFakeDispatcher() *fakeDispatcher {
	return &fakeDispatcher{refuse: map[uuid.UUID]bool{}, dispatch: map[uuid.UUID]int{}}
}

func (f *fakeDispatcher) Dispatch(followerID uuid.UUID, job domain.Job) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatch[followerID]++
	if f.refuse[followerID] {
		return false
	}
	f.jobs = append(f.jobs, job)
	return true
}

func (f *fakeDispatcher) jobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

func newTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("envoytrade_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := postgres.NewPool(ctx, connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	store := postgres.New(pool)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

func seedAccount(t *testing.T, s *postgres.Store, role string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := s.CreateAccount(context.Background(), id, role, id.String()); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	return id
}

func seedFollowLink(t *testing.T, s *postgres.Store, link domain.FollowLink) {
	t.Helper()
	if err := s.CreateFollowLink(context.Background(), link); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}
}

// seedInstrument registers a lot size in the instrument master. insertFill
// seeds NSE/INFY at lot size 1 by default; tests exercising other symbols
// or lot sizes (e.g. F&O contracts) call this directly.
func seedInstrument(t *testing.T, s *postgres.Store, ins domain.Instrument) {
	t.Helper()
	if err := s.UpsertInstrument(context.Background(), ins); err != nil {
		t.Fatalf("UpsertInstrument: %v", err)
	}
}

func insertFill(t *testing.T, s *postgres.Store, masterID uuid.UUID, qty int) domain.MasterFill {
	t.Helper()
	seedInstrument(t, s, domain.Instrument{
		InstrumentToken: 1, Exchange: "NSE", Tradingsymbol: "INFY",
		LotSize: 1, TickSize: decimal.NewFromFloat(0.05),
	})
	fill := domain.MasterFill{
		MasterID:        masterID,
		BrokerOrderID:   uuid.NewString(),
		Exchange:        "NSE",
		Tradingsymbol:   "INFY",
		InstrumentToken: 1,
		TransactionType: "BUY",
		Product:         "MIS",
		OrderType:       "MARKET",
		FilledQuantity:  qty,
		AveragePrice:    decimal.NewFromInt(100),
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
		RawPayload:      []byte(`{}`),
	}
	id, err := s.InsertMasterFill(context.Background(), fill)
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	fill.ID = id
	return fill
}

func TestHandleMasterFill_FansOutToAllEnabledFollowers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	followerA := seedAccount(t, s, "follower")
	followerB := seedAccount(t, s, "follower")
	seedFollowLink(t, s, domain.FollowLink{FollowerID: followerA, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true})
	seedFollowLink(t, s, domain.FollowLink{FollowerID: followerB, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.25), Enabled: true})

	fill := insertFill(t, s, master, 100)
	disp := newFakeDispatcher()
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("got %d follower_orders, want 2", len(orders))
	}
	if disp.jobCount() != 2 {
		t.Fatalf("got %d dispatched jobs, want 2", disp.jobCount())
	}

	wantQty := map[uuid.UUID]int{followerA: 50, followerB: 25}
	for _, job := range disp.jobs {
		if job.Quantity != wantQty[job.FollowerID] {
			t.Fatalf("follower %v quantity = %d, want %d", job.FollowerID, job.Quantity, wantQty[job.FollowerID])
		}
	}
}

func TestHandleMasterFill_DisabledLinkGetsNoOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	disabledFollower := seedAccount(t, s, "follower")
	seedFollowLink(t, s, domain.FollowLink{FollowerID: disabledFollower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: false})

	fill := insertFill(t, s, master, 100)
	disp := newFakeDispatcher()
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 0 {
		t.Fatalf("got %d follower_orders for a disabled link, want 0", len(orders))
	}
}

func TestHandleMasterFill_BelowOneLotCreatesRowButNotDispatched(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	// ratio so small that even 100 * ratio floors to 0 given lot size 1... use lot size via fill.
	seedFollowLink(t, s, domain.FollowLink{FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.001), Enabled: true})

	fill := insertFill(t, s, master, 10) // lot size 1, ratio 0.001 -> floor(10*0.001/1) = 0
	disp := newFakeDispatcher()
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("got %d follower_orders, want 1 (recorded even at zero qty)", len(orders))
	}
	if disp.jobCount() != 0 {
		t.Fatalf("got %d dispatched jobs, want 0 for a below-one-lot order", disp.jobCount())
	}

	got, err := s.GetFollowerOrder(ctx, orders[0].ID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if got.IntendedQty != 0 {
		t.Fatalf("IntendedQty = %d, want 0", got.IntendedQty)
	}
	if got.SizingReason != domain.ReasonBelowOneLot {
		t.Fatalf("SizingReason = %v, want ReasonBelowOneLot", got.SizingReason)
	}
}

func TestHandleMasterFill_RedeliveryIsANoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	seedFollowLink(t, s, domain.FollowLink{FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true})

	fill := insertFill(t, s, master, 100)
	disp := newFakeDispatcher()
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("first HandleMasterFill: %v", err)
	}
	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("second HandleMasterFill (redelivery): %v", err)
	}

	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("got %d follower_orders after redelivery, want 1 (no duplicate)", len(orders))
	}
	if disp.jobCount() != 1 {
		t.Fatalf("got %d dispatched jobs after redelivery, want 1 (no duplicate dispatch)", disp.jobCount())
	}
}

func TestHandleMasterFill_ZeroEnabledFollowersStillMarksDispatched(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	fill := insertFill(t, s, master, 100)
	disp := newFakeDispatcher()
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}
	// dispatch_state isn't exposed by a getter yet; absence of error plus
	// zero orders is the observable behavior for this slice.
	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 0 {
		t.Fatalf("got %d follower_orders with zero enabled followers, want 0", len(orders))
	}
}

func TestHandleMasterFill_OneFollowerDeadLetteredOthersUnaffected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	blocked := seedAccount(t, s, "follower")
	healthy := seedAccount(t, s, "follower")
	seedFollowLink(t, s, domain.FollowLink{FollowerID: blocked, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true})
	seedFollowLink(t, s, domain.FollowLink{FollowerID: healthy, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true})

	fill := insertFill(t, s, master, 100)
	disp := newFakeDispatcher()
	disp.refuse[blocked] = true
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("got %d follower_orders, want 2", len(orders))
	}
	if disp.jobCount() != 1 {
		t.Fatalf("got %d successfully dispatched jobs, want 1 (healthy only)", disp.jobCount())
	}

	for _, o := range orders {
		got, err := s.GetFollowerOrder(ctx, o.ID)
		if err != nil {
			t.Fatalf("GetFollowerOrder: %v", err)
		}
		if got.FollowerID == blocked {
			if got.TerminalStatus != domain.TerminalDeadLettered {
				t.Fatalf("blocked follower TerminalStatus = %q, want %q", got.TerminalStatus, domain.TerminalDeadLettered)
			}
		} else {
			if got.TerminalStatus == domain.TerminalDeadLettered {
				t.Fatalf("healthy follower was dead-lettered, should be unaffected")
			}
		}
	}
}

// F&O lot sizes are resolved from the instrument master, not hardcoded —
// this exercises a multi-unit commodity lot (MCX) to prove sizing isn't
// equity-specific.
func TestHandleMasterFill_FnOLotSizeResolvedFromInstrumentMaster(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	seedFollowLink(t, s, domain.FollowLink{FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(1), Enabled: true})
	seedInstrument(t, s, domain.Instrument{
		InstrumentToken: 2, Exchange: "MCX", Tradingsymbol: "CRUDEOIL24DECFUT",
		LotSize: 100, TickSize: decimal.NewFromFloat(1),
	})

	fill := domain.MasterFill{
		MasterID: master, BrokerOrderID: uuid.NewString(), Exchange: "MCX",
		Tradingsymbol: "CRUDEOIL24DECFUT", InstrumentToken: 2, TransactionType: "BUY",
		Product: "NRML", OrderType: "MARKET", FilledQuantity: 300,
		AveragePrice: decimal.NewFromInt(6500), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	}
	id, err := s.InsertMasterFill(ctx, fill)
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	fill.ID = id

	disp := newFakeDispatcher()
	e := engine.New(s, disp)
	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	if disp.jobCount() != 1 {
		t.Fatalf("got %d dispatched jobs, want 1", disp.jobCount())
	}
	if disp.jobs[0].Quantity != 300 {
		t.Fatalf("quantity = %d, want 300 (3 lots of 100)", disp.jobs[0].Quantity)
	}
}

// A signal on a symbol the instrument master has no row for (not yet
// synced, delisted, or a spoofed payload) must never fall back to
// trusting a lot size carried by the fill — it's recorded as a bad
// instrument, not silently sized.
func TestHandleMasterFill_UnknownInstrumentIsBadInstrumentNotDispatched(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	seedFollowLink(t, s, domain.FollowLink{FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(1), Enabled: true})

	fill := domain.MasterFill{
		MasterID: master, BrokerOrderID: uuid.NewString(), Exchange: "NFO",
		Tradingsymbol: "NOTSYNCED24DECFUT", InstrumentToken: 3, TransactionType: "BUY",
		Product: "NRML", OrderType: "MARKET", FilledQuantity: 50,
		AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	}
	id, err := s.InsertMasterFill(ctx, fill)
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	fill.ID = id

	disp := newFakeDispatcher()
	e := engine.New(s, disp)
	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	orders, err := s.FollowerOrdersByMasterFill(ctx, fill.ID)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("got %d follower_orders, want 1 (recorded, not silently dropped)", len(orders))
	}
	if disp.jobCount() != 0 {
		t.Fatalf("got %d dispatched jobs, want 0 for an unresolvable instrument", disp.jobCount())
	}

	got, err := s.GetFollowerOrder(ctx, orders[0].ID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if got.SizingReason != domain.ReasonBadInstrument {
		t.Fatalf("SizingReason = %v, want ReasonBadInstrument", got.SizingReason)
	}
	if got.IntendedQty != 0 {
		t.Fatalf("IntendedQty = %d, want 0", got.IntendedQty)
	}
}

// This is Milestone 4's PLAN.md exit criterion, reproduced at the
// fan-out layer: force-fail one follower, assert the rest are
// unaffected.
func TestHandleMasterFill_ForceFailOneFollowerOthersUnaffected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	badToken := seedAccount(t, s, "follower")
	good1 := seedAccount(t, s, "follower")
	good2 := seedAccount(t, s, "follower")
	for _, f := range []uuid.UUID{badToken, good1, good2} {
		seedFollowLink(t, s, domain.FollowLink{FollowerID: f, MasterID: master, CapitalRatio: decimal.NewFromFloat(1), Enabled: true})
	}

	fill := insertFill(t, s, master, 10)
	disp := newFakeDispatcher()
	disp.refuse[badToken] = true
	e := engine.New(s, disp)

	if err := e.HandleMasterFill(ctx, fill); err != nil {
		t.Fatalf("HandleMasterFill: %v", err)
	}

	if disp.jobCount() != 2 {
		t.Fatalf("got %d successful dispatches, want 2 (good1, good2)", disp.jobCount())
	}
	for _, f := range []uuid.UUID{good1, good2} {
		if disp.dispatch[f] != 1 {
			t.Fatalf("follower %v was dispatched to %d times, want 1", f, disp.dispatch[f])
		}
	}
}
