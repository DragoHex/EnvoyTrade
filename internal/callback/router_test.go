package callback_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"envoytrade/internal/callback"
)

type recordHandler struct {
	mu     sync.Mutex
	called int
	bodies [][]byte
}

func (h *recordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.called++
	body, _ := io.ReadAll(r.Body)
	h.bodies = append(h.bodies, body)
	w.WriteHeader(http.StatusOK)
}

func (h *recordHandler) getCalled() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.called
}

func TestAdaptiveRouter_Fingerprinting(t *testing.T) {
	kiteH := &recordHandler{}
	angelH := &recordHandler{}
	growwH := &recordHandler{}
	dhanH := &recordHandler{}
	kotakH := &recordHandler{}
	symphonyH := &recordHandler{}

	router := callback.NewAdaptiveRouter()
	router.Register("kite", kiteH)
	router.Register("zerodha", kiteH)
	router.Register("angelone", angelH)
	router.Register("groww", growwH)
	router.Register("dhan", dhanH)
	router.Register("kotakneo", kotakH)
	router.Register("symphony", symphonyH)
	router.SetDefault(kiteH)

	tests := []struct {
		name         string
		body         string
		query        string
		headerBroker string
		path         string
		wantKite     int
		wantAngel    int
		wantGroww    int
		wantDhan     int
		wantKotak    int
		wantSymphony int
		wantStatus   int
	}{
		{
			name:       "Fingerprint Zerodha Kite payload",
			body:       `{"user_id":"AB1234","order_id":"220303000000001","checksum":"abcdef1234567890","status":"COMPLETE"}`,
			wantKite:   1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Fingerprint Angel One payload",
			body:       `{"uniqueorderid":"ANGEL-101","orderstatus":"complete","filledshares":50,"tradingsymbol":"INFY"}`,
			wantAngel:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Fingerprint Groww payload",
			body:       `{"groww_order_id":"GROWW-8899","order_status":"EXECUTED","trading_symbol":"TCS"}`,
			wantGroww:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Fingerprint Dhan payload",
			body:       `{"dhanClientId":"1000000001","orderId":"DHAN-1234","orderStatus":"TRADED","tradingSymbol":"RELIANCE","filledQty":25}`,
			wantDhan:   1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Fingerprint Kotak Neo payload",
			body:       `{"nOrdNo":"2410240001","trdSym":"TCS","ordSt":"complete","fldQty":10,"avgPrc":"3500.50"}`,
			wantKotak:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:         "Fingerprint Symphony payload",
			body:         `{"LoginID":"SYM001","ClientID":"CLI001","AppOrderID":"APP123","ExchangeOrderID":"EXCH456","ExchangeInstrumentID":2885,"OrderStatus":"Completed"}`,
			wantSymphony: 1,
			wantStatus:   http.StatusOK,
		},
		{
			name:         "Header X-Broker overrides payload fingerprint",
			body:         `{"user_id":"AB1234","checksum":"abcdef1234567890"}`, // looks like Kite
			headerBroker: "angelone",
			wantAngel:    1,
			wantStatus:   http.StatusOK,
		},
		{
			name:       "Query param ?broker= overrides payload fingerprint",
			body:       `{"groww_order_id":"GROWW-8899"}`, // looks like Groww
			query:      "?broker=dhan",
			wantDhan:   1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Query param ?broker=kotak aliases to kotakneo handler",
			body:       `{"status":"COMPLETE"}`,
			query:      "?broker=kotak",
			wantKotak:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:         "Path param /broker-callback/symphony overrides payload",
			body:         `{"status":"COMPLETE"}`,
			path:         "/broker-callback/symphony",
			wantSymphony: 1,
			wantStatus:   http.StatusOK,
		},
		{
			name:       "Path param /broker-callback/groww overrides payload",
			body:       `{"status":"COMPLETE"}`,
			path:       "/broker-callback/groww",
			wantGroww:  1,
			wantStatus: http.StatusOK,
		},
		{
			name:       "Unrecognized payload falls back to default handler (Kite)",
			body:       `{"status":"COMPLETE","order_id":"GENERIC-1"}`,
			wantKite:   1,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kiteBefore := kiteH.getCalled()
			angelBefore := angelH.getCalled()
			growwBefore := growwH.getCalled()
			dhanBefore := dhanH.getCalled()
			kotakBefore := kotakH.getCalled()
			symphonyBefore := symphonyH.getCalled()

			reqPath := "/broker-callback"
			if tc.path != "" {
				reqPath = tc.path
			}
			if tc.query != "" {
				reqPath += tc.query
			}

			req := httptest.NewRequest(http.MethodPost, reqPath, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.headerBroker != "" {
				req.Header.Set("X-Broker", tc.headerBroker)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			kiteDelta := kiteH.getCalled() - kiteBefore
			angelDelta := angelH.getCalled() - angelBefore
			growwDelta := growwH.getCalled() - growwBefore
			dhanDelta := dhanH.getCalled() - dhanBefore
			kotakDelta := kotakH.getCalled() - kotakBefore
			symphonyDelta := symphonyH.getCalled() - symphonyBefore

			if kiteDelta != tc.wantKite {
				t.Errorf("kite called delta = %d, want %d", kiteDelta, tc.wantKite)
			}
			if angelDelta != tc.wantAngel {
				t.Errorf("angel called delta = %d, want %d", angelDelta, tc.wantAngel)
			}
			if growwDelta != tc.wantGroww {
				t.Errorf("groww called delta = %d, want %d", growwDelta, tc.wantGroww)
			}
			if dhanDelta != tc.wantDhan {
				t.Errorf("dhan called delta = %d, want %d", dhanDelta, tc.wantDhan)
			}
			if kotakDelta != tc.wantKotak {
				t.Errorf("kotak called delta = %d, want %d", kotakDelta, tc.wantKotak)
			}
			if symphonyDelta != tc.wantSymphony {
				t.Errorf("symphony called delta = %d, want %d", symphonyDelta, tc.wantSymphony)
			}
		})
	}
}

func TestAdaptiveRouter_PreservesRequestBody(t *testing.T) {
	kiteH := &recordHandler{}
	router := callback.NewAdaptiveRouter()
	router.Register("kite", kiteH)
	router.SetDefault(kiteH)

	payload := `{"user_id":"AB1234","checksum":"abc12345","status":"COMPLETE"}`
	req := httptest.NewRequest(http.MethodPost, "/broker-callback", bytes.NewBufferString(payload))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if kiteH.getCalled() != 1 {
		t.Fatalf("handler not called")
	}
	if string(kiteH.bodies[0]) != payload {
		t.Errorf("body received = %q, want %q", string(kiteH.bodies[0]), payload)
	}
}

func TestAdaptiveRouter_NegativeAndEdgeCases(t *testing.T) {
	t.Run("Empty body falls back to default and returns 200", func(t *testing.T) {
		kiteH := &recordHandler{}
		router := callback.NewAdaptiveRouter()
		router.SetDefault(kiteH)

		req := httptest.NewRequest(http.MethodPost, "/broker-callback", bytes.NewBufferString(""))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("empty body status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("Unknown broker specified explicitly falls back to 200 drop", func(t *testing.T) {
		kiteH := &recordHandler{}
		router := callback.NewAdaptiveRouter()
		router.SetDefault(kiteH)

		req := httptest.NewRequest(http.MethodPost, "/broker-callback?broker=unknown_broker", bytes.NewBufferString(`{"foo":"bar"}`))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("unknown broker status = %d, want %d", rec.Code, http.StatusOK)
		}
		// Since unknown broker was specified and no handler exists, it drops cleanly without invoking default
		if kiteH.getCalled() != 0 {
			t.Errorf("default handler should not have been called for explicit unknown broker")
		}
	})

	t.Run("Oversized payload rejected with 400", func(t *testing.T) {
		router := callback.NewAdaptiveRouter()
		// Limit to 64KB
		router.MaxBodyBytes = 64 * 1024

		hugeBody := strings.Repeat("A", 128*1024)
		req := httptest.NewRequest(http.MethodPost, "/broker-callback", bytes.NewBufferString(hugeBody))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("oversized body status = %d, want 400", rec.Code)
		}
	})
}

func BenchmarkAdaptiveRouter_Fingerprint(b *testing.B) {
	kiteH := &recordHandler{}
	angelH := &recordHandler{}
	growwH := &recordHandler{}

	router := callback.NewAdaptiveRouter()
	router.Register("kite", kiteH)
	router.Register("angelone", angelH)
	router.Register("groww", growwH)
	router.SetDefault(kiteH)

	body := []byte(`{"user_id":"AB1234","order_id":"220303000000001","checksum":"abcdef1234567890","status":"COMPLETE"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/broker-callback", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}
}
