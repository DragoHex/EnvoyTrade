//go:build integration

// These tests require Docker (testcontainers-go spins up a real
// Postgres). Run with: go test -tags integration ./internal/store/postgres/...
package postgres_test

import (
	"context"
	"errors"
	"fmt"
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
	ip := fmt.Sprintf("10.0.%d.%d", id[0], id[1])
	if err := s.CreateAccount(context.Background(), id, "Account "+id.String()[:8], role, "zerodha", id.String(), "test-api-key", apiSecret, ip); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if role == "master" {
		if err := s.CreateGroup(context.Background(), id, "Group "+id.String()[:8], id); err != nil {
			t.Fatalf("CreateGroup: %v", err)
		}
	}
	return id, id.String()
}

func seedAccountWithoutGroup(t *testing.T, s *postgres.Store, role string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ip := fmt.Sprintf("10.0.%d.%d", id[0], id[1])
	if err := s.CreateAccount(context.Background(), id, "Account "+id.String()[:8], role, "zerodha", id.String(), "test-api-key", "test-secret", ip); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	return id
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
		{FollowerID: enabledFollower, MasterID: master, CloneFactor: decimal.NewFromFloat(0.5), Enabled: true},
		{FollowerID: disabledFollower, MasterID: master, CloneFactor: decimal.NewFromFloat(0.5), Enabled: false},
		{FollowerID: otherMasterFollower, MasterID: otherMaster, CloneFactor: decimal.NewFromFloat(0.5), Enabled: true},
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

func TestInsertFollowerOrdersBatch_AtomicityAndConflictHandling(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	f1 := seedAccount(t, s, "follower")
	f2 := seedAccount(t, s, "follower")
	f3 := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: master, BrokerOrderID: "B1", Exchange: "NSE", Tradingsymbol: "RELIANCE",
		InstrumentToken: 1, TransactionType: "BUY", Product: "CNC", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(2500), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	orders := []domain.FollowerOrder{
		{
			MasterFillID:    fillID,
			FollowerID:      f1,
			IdempotencyTag:  domain.IdempotencyTag(fillID, f1),
			IntendedQty:     100,
			LotSize:         1,
			SizingReason:    domain.ReasonOK,
			Origin:          "copy_trade",
			Tradingsymbol:   "RELIANCE",
			Exchange:        "NSE",
			Product:         "CNC",
			TransactionType: "BUY",
			OrderType:       "MARKET",
		},
		{
			MasterFillID:    fillID,
			FollowerID:      f2,
			IdempotencyTag:  domain.IdempotencyTag(fillID, f2),
			IntendedQty:     200,
			LotSize:         1,
			SizingReason:    domain.ReasonOK,
			Origin:          "copy_trade",
			Tradingsymbol:   "RELIANCE",
			Exchange:        "NSE",
			Product:         "CNC",
			TransactionType: "BUY",
			OrderType:       "MARKET",
		},
		{
			MasterFillID:    fillID,
			FollowerID:      f3,
			IdempotencyTag:  domain.IdempotencyTag(fillID, f3),
			IntendedQty:     50,
			LotSize:         1,
			SizingReason:    domain.ReasonCapped,
			Origin:          "copy_trade",
			Tradingsymbol:   "RELIANCE",
			Exchange:        "NSE",
			Product:         "CNC",
			TransactionType: "BUY",
			OrderType:       "MARKET",
		},
	}

	// First batch insert: all 3 should be inserted
	res, err := s.InsertFollowerOrdersBatch(ctx, orders)
	if err != nil {
		t.Fatalf("InsertFollowerOrdersBatch: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 inserted orders, got %d", len(res))
	}
	for _, o := range orders {
		id, ok := res[o.IdempotencyTag]
		if !ok || id <= 0 {
			t.Errorf("missing or invalid id for tag %s", o.IdempotencyTag)
		}
	}

	// Second batch insert (redelivery of identical tags): ON CONFLICT DO NOTHING -> returns empty map
	res2, err := s.InsertFollowerOrdersBatch(ctx, orders)
	if err != nil {
		t.Fatalf("InsertFollowerOrdersBatch second run: %v", err)
	}
	if len(res2) != 0 {
		t.Fatalf("expected 0 inserted orders on duplicate redelivery, got %d", len(res2))
	}

	// Edge case 1: Empty slice should return empty map immediately with nil error
	emptyRes, err := s.InsertFollowerOrdersBatch(ctx, nil)
	if err != nil {
		t.Fatalf("InsertFollowerOrdersBatch(nil): %v", err)
	}
	if len(emptyRes) != 0 {
		t.Fatalf("expected 0 entries for empty orders slice, got %d", len(emptyRes))
	}

	// Edge case 2: Partial overlap (1 existing tag + 1 new tag)
	f4 := uuid.New()
	if err := s.CreateAccount(ctx, f4, "Follower 4", "follower", "kite", "FOLLOW04", "k4", "s4", "127.0.0.4"); err != nil {
		t.Fatalf("create follower 4: %v", err)
	}
	mixedOrders := []domain.FollowerOrder{
		orders[0], // existing tag: should be skipped
		{
			MasterFillID:    fillID,
			FollowerID:      f4,
			IdempotencyTag:  domain.IdempotencyTag(fillID, f4),
			IntendedQty:     30,
			LotSize:         1,
			SizingReason:    domain.ReasonOK,
			Origin:          "copy_trade",
			Tradingsymbol:   "RELIANCE",
			Exchange:        "NSE",
			Product:         "CNC",
			TransactionType: "BUY",
			OrderType:       "MARKET",
		},
	}
	mixedRes, err := s.InsertFollowerOrdersBatch(ctx, mixedOrders)
	if err != nil {
		t.Fatalf("InsertFollowerOrdersBatch partial overlap: %v", err)
	}
	if len(mixedRes) != 1 {
		t.Fatalf("expected 1 newly inserted order, got %d", len(mixedRes))
	}
	if _, ok := mixedRes[domain.IdempotencyTag(fillID, f4)]; !ok {
		t.Fatalf("expected new follower 4 order to be inserted")
	}

	// Edge case 3: Foreign key violation (non-existent follower UUID) should fail and rollback atomically
	badFollowerID := uuid.New()
	badOrders := []domain.FollowerOrder{
		{
			MasterFillID:    fillID,
			FollowerID:      badFollowerID,
			IdempotencyTag:  domain.IdempotencyTag(fillID, badFollowerID),
			IntendedQty:     10,
			LotSize:         1,
			SizingReason:    domain.ReasonOK,
			Tradingsymbol:   "RELIANCE",
			Exchange:        "NSE",
			Product:         "CNC",
			TransactionType: "BUY",
			OrderType:       "MARKET",
		},
	}
	_, err = s.InsertFollowerOrdersBatch(ctx, badOrders)
	if err == nil {
		t.Fatalf("expected foreign key error for non-existent follower, got nil")
	}
}

func TestStore_OrderTx_SynchronousCommitIsOff(t *testing.T) {
	ctx := context.Background()
	store, pool := newTestStoreWithPool(t)

	// Baseline: pool level synchronous_commit should default to 'on'
	var defaultSyncCommit string
	if err := pool.QueryRow(ctx, "SHOW synchronous_commit").Scan(&defaultSyncCommit); err != nil {
		t.Fatalf("query default synchronous_commit: %v", err)
	}
	if defaultSyncCommit != "on" {
		t.Errorf("default pool synchronous_commit = %s, want 'on'", defaultSyncCommit)
	}

	// Order tx should set synchronous_commit to 'off' within transaction
	tx, err := store.BeginOrderTx(ctx)
	if err != nil {
		t.Fatalf("BeginOrderTx: %v", err)
	}
	defer tx.Rollback(ctx)

	var txSyncCommit string
	if err := tx.QueryRow(ctx, "SHOW synchronous_commit").Scan(&txSyncCommit); err != nil {
		t.Fatalf("query tx synchronous_commit: %v", err)
	}
	if txSyncCommit != "off" {
		t.Errorf("order tx synchronous_commit = %s, want 'off'", txSyncCommit)
	}

	// After rollback/commit, pool level remains 'on'
	_ = tx.Rollback(ctx)
	var afterSyncCommit string
	if err := pool.QueryRow(ctx, "SHOW synchronous_commit").Scan(&afterSyncCommit); err != nil {
		t.Fatalf("query after synchronous_commit: %v", err)
	}
	if afterSyncCommit != "on" {
		t.Errorf("after tx synchronous_commit = %s, want 'on'", afterSyncCommit)
	}
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

func TestAccountByBrokerAndUserID_ReturnsMatchingBrokerAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, brokerUserID := seedAccountWithSecret(t, s, "follower", "secret-z")

	gotID, role, apiSecret, err := s.AccountByBrokerAndUserID(ctx, "zerodha", brokerUserID)
	if err != nil {
		t.Fatalf("AccountByBrokerAndUserID: %v", err)
	}
	if gotID != id || role != "follower" || apiSecret != "secret-z" {
		t.Errorf("got (%v, %q, %q), want (%v, follower, secret-z)", gotID, role, apiSecret, id)
	}
}

func TestAccountByBrokerAndUserID_MultiBrokerSameUserID_ResolvesCorrectly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	commonUserID := "SHARED_USER_123"
	id1 := uuid.New()
	id2 := uuid.New()

	if err := s.CreateAccount(ctx, id1, "Zero Acc", "follower", "zerodha", commonUserID, "key-z", "secret-z", "10.0.1.1"); err != nil {
		t.Fatalf("CreateAccount 1: %v", err)
	}
	if err := s.CreateAccount(ctx, id2, "Test Acc", "follower", "testbroker", commonUserID, "key-t", "secret-t", "10.0.1.2"); err != nil {
		t.Fatalf("CreateAccount 2: %v", err)
	}

	gotID1, _, secret1, err1 := s.AccountByBrokerAndUserID(ctx, "zerodha", commonUserID)
	if err1 != nil || gotID1 != id1 || secret1 != "secret-z" {
		t.Errorf("lookup zerodha = (%v, %v), want (%v, secret-z)", gotID1, err1, id1)
	}

	gotID2, _, secret2, err2 := s.AccountByBrokerAndUserID(ctx, "testbroker", commonUserID)
	if err2 != nil || gotID2 != id2 || secret2 != "secret-t" {
		t.Errorf("lookup testbroker = (%v, %v), want (%v, secret-t)", gotID2, err2, id2)
	}

	_, _, _, errUnknown := s.AccountByBrokerAndUserID(ctx, "nonexistent", commonUserID)
	if errUnknown != domain.ErrNotFound {
		t.Errorf("lookup unknown broker = %v, want domain.ErrNotFound", errUnknown)
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

func TestUpdateFollowerOrderPlaced_ConsumesStashedUpdateInSameTx(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
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

	// 1. Stash postback before placement response commits
	expectedPrice := decimal.NewFromFloat(123.45)
	if err := s.StashPendingOrderUpdate(ctx, "F-STASH-1", domain.TerminalComplete, 50, expectedPrice, []byte(`{"status":"COMPLETE"}`)); err != nil {
		t.Fatalf("StashPendingOrderUpdate: %v", err)
	}

	// 2. Worker placement returns and calls UpdateFollowerOrderPlaced
	if err := s.UpdateFollowerOrderPlaced(ctx, orderID, "F-STASH-1", 50); err != nil {
		t.Fatalf("UpdateFollowerOrderPlaced: %v", err)
	}

	// 3. Verify follower order was immediately marked COMPLETE with stashed price & qty (0ms delay)
	got, err := s.GetFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if got.BrokerOrderID != "F-STASH-1" {
		t.Errorf("BrokerOrderID = %q, want F-STASH-1", got.BrokerOrderID)
	}
	if got.TerminalStatus != domain.TerminalComplete {
		t.Errorf("TerminalStatus = %q, want %q", got.TerminalStatus, domain.TerminalComplete)
	}
	if got.FilledQty != 50 {
		t.Errorf("FilledQty = %d, want 50", got.FilledQty)
	}
	if !got.AveragePrice.Equal(expectedPrice) {
		t.Errorf("AveragePrice = %v, want %v", got.AveragePrice, expectedPrice)
	}

	// 4. Verify stashed row was deleted
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM pending_order_updates WHERE broker_order_id = $1`, "F-STASH-1").Scan(&count); err != nil {
		t.Fatalf("query pending_order_updates count: %v", err)
	}
	if count != 0 {
		t.Errorf("pending_order_updates count = %d, want 0 (must be consumed)", count)
	}

	// 5. Verify audit event was written
	events, err := s.OrderEventsByFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("OrderEventsByFollowerOrder: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected status_update audit event, got none")
	}
	if events[len(events)-1].EventType != "status_update" {
		t.Errorf("last event type = %q, want status_update", events[len(events)-1].EventType)
	}
}

func TestSweepPendingOrderUpdates(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	ctx := context.Background()

	// Insert stale update (>24h old) and fresh update (<24h old)
	_, err := pool.Exec(ctx, `
		INSERT INTO pending_order_updates (broker_order_id, status, filled_quantity, average_price, raw_payload, received_at)
		VALUES ($1, 'COMPLETE', 10, 100, '{}', now() - INTERVAL '25 hours')`, "STALE-1")
	if err != nil {
		t.Fatalf("insert stale pending update: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO pending_order_updates (broker_order_id, status, filled_quantity, average_price, raw_payload, received_at)
		VALUES ($1, 'COMPLETE', 10, 100, '{}', now())`, "FRESH-1")
	if err != nil {
		t.Fatalf("insert fresh pending update: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	deleted, err := s.SweepPendingOrderUpdates(ctx, cutoff)
	if err != nil {
		t.Fatalf("SweepPendingOrderUpdates: %v", err)
	}

	foundStale := false
	for _, id := range deleted {
		if id == "STALE-1" {
			foundStale = true
		}
		if id == "FRESH-1" {
			t.Errorf("fresh pending order update FRESH-1 was unexpectedly swept")
		}
	}
	if !foundStale {
		t.Errorf("SweepPendingOrderUpdates did not return STALE-1, got %v", deleted)
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
		CloneFactor: decimal.NewFromFloat(0.5), MaxQtyPerOrder: maxQty, Enabled: true,
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
	if gotMaster.MasterID != nil || gotMaster.CloneFactor != nil || gotMaster.MaxQtyPerOrder != nil {
		t.Errorf("master row should have nil group fields, got %+v", gotMaster)
	}
	if !gotMaster.Enabled {
		t.Errorf("master row Enabled = false, want true")
	}
	if gotFollower.MasterID == nil || *gotFollower.MasterID != master {
		t.Errorf("follower MasterID = %v, want %v", gotFollower.MasterID, master)
	}
	if gotFollower.CloneFactor == nil || !gotFollower.CloneFactor.Equal(decimal.NewFromFloat(0.5)) {
		t.Errorf("follower CloneFactor = %v, want 0.5", gotFollower.CloneFactor)
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

	err := s.CreateAccount(ctx, uuid.New(), "Another Master", "master", "zerodha", brokerUserID, "key2", "secret2", "127.0.0.1")
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
		CloneFactor: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master,
		CloneFactor: decimal.NewFromFloat(0.5), Enabled: true,
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
		CloneFactor: decimal.NewFromFloat(0.5), Enabled: true,
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
	if got.CloneFactor == nil || !got.CloneFactor.Equal(decimal.NewFromFloat(0.75)) {
		t.Errorf("CloneFactor = %v, want 0.75", got.CloneFactor)
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

	if err := s.SetAccountStatus(ctx, master, domain.AccountStatusError); err != nil {
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

func TestSetAccountStatus_Invalid(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	err := s.SetAccountStatus(ctx, master, domain.AccountStatus("invalid_value"))
	if !errors.Is(err, domain.ErrInvalidAccountStatus) {
		t.Fatalf("SetAccountStatus invalid status: err = %v, want ErrInvalidAccountStatus", err)
	}
}

func TestSetAccountStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.SetAccountStatus(ctx, uuid.New(), domain.AccountStatusError)
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
		CloneFactor: decimal.NewFromFloat(0.5), Enabled: true,
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
		CloneFactor: decimal.NewFromFloat(0.5), Enabled: true,
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

func TestDeleteAccount_WithOrderHistory_PreservesOrdersAndFreesIP(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	// Set a known IP on proxy_ips table and follower
	testIP := "192.168.100.55"
	if err := s.UpsertProxyIP(ctx, domain.ProxyIP{
		IPAddress:  testIP,
		IPType:     "ipv4",
		Host:       "proxy.example.com",
		Port:       8080,
		Username:   "user",
		Password:   "pass",
		ValidFrom:  time.Now().Add(-24 * time.Hour),
		ValidUntil: time.Now().Add(24 * time.Hour),
		Plan:       "PRO",
	}); err != nil {
		t.Fatalf("UpsertProxyIP: %v", err)
	}
	if err := s.SetAccountIPAddress(ctx, follower, testIP); err != nil {
		t.Fatalf("SetAccountIPAddress: %v", err)
	}

	// Verify testIP is NOT available while assigned to follower
	availBefore, err := s.AvailableProxyIPs(ctx, "", nil)
	if err != nil {
		t.Fatalf("AvailableProxyIPs before: %v", err)
	}
	for _, ip := range availBefore {
		if ip.IPAddress == testIP {
			t.Fatalf("expected %s to be assigned, but was in available list", testIP)
		}
	}

	// Insert master fill and follower order
	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID:       master,
		BrokerOrderID:  "m-order-1",
		Exchange:       "NSE",
		Tradingsymbol:  "RELIANCE",
		FilledQuantity: 10,
		AveragePrice:   decimal.NewFromInt(2500),
		Status:         "COMPLETE",
		OrderTimestamp: time.Now(),
		RawPayload:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID:   fillID,
		FollowerID:     follower,
		IdempotencyTag: "tag-1",
		IntendedQty:    10,
		LotSize:        1,
		SizingReason:   domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	// Now delete the follower. Even though order history exists, this MUST succeed!
	if err := s.DeleteAccount(ctx, follower); err != nil {
		t.Fatalf("DeleteAccount with order history failed: %v, want success", err)
	}

	// 1. Account should no longer appear in Accounts listing
	accs, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accs) != 0 {
		t.Fatalf("expected account to be hidden, found: %+v", accs)
	}

	// 2. Proxy IP should immediately be available again
	availAfter, err := s.AvailableProxyIPs(ctx, "", nil)
	if err != nil {
		t.Fatalf("AvailableProxyIPs after: %v", err)
	}
	found := false
	for _, ip := range availAfter {
		if ip.IPAddress == testIP {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected IP %s to be freed and available, but was not found in available list", testIP)
	}

	// 3. Follower order MUST still exist in database
	var gotFollowerID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT follower_id FROM follower_orders WHERE id = $1", orderID).Scan(&gotFollowerID)
	if err != nil {
		t.Fatalf("follower_orders row should still exist in DB, got err = %v", err)
	}
	if gotFollowerID != follower {
		t.Fatalf("gotFollowerID = %v, want %v", gotFollowerID, follower)
	}
}

func TestReactivateAccount_RestoresOrderHistory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	accs, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil || len(accs) == 0 {
		t.Fatalf("Accounts: %v", err)
	}
	brokerUserID := accs[0].BrokerAccountID

	// Create an order
	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID:       master,
		BrokerOrderID:  "m-order-reactivate",
		Exchange:       "NSE",
		Tradingsymbol:  "INFY",
		FilledQuantity: 10,
		AveragePrice:   decimal.NewFromInt(1500),
		Status:         "COMPLETE",
		OrderTimestamp: time.Now(),
		RawPayload:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}
	placedQty := 10
	_, err = s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID:   fillID,
		FollowerID:     follower,
		IdempotencyTag: "tag-reactivate-1",
		IntendedQty:    10,
		LotSize:        1,
		SizingReason:   domain.ReasonOK,
		PlacedQty:      &placedQty,
		TerminalStatus: "COMPLETE",
		FilledQty:      10,
		Tradingsymbol:  "INFY",
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	// Delete the follower
	if err := s.DeleteAccount(ctx, follower); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	// Account is hidden
	delAccs, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil || len(delAccs) != 0 {
		t.Fatalf("expected 0 accounts for deleted follower, got %d", len(delAccs))
	}

	// Re-add the same broker user id with CreateAccountWithCredentials
	newID := uuid.New()
	restoredID, err := s.CreateAccountWithCredentials(ctx, newID, "Restored Account", "follower", "zerodha", brokerUserID, "new-key", "new-secret", "10.0.99.1", "new-pass", "new-totp")
	if err != nil {
		t.Fatalf("CreateAccountWithCredentials (reactivation): %v", err)
	}
	if restoredID != follower {
		t.Fatalf("restoredID = %v, want original ID %v", restoredID, follower)
	}

	// The account should now appear in Accounts listing with new details
	restoredAccs, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil || len(restoredAccs) != 1 {
		t.Fatalf("Accounts after reactivation: len = %d, want 1, err = %v", len(restoredAccs), err)
	}
	if restoredAccs[0].Name != "Restored Account" {
		t.Errorf("Name = %v, want Restored Account", restoredAccs[0].Name)
	}
	if restoredAccs[0].IPAddress != "10.0.99.1" {
		t.Errorf("IPAddress = %v, want 10.0.99.1", restoredAccs[0].IPAddress)
	}

	// Order history should immediately be accessible again!
	ordersDetail, err := s.AccountOrders(ctx, follower, "open_orders", 1, 10)
	if err != nil {
		t.Fatalf("AccountOrders: %v", err)
	}
	if len(ordersDetail.OpenOrders) == 0 {
		t.Fatalf("expected order history to be restored, but found 0 open orders")
	}

	// Attempting to re-add while already active should return ErrDuplicate
	_, err = s.CreateAccountWithCredentials(ctx, uuid.New(), "Another Account", "follower", "zerodha", brokerUserID, "k", "s", "10.0.99.2", "p", "t")
	if !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("re-adding active account err = %v, want ErrDuplicate", err)
	}
}

func TestDeleteMasterAccount_WithFillsAndEmptyGroup_DissolvesGroupAndPreservesFills(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID:       master,
		BrokerOrderID:  "master-order-x",
		Exchange:       "NSE",
		Tradingsymbol:  "TCS",
		FilledQuantity: 5,
		AveragePrice:   decimal.NewFromInt(3800),
		Status:         "COMPLETE",
		OrderTimestamp: time.Now(),
		RawPayload:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	// Master has group with 0 followers. Deleting master must succeed!
	if err := s.DeleteAccount(ctx, master); err != nil {
		t.Fatalf("DeleteAccount master: %v", err)
	}

	// 1. Master is hidden from Accounts
	accs, err := s.Accounts(ctx, []uuid.UUID{master})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accs) != 0 {
		t.Fatalf("expected master to be hidden, got %+v", accs)
	}

	// 2. Group was dissolved
	groups, err := s.Groups(ctx)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups after master deletion, got %d", len(groups))
	}

	// 3. master_fills row STILL EXISTS in database
	var gotMasterID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT master_id FROM master_fills WHERE id = $1", fillID).Scan(&gotMasterID)
	if err != nil {
		t.Fatalf("master_fills row should still exist in DB, got err = %v", err)
	}
	if gotMasterID != master {
		t.Fatalf("gotMasterID = %v, want %v", gotMasterID, master)
	}
}

func TestDeleteAccount_WithOrderEvents_PreservesAuditTrail(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	fillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID:       master,
		BrokerOrderID:  "m-order-events",
		Exchange:       "NSE",
		Tradingsymbol:  "SBIN",
		FilledQuantity: 20,
		AveragePrice:   decimal.NewFromInt(600),
		Status:         "COMPLETE",
		OrderTimestamp: time.Now(),
		RawPayload:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID:   fillID,
		FollowerID:     follower,
		IdempotencyTag: "tag-events-1",
		IntendedQty:    20,
		LotSize:        1,
		SizingReason:   domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	// Append order event for follower account
	if err := s.AppendOrderEvent(ctx, domain.OrderEvent{
		FollowerOrderID: &orderID,
		MasterFillID:    &fillID,
		AccountID:       follower,
		EventType:       "placed",
		Payload:         []byte(`{"status":"placed"}`),
	}); err != nil {
		t.Fatalf("AppendOrderEvent: %v", err)
	}

	// Delete follower with order events
	if err := s.DeleteAccount(ctx, follower); err != nil {
		t.Fatalf("DeleteAccount with order events: %v", err)
	}

	// order_events row MUST still exist in database
	var eventCount int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM order_events WHERE account_id = $1", follower).Scan(&eventCount)
	if err != nil {
		t.Fatalf("query order_events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("order_events count = %d, want 1", eventCount)
	}
}

func TestDeletedAccount_HiddenFromLookupAndOrders(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	follower := seedAccount(t, s, "follower")

	accs, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil || len(accs) == 0 {
		t.Fatalf("Accounts: %v", err)
	}
	brokerUserID := accs[0].BrokerAccountID

	// Delete follower
	if err := s.DeleteAccount(ctx, follower); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	// 1. AccountByBrokerUserID MUST return ErrNotFound (so postback webhook drops callbacks for deleted account)
	_, _, _, err = s.AccountByBrokerUserID(ctx, brokerUserID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AccountByBrokerUserID for deleted account = %v, want ErrNotFound", err)
	}

	// 2. AccountOrders MUST return ErrNotFound
	_, err = s.AccountOrders(ctx, follower, "open_orders", 1, 10)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AccountOrders for deleted account = %v, want ErrNotFound", err)
	}
}

func TestReactivateAccount_AttachToGroupAndIPConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	// Follower attached to master's group
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID:  follower,
		MasterID:    master,
		CloneFactor: decimal.NewFromFloat(1.0),
		Enabled:     true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	accs, err := s.Accounts(ctx, []uuid.UUID{follower})
	if err != nil || len(accs) == 0 {
		t.Fatalf("Accounts: %v", err)
	}
	brokerUserID := accs[0].BrokerAccountID

	// Detach follower and delete it
	if err := s.DeleteFollowLink(ctx, follower); err != nil {
		t.Fatalf("DeleteFollowLink: %v", err)
	}
	if err := s.DeleteAccount(ctx, follower); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	// Seed another active account holding IP 10.0.88.88
	activeOther := uuid.New()
	if err := s.CreateAccount(ctx, activeOther, "Other", "follower", "zerodha", "OTHER_USER", "k", "s", "10.0.88.88"); err != nil {
		t.Fatalf("CreateAccount activeOther: %v", err)
	}

	// Attempting to reactivate with 10.0.88.88 MUST return ErrIPAlreadyAssigned
	_, err = s.CreateAccountWithCredentials(ctx, uuid.New(), "Reactivated", "follower", "zerodha", brokerUserID, "k", "s", "10.0.88.88", "p", "t")
	if !errors.Is(err, domain.ErrIPAlreadyAssigned) {
		t.Fatalf("reactivating with taken IP err = %v, want ErrIPAlreadyAssigned", err)
	}

	// Reactivating with free IP 10.0.88.99 succeeds and returns original UUID
	restoredID, err := s.CreateAccountWithCredentials(ctx, uuid.New(), "Reactivated Follower", "follower", "zerodha", brokerUserID, "k", "s", "10.0.88.99", "p", "t")
	if err != nil {
		t.Fatalf("reactivate with free IP: %v", err)
	}
	if restoredID != follower {
		t.Fatalf("restoredID = %v, want original %v", restoredID, follower)
	}

	// Attach restored follower back to group
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID:  restoredID,
		MasterID:    master,
		CloneFactor: decimal.NewFromFloat(2.0),
		Enabled:     true,
	}); err != nil {
		t.Fatalf("CreateFollowLink after reactivation: %v", err)
	}

	// Verify GroupDetail shows restored follower
	detail, err := s.GroupDetail(ctx, master)
	if err != nil {
		t.Fatalf("GroupDetail: %v", err)
	}
	if len(detail.Followers) != 1 {
		t.Fatalf("GroupDetail followers count = %d, want 1", len(detail.Followers))
	}
	if detail.Followers[0].AccountID != follower {
		t.Errorf("detail AccountID = %v, want %v", detail.Followers[0].AccountID, follower)
	}
}

func TestSetAccountAPIKey_UpdatesKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	accountID, _ := seedAccountWithSecret(t, s, "master", "initial-secret")

	if err := s.SetAccountAPIKey(ctx, accountID, "new-api-key"); err != nil {
		t.Fatalf("SetAccountAPIKey: %v", err)
	}

	accs, err := s.Accounts(ctx, []uuid.UUID{accountID})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("Accounts len = %d, want 1", len(accs))
	}
	if accs[0].ApiKey != "new-api-key" {
		t.Errorf("ApiKey = %q, want %q", accs[0].ApiKey, "new-api-key")
	}
	if accs[0].ApiSecret != "initial-secret" {
		t.Errorf("ApiSecret = %q, want %q", accs[0].ApiSecret, "initial-secret")
	}
}

func TestSetAccountAPISecret_UpdatesSecret(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	accountID, _ := seedAccountWithSecret(t, s, "follower", "initial-secret")

	if err := s.SetAccountAPISecret(ctx, accountID, "updated-secret"); err != nil {
		t.Fatalf("SetAccountAPISecret: %v", err)
	}

	accs, err := s.Accounts(ctx, []uuid.UUID{accountID})
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(accs) != 1 {
		t.Fatalf("Accounts len = %d, want 1", len(accs))
	}
	if accs[0].ApiSecret != "updated-secret" {
		t.Errorf("ApiSecret = %q, want %q", accs[0].ApiSecret, "updated-secret")
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

func TestInsertDirectFollowerOrder_InsertsAndFetchesDirectOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	followerID := seedAccount(t, s, "follower")

	upd := domain.OrderUpdate{
		FollowerID:      followerID,
		BrokerOrderID:   "DIRECT-001",
		Tradingsymbol:   "RELIANCE",
		Exchange:        "NSE",
		Product:         "CNC",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        5,
		FilledQuantity:  5,
		AveragePrice:    decimal.NewFromFloat(2500.50),
		Status:          domain.TerminalComplete,
	}

	orderID, err := s.InsertDirectFollowerOrder(ctx, upd)
	if err != nil {
		t.Fatalf("InsertDirectFollowerOrder: %v", err)
	}
	if orderID <= 0 {
		t.Fatalf("got orderID %d, want > 0", orderID)
	}

	order, err := s.GetFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if order.MasterFillID != 0 {
		t.Errorf("MasterFillID = %d, want 0", order.MasterFillID)
	}
	if order.BrokerOrderID != "DIRECT-001" {
		t.Errorf("BrokerOrderID = %q, want DIRECT-001", order.BrokerOrderID)
	}
	if order.Tradingsymbol != "RELIANCE" || order.Exchange != "NSE" {
		t.Errorf("Tradingsymbol/Exchange = %q/%q", order.Tradingsymbol, order.Exchange)
	}
	if order.Origin != "manual" {
		t.Errorf("Origin = %q, want manual", order.Origin)
	}
	if order.TerminalStatus != domain.TerminalComplete {
		t.Errorf("TerminalStatus = %q, want COMPLETE", order.TerminalStatus)
	}
	if order.FilledQty != 5 {
		t.Errorf("FilledQty = %d, want 5", order.FilledQty)
	}
}

func TestUpdateFollowerOrderByTag_ResolvesInFlightWorkerOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	masterID := seedAccount(t, s, "master")
	followerID := seedAccount(t, s, "follower")
	masterFillID, err := s.InsertMasterFill(ctx, domain.MasterFill{
		MasterID: masterID, BrokerOrderID: "MF-TAG-1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 100, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now(), RawPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("InsertMasterFill: %v", err)
	}

	tag := "test-tag-race-12345"
	orderID, err := s.InsertFollowerOrder(ctx, domain.FollowerOrder{
		MasterFillID:   masterFillID,
		FollowerID:     followerID,
		IdempotencyTag: tag,
		IntendedQty:    10,
		LotSize:        1,
		SizingReason:   domain.ReasonOK,
	})
	if err != nil {
		t.Fatalf("InsertFollowerOrder: %v", err)
	}

	// Postback arrives with tag before worker placement write
	updatedID, err := s.UpdateFollowerOrderByTag(ctx, followerID, tag, "BROKER-ORD-123", domain.TerminalComplete, 10, decimal.NewFromFloat(100.5))
	if err != nil {
		t.Fatalf("UpdateFollowerOrderByTag: %v", err)
	}
	if updatedID != orderID {
		t.Errorf("updatedID = %d, want %d", updatedID, orderID)
	}

	order, err := s.GetFollowerOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("GetFollowerOrder: %v", err)
	}
	if order.BrokerOrderID != "BROKER-ORD-123" {
		t.Errorf("BrokerOrderID = %q, want BROKER-ORD-123", order.BrokerOrderID)
	}
	if order.TerminalStatus != domain.TerminalComplete {
		t.Errorf("TerminalStatus = %q, want COMPLETE", order.TerminalStatus)
	}
}

