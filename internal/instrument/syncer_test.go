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
	prunedBroker   string
	insertedTokens []int64
	vacuumCalls    int
	pruneError     error
	insertError    error
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
