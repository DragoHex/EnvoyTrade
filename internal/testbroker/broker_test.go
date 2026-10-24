package testbroker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"envoytrade/internal/broker"
)

func TestBroker_PlaceOrder_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders/regular" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token key1:tok1" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		_ = r.ParseForm()
		if r.FormValue("tradingsymbol") != "NIFTY26OCTFUT" || r.FormValue("quantity") != "75" {
			t.Errorf("unexpected form: %v", r.Form)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"order_id":"TB_ORD_999"}}`))
	}))
	defer server.Close()

	mockProxy := &ProxyConfig{Host: "mock-proxy.local", Port: 443}
	b := NewLiveBroker("key1", "tok1", server.URL, mockProxy)

	resp, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if resp.OrderID != "TB_ORD_999" {
		t.Errorf("expected orderID TB_ORD_999, got %s", resp.OrderID)
	}
}

func TestBroker_PlaceOrder_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error","error_type":"InputException","message":"invalid lot size"}`))
	}))
	defer server.Close()

	mockProxy := &ProxyConfig{Host: "mock-proxy.local", Port: 443}
	b := NewLiveBroker("key1", "tok1", server.URL, mockProxy)

	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Quantity: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBroker_Unproxied_BlocksPlaceAndCancel(t *testing.T) {
	b := NewLiveBroker("key1", "tok1", "http://localhost:8089", nil)

	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{Quantity: 10})
	if err == nil {
		t.Fatal("PlaceOrder unproxied expected error, got nil")
	}

	_, err = b.CancelOrder(context.Background(), "regular", "TB-123")
	if err == nil {
		t.Fatal("CancelOrder unproxied expected error, got nil")
	}
}

func TestBroker_DefaultURL(t *testing.T) {
	t.Run("Default Localhost", func(t *testing.T) {
		os.Unsetenv("TESTBROKER_URL")
		b := NewLiveBroker("k", "t", "", nil)
		if b.BaseURI() != "http://localhost:8089" {
			t.Errorf("BaseURI = %s, want http://localhost:8089", b.BaseURI())
		}
	})

	t.Run("Env Override", func(t *testing.T) {
		t.Setenv("TESTBROKER_URL", "http://test-server:9000")
		b := NewLiveBroker("k", "t", "", nil)
		if b.BaseURI() != "http://test-server:9000" {
			t.Errorf("BaseURI = %s, want http://test-server:9000", b.BaseURI())
		}
	})
}

func TestBroker_GetPositions_MTMCalculation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/portfolio/positions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"net": [
					{
						"tradingsymbol": "NIFTY26OCTFUT",
						"exchange": "NFO",
						"product": "NRML",
						"quantity": 150,
						"average_price": 25000.0,
						"last_price": 25150.0,
						"pnl": 22500.0,
						"m2m": 22500.0
					},
					{
						"tradingsymbol": "RELIANCE",
						"exchange": "NSE",
						"product": "MIS",
						"quantity": -10,
						"average_price": 2450.0,
						"last_price": 2420.0,
						"pnl": 300.0,
						"m2m": 300.0
					}
				]
			}
		}`))
	}))
	defer server.Close()

	b := NewLiveBroker("key1", "tok1", server.URL, nil)
	positions, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}
	var totalMtm float64
	for _, p := range positions {
		totalMtm += p.M2M
	}
	expectedTotal := 22800.0
	if totalMtm != expectedTotal {
		t.Errorf("totalMtm = %f, want %f", totalMtm, expectedTotal)
	}
}

func TestBroker_GetPositions_OvernightZeroM2M(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": {
				"net": [
					{
						"tradingsymbol": "CRUDEOIL26OCT10200CE",
						"exchange": "MCX",
						"product": "NRML",
						"quantity": -2,
						"average_price": 72.65,
						"last_price": 20.40,
						"pnl": 10450.0,
						"m2m": 0.0
					}
				]
			}
		}`))
	}))
	defer server.Close()

	b := NewLiveBroker("key1", "tok1", server.URL, nil)
	positions, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].M2M != 10450.0 {
		t.Errorf("expected M2M = 10450.0 (from PnL), got %f", positions[0].M2M)
	}
	if positions[0].PnL != 10450.0 {
		t.Errorf("expected PnL = 10450.0, got %f", positions[0].PnL)
	}
}

func TestBroker_GetOrders_And_History(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"status": "success",
			"data": [
				{
					"order_id": "TB_ORD_1",
					"tradingsymbol": "INFY",
					"exchange": "NSE",
					"transaction_type": "BUY",
					"product": "CNC",
					"order_type": "LIMIT",
					"status": "COMPLETE",
					"quantity": 10,
					"filled_quantity": 10,
					"price": 1400.0,
					"average_price": 1400.0,
					"order_timestamp": "2026-10-24 10:00:00"
				},
				{
					"order_id": "TB_ORD_2",
					"tradingsymbol": "TCS",
					"exchange": "NSE",
					"transaction_type": "SELL",
					"product": "MIS",
					"order_type": "MARKET",
					"status": "OPEN",
					"quantity": 5,
					"filled_quantity": 0,
					"price": 0.0,
					"average_price": 0.0,
					"order_timestamp": "2026-10-24 10:05:00"
				}
			]
		}`))
	}))
	defer server.Close()

	b := NewLiveBroker("key1", "tok1", server.URL, nil)

	// Test GetOrders
	orders, err := b.GetOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}
	if orders[0].OrderID != "TB_ORD_1" || orders[0].Status != "COMPLETE" {
		t.Errorf("orders[0] mismatch: %+v", orders[0])
	}

	// Test GetOrderHistory
	hist, err := b.GetOrderHistory(context.Background(), "TB_ORD_1")
	if err != nil {
		t.Fatalf("GetOrderHistory failed: %v", err)
	}
	if len(hist) != 1 || hist[0].OrderID != "TB_ORD_1" {
		t.Fatalf("expected 1 history item for TB_ORD_1, got %+v", hist)
	}

	// Test GetOpenOrders
	openOrders, err := b.GetOpenOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}
	if len(openOrders) != 1 || openOrders[0].OrderID != "TB_ORD_2" {
		t.Fatalf("expected 1 open order (TB_ORD_2), got %+v", openOrders)
	}
}

