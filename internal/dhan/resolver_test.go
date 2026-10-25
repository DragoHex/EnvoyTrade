package dhan_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"envoytrade/internal/dhan"
	"envoytrade/internal/domain"
)

const sampleScripMasterCSV = `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_TRADING_SYMBOL,SEM_LOT_UNITS,SEM_INSTRUMENT_NAME,SEM_EXPIRY_DATE,SEM_STRIKE_PRICE
NSE,E,11536,TATASTEEL,1,EQUITY,,0
NSE,E,2885,RELIANCE,1,EQUITY,,0
NSE,D,45210,NIFTY-Oct2026-25000-CE,25,OPTIDX,2026-10-29,25000.00
BSE,E,500325,RELIANCE,1,EQUITY,,0
MCX,M,251200,GOLD-Oct2026,1,FUTCOM,2026-10-05,0
`

func TestInstrumentResolver_LoadFromReader_Positive(t *testing.T) {
	resolver := dhan.NewInstrumentResolver(nil)
	err := resolver.LoadFromReader(strings.NewReader(sampleScripMasterCSV))
	if err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	// 1. Resolve NSE Equity
	seg, secID, lot, err := resolver.ResolveToSecurityID("NSE", "TATASTEEL")
	if err != nil {
		t.Fatalf("ResolveToSecurityID NSE TATASTEEL: %v", err)
	}
	if seg != "NSE_EQ" || secID != "11536" || lot != 1 {
		t.Errorf("got seg=%s, secID=%s, lot=%d; want NSE_EQ, 11536, 1", seg, secID, lot)
	}

	// 2. Resolve Derivative with NFO exchange alias
	seg, secID, lot, err = resolver.ResolveToSecurityID("NFO", "NIFTY-Oct2026-25000-CE")
	if err != nil {
		t.Fatalf("ResolveToSecurityID NFO NIFTY: %v", err)
	}
	if seg != "NSE_FNO" || secID != "45210" || lot != 25 {
		t.Errorf("got seg=%s, secID=%s, lot=%d; want NSE_FNO, 45210, 25", seg, secID, lot)
	}

	// 3. Resolve Derivative with NSE exchange
	seg, secID, lot, err = resolver.ResolveToSecurityID("NSE", "NIFTY-Oct2026-25000-CE")
	if err != nil {
		t.Fatalf("ResolveToSecurityID NSE NIFTY: %v", err)
	}
	if seg != "NSE_FNO" || secID != "45210" || lot != 25 {
		t.Errorf("got seg=%s, secID=%s, lot=%d; want NSE_FNO, 45210, 25", seg, secID, lot)
	}

	// 4. Resolve Reverse: SecurityID -> Symbol
	ex, sym, err := resolver.ResolveToSymbol("NSE_EQ", "11536")
	if err != nil {
		t.Fatalf("ResolveToSymbol NSE_EQ 11536: %v", err)
	}
	if ex != "NSE" || sym != "TATASTEEL" {
		t.Errorf("got ex=%s, sym=%s; want NSE, TATASTEEL", ex, sym)
	}

	// 5. Reverse derivative
	ex, sym, err = resolver.ResolveToSymbol("NSE_FNO", "45210")
	if err != nil {
		t.Fatalf("ResolveToSymbol NSE_FNO 45210: %v", err)
	}
	if (ex != "NFO" && ex != "NSE") || sym != "NIFTY-Oct2026-25000-CE" {
		t.Errorf("got ex=%s, sym=%s; want NFO/NSE, NIFTY-Oct2026-25000-CE", ex, sym)
	}
}

func TestInstrumentResolver_NegativeScenarios(t *testing.T) {
	resolver := dhan.NewInstrumentResolver(nil)
	_ = resolver.LoadFromReader(strings.NewReader(sampleScripMasterCSV))

	// Unknown Symbol
	_, _, _, err := resolver.ResolveToSecurityID("NSE", "UNKNOWN_SYMBOL")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown symbol, got %v", err)
	}

	// Unknown Security ID
	_, _, err = resolver.ResolveToSymbol("NSE_EQ", "9999999")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown security ID, got %v", err)
	}

	// Malformed CSV Header
	badCSV := `INVALID_HEADER_1,INVALID_HEADER_2\n1,2`
	err = resolver.LoadFromReader(strings.NewReader(badCSV))
	if err == nil {
		t.Errorf("expected error for malformed CSV, got nil")
	}
}

func TestInstrumentResolver_EdgeCases(t *testing.T) {
	resolver := dhan.NewInstrumentResolver(nil)

	// Whitespace & Empty inputs
	_, _, _, err := resolver.ResolveToSecurityID("", "   ")
	if err == nil {
		t.Errorf("expected error for empty inputs, got nil")
	}

	// Case Insensitivity
	_ = resolver.LoadFromReader(strings.NewReader(sampleScripMasterCSV))
	seg, secID, _, err := resolver.ResolveToSecurityID("nse", "tatasteel")
	if err != nil {
		t.Fatalf("case insensitivity failed: %v", err)
	}
	if seg != "NSE_EQ" || secID != "11536" {
		t.Errorf("got seg=%s, secID=%s; want NSE_EQ, 11536", seg, secID)
	}

	// Zero or Negative lot size in CSV should fallback to 1
	zeroLotCSV := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_TRADING_SYMBOL,SEM_LOT_UNITS
NSE,E,999,ZEROLOT,0
`
	_ = resolver.LoadFromReader(strings.NewReader(zeroLotCSV))
	_, _, lot, err := resolver.ResolveToSecurityID("NSE", "ZEROLOT")
	if err != nil {
		t.Fatalf("Resolve ZEROLOT: %v", err)
	}
	if lot != 1 {
		t.Errorf("expected lot=1 for 0 lot size in CSV, got %d", lot)
	}

	// Concurrency test: concurrent readers and re-loading
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _, _, _ = resolver.ResolveToSecurityID("NSE", "TATASTEEL")
				_, _, _ = resolver.ResolveToSymbol("NSE_EQ", "11536")
			}
		}()
	}
	// Re-load while reads are in flight
	_ = resolver.LoadFromReader(strings.NewReader(sampleScripMasterCSV))
	wg.Wait()
}
