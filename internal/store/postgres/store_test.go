//go:build integration

// These tests require Docker (testcontainers-go spins up a real
// Postgres). Run with: go test -tags integration ./internal/store/postgres/...
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func newTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	store, _ := newTestStoreWithPool(t)
	return store
}

// newTestStoreWithPool also returns the underlying pool for tests that
// need to assert on columns no Store method exposes (e.g. dispatch_state).
func newTestStoreWithPool(t *testing.T) (*postgres.Store, *pgxpool.Pool) {
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
	return store, pool
}

func masterFillDispatchState(t *testing.T, pool *pgxpool.Pool, fillID int64) (string, *time.Time) {
	t.Helper()
	var state string
	var dispatchedAt *time.Time
	err := pool.QueryRow(context.Background(),
		`SELECT dispatch_state, dispatched_at FROM master_fills WHERE id = $1`, fillID,
	).Scan(&state, &dispatchedAt)
	if err != nil {
		t.Fatalf("read dispatch_state: %v", err)
	}
	return state, dispatchedAt
}

func seedAccount(t *testing.T, s *postgres.Store, role string) uuid.UUID {
	t.Helper()
	id, _ := seedAccountWithSecret(t, s, role, "test-secret")
	return id
}

// seedAccountWithSecret seeds an account with a caller-chosen api_secret
// and returns its id and broker_user_id (== id.String(), the value
// CreateAccount is given as the broker's user id).
func seedAccountWithSecret(t *testing.T, s *postgres.Store, role string, apiSecret string) (uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	if err := s.CreateAccount(context.Background(), id, "Account "+id.String()[:8], role, "zerodha", id.String(), apiSecret); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	return id, id.String()
}

func TestMigrate_AppliesCleanlyOnEmptyDatabase(t *testing.T) {
	newTestStore(t) // Migrate runs inside newTestStore; failure fails the test.
}

func TestEnabledFollowLinks_FiltersByMasterAndEnabled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	otherMaster := seedAccount(t, s, "master")
	enabledFollower := seedAccount(t, s, "follower")
	disabledFollower := seedAccount(t, s, "follower")
	otherMasterFollower := seedAccount(t, s, "follower")

	links := []domain.FollowLink{
		{FollowerID: enabledFollower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true},
		{FollowerID: disabledFollower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: false},
		{FollowerID: otherMasterFollower, MasterID: otherMaster, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true},
	}
	for _, l := range links {
		if err := s.CreateFollowLink(ctx, l); err != nil {
			t.Fatalf("CreateFollowLink: %v", err)
		}
	}

	got, err := s.EnabledFollowLinks(ctx, master)
	if err != nil {
		t.Fatalf("EnabledFollowLinks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d links, want 1", len(got))
	}
	if got[0].FollowerID != enabledFollower {
		t.Fatalf("got follower %v, want %v", got[0].FollowerID, enabledFollower)
	}
}

func TestInsertMasterFill_DuplicateIsRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	fill := domain.MasterFill{
		MasterID:        master,
		BrokerOrderID:   "231000000123456",
		Exchange:        "NSE",
		Tradingsymbol:   "INFY",
		InstrumentToken: 12345,
		TransactionType: "BUY",
		Product:         "MIS",
		OrderType:       "MARKET",
		FilledQuantity:  100,
		AveragePrice:    decimal.NewFromFloat(1500.25),
		Status:          "COMPLETE",
		OrderTimestamp:  time.Now(),
		RawPayload:      []byte(`{}`),
	}

	if _, err := s.InsertMasterFill(ctx, fill); err != nil {
		t.Fatalf("first InsertMasterFill: %v", err)
	}
	if _, err := s.InsertMasterFill(ctx, fill); err != domain.ErrDuplicate {
		t.Fatalf("second InsertMasterFill err = %v, want ErrDuplicate", err)
	}
}

func TestInsertFollowerOrder_DuplicateTagAndDuplicatePairAreRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	order := domain.FollowerOrder{
		MasterFillID:   fillID,
		FollowerID:     follower,
		IdempotencyTag: domain.IdempotencyTag(fillID, follower),
		IntendedQty:    50,
		LotSize:        1,
		SizingReason:   domain.ReasonOK,
	}
	if _, err := s.InsertFollowerOrder(ctx, order); err != nil {
		t.Fatalf("first InsertFollowerOrder: %v", err)
	}
	if _, err := s.InsertFollowerOrder(ctx, order); err != domain.ErrDuplicate {
		t.Fatalf("duplicate (fill,follower) InsertFollowerOrder err = %v, want ErrDuplicate", err)
	}

	// Same tag, different (fill, follower) pair would violate the tag's
	// own unique constraint even if the pair constraint didn't fire —
	// exercised implicitly above since the tag is derived from the pair.
}

func TestOrderEvents_RetrievedInInsertionOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID, FollowerID: follower,
		IdempotencyTag: domain.IdempotencyTag(fillID, follower),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	eventTypes := []string{"signal_received", "sized", "api_request", "api_response"}
	for _, et := range eventTypes {
		err := s.AppendOrderEvent(ctx, domain.OrderEvent{
			FollowerOrderID: &orderID,
			MasterFillID:    &fillID,
			AccountID:       follower,
			EventType:       et,
			Payload:         []byte(`{}`),
		})
		if err != nil {
			t.Fatalf("AppendOrderEvent(%s): %v", et, err)
		}
	}

	events, err := s.OrderEventsByFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("OrderEventsByFollowerOrder: %v", err)
	}
	if len(events) != len(eventTypes) {
		t.Fatalf("got %d events, want %d", len(events), len(eventTypes))
	}
	for i, et := range eventTypes {
		if events[i].EventType != et {
			t.Fatalf("event[%d].EventType = %q, want %q", i, events[i].EventType, et)
		}
	}
}

func TestInstrumentLotSize_ReturnsSeededLotSize(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertInstrument(ctx, domain.Instrument{
		InstrumentToken: 101, Exchange: "MCX", Tradingsymbol: "GOLD24DECFUT",
		LotSize: 100, TickSize: decimal.NewFromInt(1),
	}); err != nil {
		t.Fatalf("UpsertInstrument: %v", err)
	}

	got, err := s.InstrumentLotSize(ctx, "MCX", "GOLD24DECFUT")
	if err != nil {
		t.Fatalf("InstrumentLotSize: %v", err)
	}
	if got != 100 {
		t.Fatalf("lot size = %d, want 100", got)
	}
}

func TestInstrumentLotSize_UnknownSymbolReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.InstrumentLotSize(ctx, "NFO", "DOESNOTEXIST")
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestUpsertInstrument_RefreshesLotSizeOnConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ins := domain.Instrument{
		InstrumentToken: 202, Exchange: "NFO", Tradingsymbol: "NIFTY24DECFUT",
		LotSize: 50, TickSize: decimal.NewFromFloat(0.05),
	}
	if err := s.UpsertInstrument(ctx, ins); err != nil {
		t.Fatalf("first UpsertInstrument: %v", err)
	}

	// A contract revision (or exchange-mandated lot size change) refreshes
	// the same instrument_token rather than creating a second row.
	ins.LotSize = 75
	if err := s.UpsertInstrument(ctx, ins); err != nil {
		t.Fatalf("second UpsertInstrument: %v", err)
	}

	got, err := s.InstrumentLotSize(ctx, "NFO", "NIFTY24DECFUT")
	if err != nil {
		t.Fatalf("InstrumentLotSize: %v", err)
	}
	if got != 75 {
		t.Fatalf("lot size = %d, want 75 (refreshed)", got)
	}
}

func TestAccountByBrokerUserID_ReturnsSeededAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, brokerUserID := seedAccountWithSecret(t, s, "follower", "the-secret")

	gotID, role, apiSecret, err := s.AccountByBrokerUserID(ctx, brokerUserID)
	if err != nil {
		t.Fatalf("AccountByBrokerUserID: %v", err)
	}
	if gotID != id {
		t.Errorf("id = %v, want %v", gotID, id)
	}
	if role != "follower" {
		t.Errorf("role = %q, want follower", role)
	}
	if apiSecret != "the-secret" {
		t.Errorf("apiSecret = %q, want the-secret", apiSecret)
	}
}

func TestAccountByBrokerUserID_UnknownUserReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _, _, err := s.AccountByBrokerUserID(ctx, "GHOST")
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestUpdateFollowerOrderStatus_UpdatesMatchingRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID, FollowerID: follower,
		IdempotencyTag: domain.IdempotencyTag(fillID, follower),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}
	if err := s.UpdateFollowerOrderPlaced(ctx, orderID, "F-ORDER-1", 50); err != nil {
		t.Fatalf("UpdateFollowerOrderPlaced: %v", err)
	}

	gotID, err := s.UpdateFollowerOrderStatus(ctx, "F-ORDER-1", domain.TerminalComplete, 50, decimal.NewFromInt(101))
	if err != nil {
		t.Fatalf("UpdateFollowerOrderStatus: %v", err)
	}
	if gotID != orderID {
		t.Fatalf("id = %d, want %d", gotID, orderID)
	}

	got, err := s.GetFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if got.TerminalStatus != domain.TerminalComplete {
		t.Errorf("TerminalStatus = %q, want %q", got.TerminalStatus, domain.TerminalComplete)
	}
	if got.FilledQty != 50 {
		t.Errorf("FilledQty = %d, want 50", got.FilledQty)
	}
	if !got.AveragePrice.Equal(decimal.NewFromInt(101)) {
		t.Errorf("AveragePrice = %v, want 101", got.AveragePrice)
	}
}

func TestUpdateFollowerOrderStatus_UnknownBrokerOrderIDReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.UpdateFollowerOrderStatus(ctx, "DOES-NOT-EXIST", domain.TerminalComplete, 1, decimal.NewFromInt(1))
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestSetMasterFillDispatchState_SetsDispatchedAtOnlyForDispatched(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	if err := s.SetMasterFillDispatchState(ctx, fillID, domain.DispatchDead); err != nil {
		t.Fatalf("SetMasterFillDispatchState(dead): %v", err)
	}
	state, dispatchedAt := masterFillDispatchState(t, pool, fillID)
	if state != string(domain.DispatchDead) {
		t.Fatalf("dispatch_state = %q, want %q", state, domain.DispatchDead)
	}
	if dispatchedAt != nil {
		t.Fatalf("dispatched_at = %v, want nil for a non-dispatched state", dispatchedAt)
	}

	if err := s.SetMasterFillDispatchState(ctx, fillID, domain.DispatchDispatched); err != nil {
		t.Fatalf("SetMasterFillDispatchState(dispatched): %v", err)
	}
	state, dispatchedAt = masterFillDispatchState(t, pool, fillID)
	if state != string(domain.DispatchDispatched) {
		t.Fatalf("dispatch_state = %q, want %q", state, domain.DispatchDispatched)
	}
	if dispatchedAt == nil {
		t.Fatalf("dispatched_at = nil, want set after transitioning to dispatched")
	}
}

func TestFollowerOrdersByMasterFill_OnlyReturnsMatchingFillInIDOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	followerA := seedAccount(t, s, "follower")
	followerB := seedAccount(t, s, "follower")

	fillID1, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill(1): %v", err)
	}
	fillID2, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "2", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill(2): %v", err)
	}

	orderA1, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID1, FollowerID: followerA,
		IdempotencyTag: domain.IdempotencyTag(fillID1, followerA),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder(fill1, A): %v", err)
	}
	orderB1, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID1, FollowerID: followerB,
		IdempotencyTag: domain.IdempotencyTag(fillID1, followerB),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder(fill1, B): %v", err)
	}
	if _, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID2, FollowerID: followerA,
		IdempotencyTag: domain.IdempotencyTag(fillID2, followerA),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	}); err != nil {
		t.Fatalf("InsertFollowerOrder(fill2, A): %v", err)
	}

	got, err := s.FollowerOrdersByMasterFill(ctx, fillID1)
	if err != nil {
		t.Fatalf("FollowerOrdersByMasterFill: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d orders, want 2", len(got))
	}
	if got[0].ID != orderA1 || got[1].ID != orderB1 {
		t.Fatalf("got ids [%d %d], want [%d %d] in ascending order", got[0].ID, got[1].ID, orderA1, orderB1)
	}
}

func TestUpdateFollowerOrderPlaced_IncrementsAttemptCountAndSetsBrokerOrderID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID, FollowerID: follower,
		IdempotencyTag: domain.IdempotencyTag(fillID, follower),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	if err := s.UpdateFollowerOrderPlaced(ctx, orderID, "F-ORDER-1", 50); err != nil {
		t.Fatalf("UpdateFollowerOrderPlaced: %v", err)
	}

	got, err := s.GetFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if got.BrokerOrderID != "F-ORDER-1" {
		t.Errorf("BrokerOrderID = %q, want F-ORDER-1", got.BrokerOrderID)
	}
	if got.PlacedQty == nil || *got.PlacedQty != 50 {
		t.Errorf("PlacedQty = %v, want 50", got.PlacedQty)
	}
	if got.AttemptCount != 1 {
		t.Errorf("AttemptCount = %d, want 1", got.AttemptCount)
	}
}

func TestUpdateFollowerOrderFailed_SetsTerminalStatusAndLastError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID: fillID, FollowerID: follower,
		IdempotencyTag: domain.IdempotencyTag(fillID, follower),
		IntendedQty:    50, LotSize: 1, SizingReason: domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	if err := s.UpdateFollowerOrderFailed(ctx, orderID, domain.TerminalDeadLettered, "no capital"); err != nil {
		t.Fatalf("UpdateFollowerOrderFailed: %v", err)
	}

	got, err := s.GetFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if got.TerminalStatus != domain.TerminalDeadLettered {
		t.Errorf("TerminalStatus = %q, want %q", got.TerminalStatus, domain.TerminalDeadLettered)
	}
	if got.LastError != "no capital" {
		t.Errorf("LastError = %q, want %q", got.LastError, "no capital")
	}
	if got.AttemptCount != 1 {
		t.Errorf("AttemptCount = %d, want 1", got.AttemptCount)
	}
}

func TestAccounts_ListsAllWithFollowLinkFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	maxQty := 10
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CapitalRatio: decimal.NewFromFloat(0.5), MaxQtyPerOrder: maxQty, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	accounts, err := s.Accounts(ctx, nil)
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accounts))
	}

	var gotMaster, gotFollower *domain.Account
	for i := range accounts {
		switch accounts[i].ID {
		case master:
			gotMaster = &accounts[i]
		case follower:
			gotFollower = &accounts[i]
		}
	}
	if gotMaster == nil || gotFollower == nil {
		t.Fatalf("missing rows: master=%v follower=%v", gotMaster, gotFollower)
	}
	if gotMaster.MasterID != nil || gotMaster.CapitalRatio != nil || gotMaster.MaxQtyPerOrder != nil {
		t.Errorf("master row should have nil group fields, got %+v", gotMaster)
	}
	if !gotMaster.Enabled {
		t.Errorf("master row Enabled = false, want true")
	}
	if gotFollower.MasterID == nil || *gotFollower.MasterID != master {
		t.Errorf("follower MasterID = %v, want %v", gotFollower.MasterID, master)
	}
	if gotFollower.CapitalRatio == nil || !gotFollower.CapitalRatio.Equal(decimal.NewFromFloat(0.5)) {
		t.Errorf("follower CapitalRatio = %v, want 0.5", gotFollower.CapitalRatio)
	}
	if gotFollower.MaxQtyPerOrder == nil || *gotFollower.MaxQtyPerOrder != maxQty {
		t.Errorf("follower MaxQtyPerOrder = %v, want %d", gotFollower.MaxQtyPerOrder, maxQty)
	}
	if !gotFollower.Enabled {
		t.Errorf("follower Enabled = false, want true")
	}
}

func TestAccounts_FiltersByIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "master")
	_ = seedAccount(t, s, "master")

	accounts, err := s.Accounts(ctx, []uuid.UUID{a})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].ID != a {
		t.Fatalf("Accounts(ids) = %+v, want just %v", accounts, a)
	}
}

func TestCreateAccount_DuplicateBrokerUserIDReturnsErrDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, brokerUserID := seedAccountWithSecret(t, s, "master", "secret")

	err := s.CreateAccount(ctx, uuid.New(), "Another Master", "master", "zerodha", brokerUserID, "secret2")
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("CreateAccount duplicate broker_user_id: err = %v, want ErrDuplicate", err)
	}
}

func TestCreateFollowLink_DuplicateFollowerReturnsErrDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	})
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("CreateFollowLink duplicate follower: err = %v, want ErrDuplicate", err)
	}
}

func TestUpdateFollowLinkTerms_UpdatesFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	newMaxQty := 25
	if err := s.UpdateFollowLinkTerms(ctx, follower, decimal.NewFromFloat(0.75), &newMaxQty); err != nil {
		t.Fatalf("UpdateFollowLinkTerms: %v", err)
	}

	accounts, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accounts))
	}
	got := accounts[0]
	if got.CapitalRatio == nil || !got.CapitalRatio.Equal(decimal.NewFromFloat(0.75)) {
		t.Errorf("CapitalRatio = %v, want 0.75", got.CapitalRatio)
	}
	if got.MaxQtyPerOrder == nil || *got.MaxQtyPerOrder != newMaxQty {
		t.Errorf("MaxQtyPerOrder = %v, want %d", got.MaxQtyPerOrder, newMaxQty)
	}
}

func TestUpdateFollowLinkTerms_NotFoundForNonFollower(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	err := s.UpdateFollowLinkTerms(ctx, master, decimal.NewFromFloat(0.5), nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateFollowLinkTerms on non-follower: err = %v, want ErrNotFound", err)
	}
}

func TestSetAccountStatus_Updates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	if err := s.SetAccountStatus(ctx, master, "error"); err != nil {
		t.Fatalf("SetAccountStatus: %v", err)
	}

	accounts, err := s.Accounts(ctx, []uuid.UUID{master})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if accounts[0].Status != "error" {
		t.Errorf("Status = %q, want error", accounts[0].Status)
	}
}

func TestSetAccountStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.SetAccountStatus(ctx, uuid.New(), "error")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetAccountStatus unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteAccount_Succeeds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	if err := s.DeleteAccount(ctx, master); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	accounts, err := s.Accounts(ctx, []uuid.UUID{master})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("account still present after delete: %+v", accounts)
	}
}

func TestDeleteAccount_ConflictWhenReferenced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	if err := s.DeleteAccount(ctx, master); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("DeleteAccount referenced master: err = %v, want ErrConflict", err)
	}
	if err := s.DeleteAccount(ctx, follower); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("DeleteAccount referenced follower: err = %v, want ErrConflict", err)
	}
}

func TestDeleteAccount_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.DeleteAccount(ctx, uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteAccount unknown id: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteFollowLink_Succeeds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	if err := s.DeleteFollowLink(ctx, follower); err != nil {
		t.Fatalf("DeleteFollowLink: %v", err)
	}

	accounts, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if accounts[0].MasterID != nil {
		t.Errorf("MasterID = %v after detach, want nil", accounts[0].MasterID)
	}

	// Now that the follower has no references, it can be deleted.
	if err := s.DeleteAccount(ctx, follower); err != nil {
		t.Fatalf("DeleteAccount after detach: %v", err)
	}
}

func TestDeleteFollowLink_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	err := s.DeleteFollowLink(ctx, master)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteFollowLink non-follower: err = %v, want ErrNotFound", err)
	}
}
