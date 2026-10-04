package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestE2E_ClientTickerWorkflow tests combined usage of Client and Ticker in an integrated trading scenario.
func TestE2E_ClientTickerWorkflow(t *testing.T) {
	ordersDB := make(map[string]Order)
	wsClients := make(map[*websocket.Conn]struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket upgrade endpoint
		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			q := r.URL.Query()
			if q.Get("api_key") != "key_master" || q.Get("access_token") != "tok_master" {
				http.Error(w, "unauthorized", http.StatusForbidden)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			wsClients[conn] = struct{}{}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					delete(wsClients, conn)
					return
				}
			}
		}

		// REST endpoints
		auth := r.Header.Get("Authorization")
		if auth != "token key_master:tok_master" && !strings.HasPrefix(r.URL.Path, "/api") {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/orders/regular":
			_ = r.ParseForm()
			orderID := "260905000000099"
			now := "2026-09-05 10:30:00"
			o := Order{
				OrderID:         orderID,
				UserID:          "MASTER01",
				Status:          OrderStatusComplete,
				Tradingsymbol:   r.FormValue("tradingsymbol"),
				Exchange:        r.FormValue("exchange"),
				TransactionType: r.FormValue("transaction_type"),
				Product:         r.FormValue("product"),
				OrderType:       r.FormValue("order_type"),
				Quantity:        75,
				AveragePrice:    25000.0,
				OrderTimestamp:  now,
			}
			ordersDB[orderID] = o

			// Broadcast to WS clients
			msg, _ := json.Marshal(map[string]any{
				"type": "order",
				"data": o,
			})
			for c := range wsClients {
				_ = c.WriteMessage(websocket.TextMessage, msg)
			}

			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   map[string]string{"order_id": orderID},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/orders":
			var list []Order
			for _, o := range ordersDB {
				list = append(list, o)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data":   list,
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/quote/ltp"):
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"NFO:NIFTY26OCTFUT": map[string]any{
						"instrument_token": 408065,
						"last_price":       25150.0,
					},
				},
			})

		default:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": nil})
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	// 1. Initialize Client & Ticker
	client := New("key_master")
	client.SetAccessToken("tok_master")
	client.SetBaseURI(ts.URL)

	ticker := NewTicker("key_master", "tok_master")
	ticker.SetRootURI(wsURL)

	connected := make(chan struct{}, 1)
	orderReceived := make(chan Order, 1)

	ticker.OnConnect(func() {
		connected <- struct{}{}
	})
	ticker.OnOrderUpdate(func(o Order) {
		orderReceived <- o
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ticker.ServeWithContext(ctx)

	// Wait for WebSocket connection
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ticker connection")
	}

	// 2. Fetch LTP
	quotes, err := client.GetLTP("NFO:NIFTY26OCTFUT")
	if err != nil {
		t.Fatalf("GetLTP failed: %v", err)
	}
	if quotes["NFO:NIFTY26OCTFUT"].LastPrice != 25150.0 {
		t.Errorf("expected LTP 25150.0, got %f", quotes["NFO:NIFTY26OCTFUT"].LastPrice)
	}

	// 3. Place order via client
	resp, err := client.PlaceOrder(VarietyRegular, OrderParams{
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
	if resp.OrderID != "260905000000099" {
		t.Errorf("expected order_id 260905000000099, got %s", resp.OrderID)
	}

	// 4. Verify order streamed to Ticker
	select {
	case o := <-orderReceived:
		if o.OrderID != resp.OrderID || o.Status != OrderStatusComplete {
			t.Errorf("unexpected streamed order: %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for streamed order on Ticker")
	}

	// 5. Query order book via client
	orders, err := client.GetOrders()
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}
	if len(orders) != 1 || orders[0].OrderID != resp.OrderID {
		t.Errorf("unexpected orders in book: %+v", orders)
	}

	// 6. Graceful close
	_ = ticker.Close()
}
