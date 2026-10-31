package instrument

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
)

type mockStore struct {
	mu           sync.Mutex
	activeFO     []domain.Instrument
	lotSizes     map[string]int
	secIDs       map[string]struct {
		secID string
		exSeg string
		lot   int
	}
	lookupCalls  int
	secIDCalls   int
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

func (m *mockStore) InstrumentSecurityID(ctx context.Context, exchange, tradingsymbol string) (string, string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secIDCalls++
	targetKey := strings.ToUpper(exchange) + ":" + strings.ToUpper(tradingsymbol)
	for k, entry := range m.secIDs {
		if strings.ToUpper(k) == targetKey {
			return entry.secID, entry.exSeg, entry.lot, nil
		}
	}
	return "", "", 0, domain.ErrNotFound
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

// Positive Case 3: Database fallback on cache miss for ResolveToSecurityID
func TestCache_ResolveToSecurityID_DatabaseFallbackOnCacheMiss(t *testing.T) {
	store := &mockStore{
		secIDs: map[string]struct {
			secID string
			exSeg string
			lot   int
		}{
			"NFO:NIFTY-Nov2026-21000-PE": {
				secID: "53376",
				exSeg: "NSE_FNO",
				lot:   65,
			},
		},
	}

	cache := NewCache(10000, store, nil)

	// Cold lookup: triggers DB fallback
	seg, secID, lot, err := cache.ResolveToSecurityID("NFO", "NIFTY-Nov2026-21000-PE")
	if err != nil {
		t.Fatalf("expected successful resolution via DB fallback, got: %v", err)
	}
	if seg != "NSE_FNO" || secID != "53376" || lot != 65 {
		t.Fatalf("unexpected result: seg=%s, secID=%s, lot=%d", seg, secID, lot)
	}
	if store.secIDCalls != 1 {
		t.Fatalf("expected 1 DB call, got %d", store.secIDCalls)
	}

	// Warm lookup: cached in memory, no additional DB call
	seg2, secID2, lot2, err := cache.ResolveToSecurityID("NFO", "NIFTY-Nov2026-21000-PE")
	if err != nil || seg2 != "NSE_FNO" || secID2 != "53376" || lot2 != 65 {
		t.Fatalf("unexpected cached result: seg=%s, secID=%s, lot=%d, err=%v", seg2, secID2, lot2, err)
	}
	if store.secIDCalls != 1 {
		t.Fatalf("expected secIDCalls to remain 1, got %d", store.secIDCalls)
	}

	// Reverse lookup should also be populated from the fallback
	ex, sym, err := cache.ResolveToSymbol("NSE_FNO", "53376")
	if err != nil || ex != "NFO" || sym != "NIFTY-Nov2026-21000-PE" {
		t.Fatalf("unexpected reverse lookup result: ex=%s, sym=%s, err=%v", ex, sym, err)
	}
}

// Positive Case 4: AutoLoadActiveFO preserves nearest expiry when duplicate trading symbols exist
func TestCache_AutoLoadActiveFO_PreservesNearestExpiryOnDuplicateTradingSymbol(t *testing.T) {
	now := time.Now()
	nextWeek := now.AddDate(0, 0, 7)
	twoWeeks := now.AddDate(0, 0, 14)

	store := &mockStore{
		activeFO: []domain.Instrument{
			{
				Broker:          "dhan",
				Exchange:        "NFO",
				Tradingsymbol:   "NIFTY-Nov2026-21000-PE",
				LotSize:         65,
				SecurityID:      "53376",
				ExchangeSegment: "NSE_FNO",
				Expiry:          &now, // Nearest expiry
			},
			{
				Broker:          "dhan",
				Exchange:        "NFO",
				Tradingsymbol:   "NIFTY-Nov2026-21000-PE",
				LotSize:         65,
				SecurityID:      "53377",
				ExchangeSegment: "NSE_FNO",
				Expiry:          &nextWeek, // Next week expiry with identical trading symbol
			},
			{
				Broker:          "dhan",
				Exchange:        "NFO",
				Tradingsymbol:   "NIFTY-Nov2026-21000-PE",
				LotSize:         65,
				SecurityID:      "53378",
				ExchangeSegment: "NSE_FNO",
				Expiry:          &twoWeeks, // Two weeks expiry with identical trading symbol
			},
		},
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	// Forward resolution MUST resolve to the nearest expiry (SecurityID 53376)
	seg, secID, lot, err := cache.ResolveToSecurityID("NFO", "NIFTY-Nov2026-21000-PE")
	if err != nil || secID != "53376" || seg != "NSE_FNO" || lot != 65 {
		t.Fatalf("expected nearest expiry SecurityID 53376, got: seg=%s, secID=%s, lot=%d, err=%v", seg, secID, lot, err)
	}

	// ALL three security IDs MUST be resolvable via reverse lookup
	for _, expectedSecID := range []string{"53376", "53377", "53378"} {
		ex, sym, err := cache.ResolveToSymbol("NSE_FNO", expectedSecID)
		if err != nil || ex != "NFO" || sym != "NIFTY-Nov2026-21000-PE" {
			t.Fatalf("reverse lookup failed for secID %s: ex=%s, sym=%s, err=%v", expectedSecID, ex, sym, err)
		}
	}
}

// Edge Case 3: Derivative exchange aliases (NFO <-> NSE, BFO <-> BSE)
func TestCache_DerivativeExchangeAliases(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		activeFO: []domain.Instrument{
			{
				Broker:          "dhan",
				Exchange:        "NFO",
				Tradingsymbol:   "BANKNIFTY-Nov2026-50000-CE",
				LotSize:         30,
				SecurityID:      "60001",
				ExchangeSegment: "NSE_FNO",
				Expiry:          &now,
			},
			{
				Broker:          "dhan",
				Exchange:        "BFO",
				Tradingsymbol:   "SENSEX-Nov2026-80000-CE",
				LotSize:         10,
				SecurityID:      "70001",
				ExchangeSegment: "BSE_FNO",
				Expiry:          &now,
			},
		},
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	// Query with NSE alias for NFO
	seg, secID, lot, err := cache.ResolveToSecurityID("NSE", "BANKNIFTY-Nov2026-50000-CE")
	if err != nil || secID != "60001" || seg != "NSE_FNO" || lot != 30 {
		t.Fatalf("NSE alias query failed: seg=%s, secID=%s, lot=%d, err=%v", seg, secID, lot, err)
	}

	// Query with BSE alias for BFO
	seg, secID, lot, err = cache.ResolveToSecurityID("BSE", "SENSEX-Nov2026-80000-CE")
	if err != nil || secID != "70001" || seg != "BSE_FNO" || lot != 10 {
		t.Fatalf("BSE alias query failed: seg=%s, secID=%s, lot=%d, err=%v", seg, secID, lot, err)
	}

	// Query ResolveLotSize with aliases
	lot, err = cache.ResolveLotSize(context.Background(), "NSE", "BANKNIFTY-Nov2026-50000-CE")
	if err != nil || lot != 30 {
		t.Fatalf("ResolveLotSize NSE alias failed: lot=%d, err=%v", lot, err)
	}

	lot, err = cache.ResolveLotSize(context.Background(), "BSE", "SENSEX-Nov2026-80000-CE")
	if err != nil || lot != 10 {
		t.Fatalf("ResolveLotSize BSE alias failed: lot=%d, err=%v", lot, err)
	}
}
