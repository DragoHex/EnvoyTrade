package sdk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_NewAndSetters(t *testing.T) {
	c := New("test_key")
	if c.apiKey != "test_key" {
		t.Errorf("apiKey = %s, want test_key", c.apiKey)
	}

	c.SetAccessToken("test_token")
	if c.accessToken != "test_token" {
		t.Errorf("accessToken = %s, want test_token", c.accessToken)
	}

	c.SetBaseURI("http://localhost:9999")
	if c.baseURI != "http://localhost:9999" {
		t.Errorf("baseURI = %s, want http://localhost:9999", c.baseURI)
	}

	customHTTP := &http.Client{}
	c.SetHTTPClient(customHTTP)
	if c.httpClient != customHTTP {
		t.Error("httpClient setter failed")
	}
}

func TestClient_PlaceOrder_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/orders/regular" {
			t.Errorf("expected /orders/regular, got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "token test_key:test_token" {
			t.Errorf("expected token test_key:test_token, got %s", auth)
		}

		_ = r.ParseForm()
		if r.FormValue("exchange") != "NFO" || r.FormValue("tradingsymbol") != "NIFTY26OCTFUT" {
			t.Errorf("unexpected form values: %v", r.Form)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"order_id": "260905000000001"
			}
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	resp, err := c.PlaceOrder(VarietyRegular, OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: TransactionTypeBuy,
		Product:         ProductNRML,
		OrderType:       OrderTypeMarket,
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if resp.OrderID != "260905000000001" {
		t.Errorf("OrderID = %s, want 260905000000001", resp.OrderID)
	}
}

func TestClient_PlaceOrder_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"status": "error",
			"error_type": "InputException",
			"message": "quantity 50 is not a multiple of lot size 75"
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	_, err := c.PlaceOrder(VarietyRegular, OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: TransactionTypeBuy,
		Product:         ProductNRML,
		OrderType:       OrderTypeMarket,
		Quantity:        50,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	kiteErr, ok := err.(*KiteError)
	if !ok {
		t.Fatalf("expected *KiteError, got %T: %v", err, err)
	}
	if kiteErr.StatusCode != 400 || kiteErr.ErrorType != "InputException" {
		t.Errorf("unexpected KiteError: %+v", kiteErr)
	}
	if !strings.Contains(kiteErr.Message, "not a multiple of lot size") {
		t.Errorf("unexpected message: %s", kiteErr.Message)
	}
}

func TestClient_CancelOrder(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/orders/regular/260905000000001" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"order_id":"260905000000001"}}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	resp, err := c.CancelOrder(VarietyRegular, "260905000000001", nil)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}
	if resp.OrderID != "260905000000001" {
		t.Errorf("OrderID = %s, want 260905000000001", resp.OrderID)
	}
}

func TestClient_GetOrders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/orders" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": [
				{
					"order_id": "1001",
					"tradingsymbol": "NIFTY26OCTFUT",
					"status": "COMPLETE",
					"quantity": 75,
					"average_price": 25000.0
				}
			]
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	orders, err := c.GetOrders()
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(orders))
	}
	if orders[0].OrderID != "1001" || orders[0].Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("unexpected order: %+v", orders[0])
	}
}

func TestClient_GetOrderHistory(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/orders/1001" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": [
				{"order_id": "1001", "status": "OPEN"},
				{"order_id": "1001", "status": "COMPLETE"}
			]
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	history, err := c.GetOrderHistory("1001")
	if err != nil {
		t.Fatalf("GetOrderHistory failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].Status != OrderStatusOpen || history[1].Status != OrderStatusComplete {
		t.Errorf("unexpected history: %+v", history)
	}
}

func TestClient_GetPositions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/portfolio/positions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"net": [
					{"tradingsymbol": "NIFTY26OCTFUT", "quantity": 75, "pnl": 500.0}
				],
				"day": []
			}
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	pos, err := c.GetPositions()
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(pos.Net) != 1 || pos.Net[0].Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("unexpected positions: %+v", pos)
	}
}

func TestClient_GetHoldings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/portfolio/holdings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": [
				{"tradingsymbol": "RELIANCE", "quantity": 10, "last_price": 2500.0}
			]
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	holdings, err := c.GetHoldings()
	if err != nil {
		t.Fatalf("GetHoldings failed: %v", err)
	}
	if len(holdings) != 1 || holdings[0].Tradingsymbol != "RELIANCE" {
		t.Errorf("unexpected holdings: %+v", holdings)
	}
}

func TestClient_GetUserMargins(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/margins" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"equity": {
					"enabled": true,
					"net": 1000000.0,
					"available": {
						"cash": 1000000.0,
						"live_balance": 1000000.0
					},
					"utilised": {}
				},
				"commodity": {
					"enabled": false,
					"net": 0.0,
					"available": {},
					"utilised": {}
				}
			}
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	margins, err := c.GetUserMargins()
	if err != nil {
		t.Fatalf("GetUserMargins failed: %v", err)
	}
	if !margins.Equity.Enabled || margins.Equity.Available.LiveBalance != 1000000.0 {
		t.Errorf("unexpected margins: %+v", margins)
	}
}

func TestClient_GetLTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/quote/ltp" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		insts := r.URL.Query()["i"]
		if len(insts) != 2 {
			t.Errorf("expected 2 'i' params, got %d: %v", len(insts), insts)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"NFO:NIFTY26OCTFUT": {"instrument_token": 408065, "last_price": 25000.0},
				"NSE:RELIANCE": {"instrument_token": 738561, "last_price": 2450.0}
			}
		}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetAccessToken("test_token")
	c.SetBaseURI(ts.URL)

	quotes, err := c.GetLTP("NFO:NIFTY26OCTFUT", "NSE:RELIANCE")
	if err != nil {
		t.Fatalf("GetLTP failed: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(quotes))
	}
	if quotes["NFO:NIFTY26OCTFUT"].LastPrice != 25000.0 {
		t.Errorf("expected 25000.0, got %f", quotes["NFO:NIFTY26OCTFUT"].LastPrice)
	}
}

func TestClient_GenerateSession(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/session/token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		if r.FormValue("api_key") != "test_key" || r.FormValue("request_token") != "req_tok_1" {
			t.Errorf("unexpected form values: %v", r.Form)
		}
		if r.FormValue("checksum") == "" {
			t.Error("expected non-empty checksum")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"user_id":      "USER100",
				"access_token": "generated_tok_123",
			},
		})
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetBaseURI(ts.URL)

	session, err := c.GenerateSession("req_tok_1", "my_secret")
	if err != nil {
		t.Fatalf("GenerateSession failed: %v", err)
	}
	if session.UserID != "USER100" || session.AccessToken != "generated_tok_123" {
		t.Errorf("unexpected session: %+v", session)
	}
	// Verify access token was automatically set on client
	if c.accessToken != "generated_tok_123" {
		t.Errorf("client accessToken = %s, want generated_tok_123", c.accessToken)
	}
}
