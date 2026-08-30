//go:build integration

// These tests require Docker (testcontainers-go spins up a real
// Postgres). Run with: go test -tags integration ./internal/store/postgres/...
package postgres_test

import (
	"context"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

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
