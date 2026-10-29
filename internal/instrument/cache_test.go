package instrument

import (
	"context"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
)

type mockStore struct {
	mu           sync.Mutex
	activeFO     []domain.Instrument
	lotSizes     map[string]int
	lookupCalls  int
	activeErrors error
}

func (m *mockStore) ActiveFOInstrumentsForCache(ctx context.Context) ([]domain.Instrument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeErrors != nil {
		return nil, m.activeErrors
	}
	return m.activeFO, nil
}

func (m *mockStore) InstrumentLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lookupCalls++
	key := exchange + ":" + tradingsymbol
	if lot, ok := m.lotSizes[key]; ok {
		return lot, nil
	}
	return 0, domain.ErrNotFound
}

// Positive Case 1: Pre-allocated capacity is respected
func TestCache_PreallocatedCapacity(t *testing.T) {
	c := NewCache(10000, nil, nil)
	if c.Capacity() != 10000 {
		t.Fatalf("expected capacity 10000, got %d", c.Capacity())
	}
	if c.Count() != 0 {
		t.Fatalf("expected initial count 0, got %d", c.Count())
	}
}

// Positive Case 2: Auto-load fills active F&O contracts
func TestCache_AutoLoadActiveFO(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		activeFO: []domain.Instrument{
			{
				Broker:          "dhan",
				Exchange:        "MCX",
				Tradingsymbol:   "GOLDM-29Oct2026-130000-PE",
				LotSize:         1,
				SecurityID:      "582024",
				ExchangeSegment: "MCX_COMM",
				Expiry:          &now,
			},
			{
				Broker:          "kite",
				Exchange:        "NFO",
				Tradingsymbol:   "NIFTY26OCT24000CE",
				LotSize:         25,
				Expiry:          &now,
			},
		},
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	if cache.Count() != 2 {
		t.Fatalf("expected count 2, got %d", cache.Count())
	}

	// Verify resolve lot size without DB lookup
	lot, err := cache.ResolveLotSize(context.Background(), "MCX", "GOLDM-29Oct2026-130000-PE")
	if err != nil || lot != 1 {
		t.Fatalf("expected lot 1, got %d (err: %v)", lot, err)
	}
	if store.lookupCalls != 0 {
		t.Fatalf("expected 0 DB lookup calls, got %d", store.lookupCalls)
	}

	// Verify Dhan security ID resolution
	seg, secID, lot, err := cache.ResolveToSecurityID("MCX", "GOLDM-29Oct2026-130000-PE")
	if err != nil || seg != "MCX_COMM" || secID != "582024" || lot != 1 {
		t.Fatalf("ResolveToSecurityID mismatch: seg=%s, secID=%s, lot=%d, err=%v", seg, secID, lot, err)
	}

	// Verify Dhan reverse symbol resolution
	ex, sym, err := cache.ResolveToSymbol("MCX_COMM", "582024")
	if err != nil || ex != "MCX" || sym != "GOLDM-29Oct2026-130000-PE" {
		t.Fatalf("ResolveToSymbol mismatch: ex=%s, sym=%s, err=%v", ex, sym, err)
	}
}

// Negative Case: Cache miss falls back to store and caches result
func TestCache_MissAndFallthrough(t *testing.T) {
	store := &mockStore{
		lotSizes: map[string]int{
			"MCX:SILVERMIC26NOVFUT": 5,
		},
	}

	cache := NewCache(10000, store, nil)

	// First call: DB hit
	lot, err := cache.ResolveLotSize(context.Background(), "MCX", "SILVERMIC26NOVFUT")
	if err != nil || lot != 5 {
		t.Fatalf("expected lot 5, got %d (err: %v)", lot, err)
	}
	if store.lookupCalls != 1 {
		t.Fatalf("expected 1 DB lookup call, got %d", store.lookupCalls)
	}

	// Second call: in-memory cache hit (no new DB call)
	lot2, err := cache.ResolveLotSize(context.Background(), "MCX", "SILVERMIC26NOVFUT")
	if err != nil || lot2 != 5 {
		t.Fatalf("expected lot 5, got %d", lot2)
	}
	if store.lookupCalls != 1 {
		t.Fatalf("expected lookupCalls to remain 1, got %d", store.lookupCalls)
	}

	// Unknown contract returns ErrNotFound and is negative-cached
	_, err = cache.ResolveLotSize(context.Background(), "NFO", "NONEXISTENT")
	if err == nil {
		t.Fatal("expected ErrNotFound for unknown contract")
	}
	if store.lookupCalls != 2 {
		t.Fatalf("expected 2 DB lookup calls, got %d", store.lookupCalls)
	}

	// Second query for unknown contract hits negative cache
	_, err = cache.ResolveLotSize(context.Background(), "NFO", "NONEXISTENT")
	if err == nil {
		t.Fatal("expected ErrNotFound from negative cache")
	}
	if store.lookupCalls != 2 {
		t.Fatalf("expected negative cache hit (calls=2), got %d", store.lookupCalls)
	}
}

// Edge Case 1: Capacity ceiling stops map growth beyond pre-allocated limit
func TestCache_CapacityCeiling(t *testing.T) {
	cache := NewCache(2, nil, nil) // Small capacity 2

	now := time.Now()
	store := &mockStore{
		activeFO: []domain.Instrument{
			{Exchange: "MCX", Tradingsymbol: "S1", LotSize: 1, Expiry: &now},
			{Exchange: "MCX", Tradingsymbol: "S2", LotSize: 2, Expiry: &now},
			{Exchange: "MCX", Tradingsymbol: "S3", LotSize: 3, Expiry: &now}, // Should be omitted due to ceiling
		},
	}
	cache.store = store

	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	if cache.Count() != 2 {
		t.Fatalf("expected count capped at 2, got %d", cache.Count())
	}
}

// Edge Case 2: Empty or malformed inputs
func TestCache_EdgeCases_EmptyInputs(t *testing.T) {
	cache := NewCache(10000, nil, nil)

	if _, err := cache.ResolveLotSize(context.Background(), "", ""); err == nil {
		t.Fatal("expected error on empty exchange/symbol")
	}
	if _, _, _, err := cache.ResolveToSecurityID("", ""); err == nil {
		t.Fatal("expected error on empty security ID query")
	}
	if _, _, err := cache.ResolveToSymbol("", ""); err == nil {
		t.Fatal("expected error on empty reverse lookup")
	}
}
