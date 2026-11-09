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

func TestCache_CrossBrokerResolution_KotakToDhan(t *testing.T) {
	dhanInst := domain.Instrument{
		InstrumentToken: 1583041,
		Broker:          "dhan",
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL-17Nov2026-5000-PE",
		SecurityID:      "583041",
		ExchangeSegment: "MCX_COMM",
		LotSize:         1,
	}

	store := &mockStore{
		activeFO: []domain.Instrument{dhanInst},
		secIDs: map[string]struct {
			secID string
			exSeg string
			lot   int
		}{
			"MCX:CRUDEOIL-17Nov2026-5000-PE": {
				secID: "583041",
				exSeg: "MCX_COMM",
				lot:   1,
			},
		},
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	// 1. Query with Kotak format via ResolveForBroker
	seg, secID, lot, targetSym, err := cache.ResolveForBroker("dhan", "MCX", "CRUDEOIL17NOV265000PE")
	if err != nil {
		t.Fatalf("ResolveForBroker failed: %v", err)
	}
	if secID != "583041" || seg != "MCX_COMM" || lot != 1 || targetSym != "CRUDEOIL-17Nov2026-5000-PE" {
		t.Errorf("ResolveForBroker mismatch: seg=%s, secID=%s, lot=%d, targetSym=%s", seg, secID, lot, targetSym)
	}

	// 2. Query with Kotak format via ResolveToSecurityID
	seg, secID, lot, err = cache.ResolveToSecurityID("MCX", "CRUDEOIL17NOV265000PE")
	if err != nil {
		t.Fatalf("ResolveToSecurityID failed: %v", err)
	}
	if secID != "583041" || seg != "MCX_COMM" || lot != 1 {
		t.Errorf("ResolveToSecurityID mismatch: seg=%s, secID=%s, lot=%d", seg, secID, lot)
	}
}

func TestCache_CrossBrokerResolution_KotakToKite(t *testing.T) {
	kiteInst := domain.Instrument{
		InstrumentToken: 256265,
		Broker:          "kite",
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26NOV5000PE",
		SecurityID:      "", // Kite has no security ID
		ExchangeSegment: "MCX",
		LotSize:         100,
	}

	store := &mockStore{
		activeFO: []domain.Instrument{kiteInst},
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	// Query with Kotak format via ResolveForBroker targeting Kite
	_, _, lot, targetSym, err := cache.ResolveForBroker("kite", "MCX", "CRUDEOIL17NOV265000PE")
	if err != nil {
		t.Fatalf("ResolveForBroker failed: %v", err)
	}
	if targetSym != "CRUDEOIL26NOV5000PE" {
		t.Errorf("targetSym = %q, want CRUDEOIL26NOV5000PE", targetSym)
	}
	if lot != 100 {
		t.Errorf("lot = %d, want 100", lot)
	}
}

func TestCache_CrossBrokerMatrix_Comprehensive(t *testing.T) {
	instruments := []domain.Instrument{
		// Dhan MCX
		{Broker: "dhan", Exchange: "MCX", Tradingsymbol: "NATGASMINI-20Nov2026-115-CE", SecurityID: "589278", ExchangeSegment: "MCX_COMM", LotSize: 250},
		{Broker: "dhan", Exchange: "MCX", Tradingsymbol: "GOLDM-05Mar2027-FUT", SecurityID: "584500", ExchangeSegment: "MCX_COMM", LotSize: 100},
		// Kite MCX
		{Broker: "kite", Exchange: "MCX", Tradingsymbol: "NATGASMINI26NOV115CE", SecurityID: "", ExchangeSegment: "MCX", LotSize: 250},
		{Broker: "kite", Exchange: "MCX", Tradingsymbol: "GOLDM27MARFUT", SecurityID: "", ExchangeSegment: "MCX", LotSize: 100},
		// Dhan NFO Weekly
		{Broker: "dhan", Exchange: "NFO", Tradingsymbol: "NIFTY-03Nov2026-28350-CE", SecurityID: "54896", ExchangeSegment: "NSE_FNO", LotSize: 65},
		// Kite NFO Weekly
		{Broker: "kite", Exchange: "NFO", Tradingsymbol: "NIFTY26N0328350CE", SecurityID: "", ExchangeSegment: "NFO", LotSize: 65},
		// Dhan BFO Weekly
		{Broker: "dhan", Exchange: "BFO", Tradingsymbol: "SENSEX-17Dec2026-72000-PE", SecurityID: "1172542", ExchangeSegment: "BSE_FNO", LotSize: 20},
		// Kite BFO Weekly
		{Broker: "kite", Exchange: "BFO", Tradingsymbol: "SENSEX26D1772000PE", SecurityID: "", ExchangeSegment: "BFO", LotSize: 20},
		// Dhan Stock Option
		{Broker: "dhan", Exchange: "NFO", Tradingsymbol: "INFY-Nov2026-1000-CE", SecurityID: "104780", ExchangeSegment: "NSE_FNO", LotSize: 400},
		// Kite Stock Option
		{Broker: "kite", Exchange: "NFO", Tradingsymbol: "INFY26NOV1000CE", SecurityID: "", ExchangeSegment: "NFO", LotSize: 400},
	}

	store := &mockStore{
		activeFO: instruments,
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	// 1. From Kotak MCX Option NATGASMINI20NOV26115CE -> Dhan and Kite
	segDhan, secIDDhan, lotDhan, symDhan, err := cache.ResolveForBroker("dhan", "MCX", "NATGASMINI20NOV26115CE")
	if err != nil || secIDDhan != "589278" || symDhan != "NATGASMINI-20Nov2026-115-CE" || lotDhan != 250 {
		t.Errorf("Kotak->Dhan NATGASMINI mismatch: seg=%s, secID=%s, lot=%d, sym=%s, err=%v", segDhan, secIDDhan, lotDhan, symDhan, err)
	}
	_, _, lotKite, symKite, err := cache.ResolveForBroker("kite", "MCX", "NATGASMINI20NOV26115CE")
	if err != nil || symKite != "NATGASMINI26NOV115CE" || lotKite != 250 {
		t.Errorf("Kotak->Kite NATGASMINI mismatch: lot=%d, sym=%s, err=%v", lotKite, symKite, err)
	}

	// 2. From Kotak NFO Weekly NIFTY26N0328350CE -> Dhan and Kite
	segDhan, secIDDhan, lotDhan, symDhan, err = cache.ResolveForBroker("dhan", "NFO", "NIFTY26N0328350CE")
	if err != nil || secIDDhan != "54896" || symDhan != "NIFTY-03Nov2026-28350-CE" || lotDhan != 65 {
		t.Errorf("Kotak->Dhan NIFTY Weekly mismatch: seg=%s, secID=%s, lot=%d, sym=%s, err=%v", segDhan, secIDDhan, lotDhan, symDhan, err)
	}
	_, _, lotKite, symKite, err = cache.ResolveForBroker("kite", "NFO", "NIFTY26N0328350CE")
	if err != nil || symKite != "NIFTY26N0328350CE" || lotKite != 65 {
		t.Errorf("Kotak->Kite NIFTY Weekly mismatch: lot=%d, sym=%s, err=%v", lotKite, symKite, err)
	}

	// 3. From Kotak BFO Weekly SENSEX26D1772000PE -> Dhan and Kite
	segDhan, secIDDhan, lotDhan, symDhan, err = cache.ResolveForBroker("dhan", "BFO", "SENSEX26D1772000PE")
	if err != nil || secIDDhan != "1172542" || symDhan != "SENSEX-17Dec2026-72000-PE" || lotDhan != 20 {
		t.Errorf("Kotak->Dhan SENSEX Weekly mismatch: seg=%s, secID=%s, lot=%d, sym=%s, err=%v", segDhan, secIDDhan, lotDhan, symDhan, err)
	}
	_, _, lotKite, symKite, err = cache.ResolveForBroker("kite", "BFO", "SENSEX26D1772000PE")
	if err != nil || symKite != "SENSEX26D1772000PE" || lotKite != 20 {
		t.Errorf("Kotak->Kite SENSEX Weekly mismatch: lot=%d, sym=%s, err=%v", lotKite, symKite, err)
	}

	// 4. From Kotak Stock Option INFY26NOV1000CE -> Dhan and Kite
	segDhan, secIDDhan, lotDhan, symDhan, err = cache.ResolveForBroker("dhan", "NFO", "INFY26NOV1000CE")
	if err != nil || secIDDhan != "104780" || symDhan != "INFY-Nov2026-1000-CE" || lotDhan != 400 {
		t.Errorf("Kotak->Dhan Stock Option mismatch: seg=%s, secID=%s, lot=%d, sym=%s, err=%v", segDhan, secIDDhan, lotDhan, symDhan, err)
	}
	_, _, lotKite, symKite, err = cache.ResolveForBroker("kite", "NFO", "INFY26NOV1000CE")
	if err != nil || symKite != "INFY26NOV1000CE" || lotKite != 400 {
		t.Errorf("Kotak->Kite Stock Option mismatch: lot=%d, sym=%s, err=%v", lotKite, symKite, err)
	}

	// 5. From Kotak MCX Future GOLDM05MAR27FUT -> Dhan and Kite
	segDhan, secIDDhan, lotDhan, symDhan, err = cache.ResolveForBroker("dhan", "MCX", "GOLDM05MAR27FUT")
	if err != nil || secIDDhan != "584500" || symDhan != "GOLDM-05Mar2027-FUT" || lotDhan != 100 {
		t.Errorf("Kotak->Dhan MCX Future mismatch: seg=%s, secID=%s, lot=%d, sym=%s, err=%v", segDhan, secIDDhan, lotDhan, symDhan, err)
	}
	_, _, lotKite, symKite, err = cache.ResolveForBroker("kite", "MCX", "GOLDM05MAR27FUT")
	if err != nil || symKite != "GOLDM27MARFUT" || lotKite != 100 {
		t.Errorf("Kotak->Kite MCX Future mismatch: lot=%d, sym=%s, err=%v", lotKite, symKite, err)
	}
}

func TestCache_CrossBrokerMatrix_DhanMaster_To_KotakAndKite(t *testing.T) {
	instruments := []domain.Instrument{
		// Dhan MCX
		{Broker: "dhan", Exchange: "MCX", Tradingsymbol: "CRUDEOIL-17Nov2026-5000-PE", SecurityID: "459190", ExchangeSegment: "MCX_COMM", LotSize: 100},
		{Broker: "dhan", Exchange: "MCX", Tradingsymbol: "CRUDEOIL-21Sep2026-FUT", SecurityID: "459100", ExchangeSegment: "MCX_COMM", LotSize: 100},
		// Kotak MCX (Notice: SecurityID is empty because Kotak does not use Security IDs)
		{Broker: "kotak", Exchange: "MCX", Tradingsymbol: "CRUDEOIL17NOV265000PE", SecurityID: "", ExchangeSegment: "mcx_fo", LotSize: 100},
		{Broker: "kotak", Exchange: "MCX", Tradingsymbol: "CRUDEOIL21SEP26", SecurityID: "", ExchangeSegment: "mcx_fo", LotSize: 100},
		// Kite MCX
		{Broker: "kite", Exchange: "MCX", Tradingsymbol: "CRUDEOIL26NOV5000PE", SecurityID: "", ExchangeSegment: "MCX", LotSize: 100},
		{Broker: "kite", Exchange: "MCX", Tradingsymbol: "CRUDEOIL26SEPFUT", SecurityID: "", ExchangeSegment: "MCX", LotSize: 100},
		// Dhan NFO Weekly
		{Broker: "dhan", Exchange: "NFO", Tradingsymbol: "NIFTY-03Nov2026-28350-CE", SecurityID: "54896", ExchangeSegment: "NSE_FNO", LotSize: 65},
		// Kotak NFO Weekly
		{Broker: "kotak", Exchange: "NFO", Tradingsymbol: "NIFTY26N0328350CE", SecurityID: "", ExchangeSegment: "nse_fo", LotSize: 65},
		// Kite NFO Weekly
		{Broker: "kite", Exchange: "NFO", Tradingsymbol: "NIFTY26N0328350CE", SecurityID: "", ExchangeSegment: "NFO", LotSize: 65},
	}

	store := &mockStore{
		activeFO: instruments,
	}

	cache := NewCache(10000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO failed: %v", err)
	}

	// 1. Dhan Master MCX Option -> Kotak Neo Follower
	segKotak, _, lotKotak, symKotak, err := cache.ResolveForBroker("kotak", "MCX", "CRUDEOIL-17Nov2026-5000-PE")
	if err != nil {
		t.Fatalf("ResolveForBroker('kotak') failed: %v", err)
	}
	if symKotak != "CRUDEOIL17NOV265000PE" {
		t.Errorf("symKotak = %q, want CRUDEOIL17NOV265000PE", symKotak)
	}
	if segKotak != "mcx_fo" {
		t.Errorf("segKotak = %q, want mcx_fo", segKotak)
	}
	if lotKotak != 100 {
		t.Errorf("lotKotak = %d, want 100", lotKotak)
	}

	// 2. Dhan Master MCX Option -> Kite Follower
	_, _, lotKite, symKite, err := cache.ResolveForBroker("kite", "MCX", "CRUDEOIL-17Nov2026-5000-PE")
	if err != nil {
		t.Fatalf("ResolveForBroker('kite') failed: %v", err)
	}
	if symKite != "CRUDEOIL26NOV5000PE" {
		t.Errorf("symKite = %q, want CRUDEOIL26NOV5000PE", symKite)
	}
	if lotKite != 100 {
		t.Errorf("lotKite = %d, want 100", lotKite)
	}

	// 3. Dhan Master MCX Future -> Kotak Neo Follower
	segKotak, _, lotKotak, symKotak, err = cache.ResolveForBroker("kotak", "MCX", "CRUDEOIL-21Sep2026-FUT")
	if err != nil {
		t.Fatalf("ResolveForBroker('kotak') future failed: %v", err)
	}
	if symKotak != "CRUDEOIL21SEP26" {
		t.Errorf("symKotak = %q, want CRUDEOIL21SEP26", symKotak)
	}
	if segKotak != "mcx_fo" {
		t.Errorf("segKotak = %q, want mcx_fo", segKotak)
	}

	// 4. Dhan Master NFO Weekly -> Kotak Neo Follower
	segKotak, _, lotKotak, symKotak, err = cache.ResolveForBroker("kotak", "NFO", "NIFTY-03Nov2026-28350-CE")
	if err != nil {
		t.Fatalf("ResolveForBroker('kotak') NFO failed: %v", err)
	}
	if symKotak != "NIFTY26N0328350CE" {
		t.Errorf("symKotak = %q, want NIFTY26N0328350CE", symKotak)
	}
	if segKotak != "nse_fo" {
		t.Errorf("segKotak = %q, want nse_fo", segKotak)
	}
}

func TestCache_KotakAlgorithmicFallback_WhenNotPreloadedInDB(t *testing.T) {
	// Empty store: Kotak instruments are NOT in DB
	store := &mockStore{}
	cache := NewCache(1000, store, nil)

	// 1. Dhan MCX Option -> Kotak algorithmic fallback
	seg, secID, _, sym, err := cache.ResolveForBroker("kotak", "MCX", "CRUDEOIL-17Nov2026-5000-PE")
	if err != nil {
		t.Fatalf("algorithmic fallback to kotak failed: %v", err)
	}
	if sym != "CRUDEOIL17NOV265000PE" {
		t.Errorf("sym = %q, want CRUDEOIL17NOV265000PE", sym)
	}
	if seg != "mcx_fo" {
		t.Errorf("seg = %q, want mcx_fo", seg)
	}
	if secID != "" {
		t.Errorf("secID = %q, want empty for Kotak", secID)
	}

	// 2. Dhan Cash Equity -> Kotak algorithmic fallback (should format with -EQ)
	seg, _, _, sym, err = cache.ResolveForBroker("kotak", "NSE", "RELIANCE")
	if err != nil {
		t.Fatalf("algorithmic fallback for equity failed: %v", err)
	}
	if sym != "RELIANCE-EQ" {
		t.Errorf("sym = %q, want RELIANCE-EQ", sym)
	}
	if seg != "nse_cm" {
		t.Errorf("seg = %q, want nse_cm", seg)
	}
}

func TestCache_KiteMasterToKotak_DayEnrichmentFromCache(t *testing.T) {
	// Dhan has MCX with day 17 loaded in cache
	instruments := []domain.Instrument{
		{Broker: "dhan", Exchange: "MCX", Tradingsymbol: "CRUDEOIL-17Nov2026-5000-PE", SecurityID: "459190", ExchangeSegment: "MCX_COMM", LotSize: 100},
	}
	store := &mockStore{activeFO: instruments}
	cache := NewCache(1000, store, nil)
	if err := cache.AutoLoadActiveFO(context.Background()); err != nil {
		t.Fatalf("AutoLoadActiveFO: %v", err)
	}

	// Kite master sends dayless CRUDEOIL26NOV5000PE -> Kotak Neo follower needs CRUDEOIL17NOV265000PE
	seg, _, lot, sym, err := cache.ResolveForBroker("kotak", "MCX", "CRUDEOIL26NOV5000PE")
	if err != nil {
		t.Fatalf("ResolveForBroker('kotak') with day enrichment failed: %v", err)
	}
	if sym != "CRUDEOIL17NOV265000PE" {
		t.Errorf("sym = %q, want CRUDEOIL17NOV265000PE (enriched with day 17 from canonical cache)", sym)
	}
	if seg != "mcx_fo" {
		t.Errorf("seg = %q, want mcx_fo", seg)
	}
	if lot != 100 {
		t.Errorf("lot = %d, want 100", lot)
	}
}



