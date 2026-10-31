package instrument

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"envoytrade/internal/domain"
)

type mockSyncerStore struct {
	prunedBroker        string
	insertedTokens      []int64
	insertedInstruments []domain.Instrument
	vacuumCalls         int
	pruneError          error
	insertError         error
}

func (m *mockSyncerStore) PruneInstrumentsByBroker(ctx context.Context, broker string) (int64, error) {
	if m.pruneError != nil {
		return 0, m.pruneError
	}
	m.prunedBroker = broker
	return 100, nil
}

func (m *mockSyncerStore) BatchInsertInstruments(ctx context.Context, instruments []domain.Instrument) error {
	if m.insertError != nil {
		return m.insertError
	}
	for _, ins := range instruments {
		m.insertedTokens = append(m.insertedTokens, ins.InstrumentToken)
		m.insertedInstruments = append(m.insertedInstruments, ins)
	}
	return nil
}

func (m *mockSyncerStore) VacuumInstruments(ctx context.Context) error {
	m.vacuumCalls++
	return nil
}

type mockHTTPClient struct {
	responses map[string]*http.Response
	err       error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	url := req.URL.String()
	for k, resp := range m.responses {
		if strings.Contains(url, k) {
			return resp, nil
		}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

// Positive Case 1: Dhan sync succeeds, prunes only dhan, batch inserts, and vacuums
func TestSyncer_SyncDhan_Success(t *testing.T) {
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_INSTRUMENT_NAME,SEM_EXPIRY_CODE,SEM_TRADING_SYMBOL,SEM_LOT_UNITS,SEM_CUSTOM_SYMBOL,SEM_EXPIRY_DATE,SEM_STRIKE_PRICE,SEM_OPTION_TYPE,SEM_TICK_SIZE,SEM_EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SEM_SERIES,SM_SYMBOL_NAME
MCX,M,582024,OPTFUT,0,GOLDM-29Oct2026-130000-PE,1.0,GOLDM 29 OCT 130000 PUT,2026-10-29 23:30:00,130000.00000,PE,50.0000,M,OPTFUT,2,GOLDM
`
	client := &mockHTTPClient{
		responses: map[string]*http.Response{
			"dhan": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(csvData)),
			},
		},
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)
	syncer.SetDhanURL("http://mock/dhan")

	err := syncer.SyncDhan(context.Background())
	if err != nil {
		t.Fatalf("SyncDhan failed: %v", err)
	}

	if store.prunedBroker != "dhan" {
		t.Fatalf("expected prunedBroker 'dhan', got %q", store.prunedBroker)
	}
	if len(store.insertedTokens) != 1 {
		t.Fatalf("expected 1 inserted token, got %d", len(store.insertedTokens))
	}
	if store.vacuumCalls != 1 {
		t.Fatalf("expected 1 vacuum call, got %d", store.vacuumCalls)
	}
}


// Unit test for isDhanEquity helper
func TestIsDhanEquity(t *testing.T) {
	tests := []struct {
		name           string
		segment        string
		instrumentName string
		want           bool
	}{
		{"NSE equity by segment E", "E", "EQUITY", true},
		{"BSE equity lowercase e", "e", "", true},
		{"NSE EGR cash equity by segment G", "G", "EQUITY", true},
		{"Spot index by segment I", "I", "INDEX", true},
		{"Spot index by name INDEX", "", "INDEX", true},
		{"Equity by instrument name", "", "EQUITY", true},
		{"NSE F&O derivative OPTIDX", "D", "OPTIDX", false},
		{"NSE F&O derivative OPTSTK", "D", "OPTSTK", false},
		{"NSE F&O derivative FUTSTK", "D", "FUTSTK", false},
		{"NSE F&O derivative FUTIDX", "D", "FUTIDX", false},
		{"BSE F&O derivative OPTIDX", "D", "OPTIDX", false},
		{"MCX Commodity OPTFUT", "M", "OPTFUT", false},
		{"MCX Commodity FUTCOM", "M", "FUTCOM", false},
		{"NSE Commodity derivative OPTFUT", "M", "OPTFUT", false},
		{"NSE Currency OPTCUR", "C", "OPTCUR", false},
		{"NSE Currency FUTCUR", "C", "FUTCUR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDhanEquity(tt.segment, tt.instrumentName)
			if got != tt.want {
				t.Errorf("isDhanEquity(%q, %q) = %v, want %v", tt.segment, tt.instrumentName, got, tt.want)
			}
		})
	}
}

// Positive Case 2: Dhan sync skips equity records and only inserts derivatives and commodities
func TestSyncer_SyncDhan_SkipsEquity(t *testing.T) {
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_INSTRUMENT_NAME,SEM_EXPIRY_CODE,SEM_TRADING_SYMBOL,SEM_LOT_UNITS,SEM_CUSTOM_SYMBOL,SEM_EXPIRY_DATE,SEM_STRIKE_PRICE,SEM_OPTION_TYPE,SEM_TICK_SIZE,SEM_EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SEM_SERIES,SM_SYMBOL_NAME
NSE,E,11536,EQUITY,0,TATASTEEL,1.0,TATASTEEL,,0.00,NA,0.05,NA,EQUITY,EQ,TATASTEEL
BSE,E,500325,EQUITY,0,RELIANCE,1.0,RELIANCE,,0.00,NA,0.05,NA,EQUITY,EQ,RELIANCE
NSE,G,35003,EQUITY,0,GOLD1G99,1.0,EGR 999 1 GRAM,,0.00,NA,100.0,NA,Other,EG,EGR 999 1 GRAM
NSE,I,1,INDEX,0,NIFTY 50,1.0,NIFTY 50,0001-01-01,0.00,XX,0.05,NA,INDEX,X,NIFTY 50
NSE,D,45210,OPTIDX,0,NIFTY-29Oct2026-25000-CE,25.0,NIFTY 29 OCT 25000 CALL,2026-10-29 15:30:00,25000.00000,CE,0.0500,D,OPTIDX,NA,NIFTY
NSE,M,35010,OPTFUT,0,GOLD-29Jan2027-168500-CE,1.0,GOLD 29 JAN 168500 CALL,2027-01-29 20:00:00,168500.00000,CE,50.0000,M,OPTFUT,1,GOLD
MCX,M,582024,OPTFUT,0,GOLDM-29Oct2026-130000-PE,1.0,GOLDM 29 OCT 130000 PUT,2026-10-29 23:30:00,130000.00000,PE,50.0000,M,OPTFUT,2,GOLDM
`
	client := &mockHTTPClient{
		responses: map[string]*http.Response{
			"dhan": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(csvData)),
			},
		},
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)
	syncer.SetDhanURL("http://mock/dhan")

	err := syncer.SyncDhan(context.Background())
	if err != nil {
		t.Fatalf("SyncDhan failed: %v", err)
	}

	if store.prunedBroker != "dhan" {
		t.Fatalf("expected prunedBroker 'dhan', got %q", store.prunedBroker)
	}
	if len(store.insertedInstruments) != 3 {
		t.Fatalf("expected 3 inserted instruments (NSE F&O, NSE Commodity option, MCX Commodity only), got %d", len(store.insertedInstruments))
	}

	for _, ins := range store.insertedInstruments {
		if ins.Tradingsymbol == "TATASTEEL" || ins.Tradingsymbol == "RELIANCE" || ins.Tradingsymbol == "GOLD1G99" || ins.Tradingsymbol == "NIFTY 50" {
			t.Fatalf("found equity/index instrument %s in inserted list; equities and indices should be skipped", ins.Tradingsymbol)
		}
		if ins.ExchangeSegment == "NSE_EQ" || ins.ExchangeSegment == "BSE_EQ" {
			t.Fatalf("found equity exchange segment %s in inserted list; equities should be skipped", ins.ExchangeSegment)
		}
	}
}

// Negative Case: When Dhan returns only equity records, nothing is inserted and existing records are NOT pruned
func TestSyncer_SyncDhan_OnlyEquity_SkippedAndNoPrune(t *testing.T) {
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_INSTRUMENT_NAME,SEM_EXPIRY_CODE,SEM_TRADING_SYMBOL,SEM_LOT_UNITS,SEM_CUSTOM_SYMBOL,SEM_EXPIRY_DATE,SEM_STRIKE_PRICE,SEM_OPTION_TYPE,SEM_TICK_SIZE,SEM_EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SEM_SERIES,SM_SYMBOL_NAME
NSE,E,11536,EQUITY,0,TATASTEEL,1.0,TATASTEEL,,0.00,NA,0.05,NA,EQUITY,EQ,TATASTEEL
BSE,E,500325,EQUITY,0,RELIANCE,1.0,RELIANCE,,0.00,NA,0.05,NA,EQUITY,EQ,RELIANCE
`
	client := &mockHTTPClient{
		responses: map[string]*http.Response{
			"dhan": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(csvData)),
			},
		},
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)
	syncer.SetDhanURL("http://mock/dhan")

	err := syncer.SyncDhan(context.Background())
	if err == nil {
		t.Fatal("expected error when feed has 0 non-equity instruments")
	}

	if store.prunedBroker != "" {
		t.Fatalf("prune should NOT have run when 0 non-equity instruments returned, got prunedBroker=%q", store.prunedBroker)
	}
	if len(store.insertedTokens) != 0 {
		t.Fatalf("insert should NOT have run when 0 non-equity instruments returned, got %d", len(store.insertedTokens))
	}
}

// Negative Case 1: Dhan download fails with HTTP 500 -> prune is strictly NOT called
func TestSyncer_SyncDhan_HTTPFailure_NoPrune(t *testing.T) {
	client := &mockHTTPClient{
		responses: map[string]*http.Response{
			"dhan": {
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("server error")),
			},
		},
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)
	syncer.SetDhanURL("http://mock/dhan")

	err := syncer.SyncDhan(context.Background())
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}

	if store.prunedBroker != "" {
		t.Fatalf("prune should NOT have run on failure, got prunedBroker=%q", store.prunedBroker)
	}
	if len(store.insertedTokens) != 0 {
		t.Fatalf("insert should NOT have run on failure, got %d", len(store.insertedTokens))
	}
}

// Positive Case 2: Kite sync succeeds, prunes only kite, batch inserts, and vacuums
func TestSyncer_SyncKite_Success(t *testing.T) {
	csvData := `instrument_token,exchange_token,tradingsymbol,name,last_price,expiry,strike,tick_size,lot_size,instrument_type,segment,exchange
221330181,864571,SENSEX26OCTFUT,"SENSEX",0,2026-10-29,0,0.05,20,FUT,BFO-FUT,BFO
`
	client := &mockHTTPClient{
		responses: map[string]*http.Response{
			"BFO": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(csvData)),
			},
		},
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)
	syncer.SetKiteBaseURL("http://mock/kite")

	err := syncer.SyncKite(context.Background(), []string{"BFO"})
	if err != nil {
		t.Fatalf("SyncKite failed: %v", err)
	}

	if store.prunedBroker != "kite" {
		t.Fatalf("expected prunedBroker 'kite', got %q", store.prunedBroker)
	}
	if len(store.insertedTokens) != 1 {
		t.Fatalf("expected 1 inserted token, got %d", len(store.insertedTokens))
	}
	if store.vacuumCalls != 1 {
		t.Fatalf("expected 1 vacuum call, got %d", store.vacuumCalls)
	}
}

// Negative Case 2: Network failure during Kite download -> prune is strictly NOT called
func TestSyncer_SyncKite_NetworkFailure_NoPrune(t *testing.T) {
	client := &mockHTTPClient{
		err: errors.New("network timeout"),
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)

	err := syncer.SyncKite(context.Background(), []string{"BFO"})
	if err == nil {
		t.Fatal("expected error on network timeout")
	}

	if store.prunedBroker != "" {
		t.Fatalf("prune should NOT have run on failure, got %q", store.prunedBroker)
	}
}

// Edge Case: NextRunDuration calculation for 05:00 IST
func TestNextRunDuration_CalculatesAccurately(t *testing.T) {
	istLocation := time.FixedZone("IST", 5*3600+1800)
	
	// Test before 5:00 AM (e.g. 03:00 AM IST) -> should wait 2 hours
	nowBefore := time.Date(2026, 10, 29, 3, 0, 0, 0, istLocation)
	durBefore := NextRunDuration(nowBefore, 5, 0)
	if durBefore != 2*time.Hour {
		t.Fatalf("expected 2h wait, got %v", durBefore)
	}

	// Test after 5:00 AM (e.g. 06:00 AM IST) -> should wait 23 hours
	nowAfter := time.Date(2026, 10, 29, 6, 0, 0, 0, istLocation)
	durAfter := NextRunDuration(nowAfter, 5, 0)
	if durAfter != 23*time.Hour {
		t.Fatalf("expected 23h wait, got %v", durAfter)
	}
}

func TestSyncer_SyncDhan_NormalizesDerivativeExchangeToNFOAndBFO(t *testing.T) {
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_INSTRUMENT_NAME,SEM_EXPIRY_CODE,SEM_TRADING_SYMBOL,SEM_LOT_UNITS,SEM_CUSTOM_SYMBOL,SEM_EXPIRY_DATE,SEM_STRIKE_PRICE,SEM_OPTION_TYPE,SEM_TICK_SIZE,SEM_EXPIRY_FLAG,SEM_EXCH_INSTRUMENT_TYPE,SEM_SERIES,SM_SYMBOL_NAME
NSE,D,53376,OPTIDX,0,NIFTY-Nov2026-21000-PE,65.0,NIFTY 03 NOV 21000 PUT,2026-11-03 14:30:00,21000.00000,PE,5.0000,W,OP,,
BSE,D,80012,OPTIDX,0,SENSEX-Nov2026-80000-CE,10.0,SENSEX 05 NOV 80000 CALL,2026-11-05 14:30:00,80000.00000,CE,5.0000,W,OP,,
MCX,M,582024,OPTFUT,0,GOLDM-29Oct2026-130000-PE,1.0,GOLDM 29 OCT 130000 PUT,2026-10-29 23:30:00,130000.00000,PE,50.0000,M,OPTFUT,2,GOLDM
`
	client := &mockHTTPClient{
		responses: map[string]*http.Response{
			"dhan": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(csvData)),
			},
		},
	}

	store := &mockSyncerStore{}
	syncer := NewSyncer(store, nil)
	syncer.SetHTTPClient(client)
	syncer.SetDhanURL("http://mock/dhan")

	if err := syncer.SyncDhan(context.Background()); err != nil {
		t.Fatalf("SyncDhan failed: %v", err)
	}

	if len(store.insertedInstruments) != 3 {
		t.Fatalf("expected 3 inserted instruments, got %d", len(store.insertedInstruments))
	}

	// 1. NSE derivative must be normalized to NFO
	nfoInst := store.insertedInstruments[0]
	if nfoInst.Exchange != "NFO" {
		t.Errorf("nfoInst.Exchange = %q, want 'NFO'", nfoInst.Exchange)
	}
	if nfoInst.ExchangeSegment != "NSE_FNO" {
		t.Errorf("nfoInst.ExchangeSegment = %q, want 'NSE_FNO'", nfoInst.ExchangeSegment)
	}
	if nfoInst.LotSize != 65 {
		t.Errorf("nfoInst.LotSize = %d, want 65", nfoInst.LotSize)
	}

	// 2. BSE derivative must be normalized to BFO
	bfoInst := store.insertedInstruments[1]
	if bfoInst.Exchange != "BFO" {
		t.Errorf("bfoInst.Exchange = %q, want 'BFO'", bfoInst.Exchange)
	}
	if bfoInst.ExchangeSegment != "BSE_FNO" {
		t.Errorf("bfoInst.ExchangeSegment = %q, want 'BSE_FNO'", bfoInst.ExchangeSegment)
	}
	if bfoInst.LotSize != 10 {
		t.Errorf("bfoInst.LotSize = %d, want 10", bfoInst.LotSize)
	}

	// 3. MCX remains MCX
	mcxInst := store.insertedInstruments[2]
	if mcxInst.Exchange != "MCX" {
		t.Errorf("mcxInst.Exchange = %q, want 'MCX'", mcxInst.Exchange)
	}
}
