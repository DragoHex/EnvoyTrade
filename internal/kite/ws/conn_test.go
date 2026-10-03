package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/kite/ws"

	"github.com/gorilla/websocket"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestConn_ConnectsAndReceivesOrderUpdate(t *testing.T) {
	var (
		receivedAPIKey      string
		receivedAccessToken string
		connectedCalled     bool
		connectedMu         sync.Mutex
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.URL.Query().Get("api_key")
		receivedAccessToken = r.URL.Query().Get("access_token")

		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer c.Close()

		// Send an order update JSON frame matching Kite WebSocket protocol
		orderPayload := map[string]any{
			"type": "order",
			"data": map[string]any{
				"order_id":        "TEST-WS-ORDER-1",
				"status":          "COMPLETE",
				"tradingsymbol":   "INFY",
				"exchange":        "NSE",
				"filled_quantity": 100,
			},
		}
		data, _ := json.Marshal(orderPayload)
		_ = c.WriteMessage(websocket.TextMessage, data)

		// Keep connection alive briefly
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn := ws.NewConn(ws.Config{
		APIKey:      "my-api-key",
		AccessToken: "my-access-token",
		RootURL:     wsURL,
	}, nil)

	conn.OnConnect(func() {
		connectedMu.Lock()
		connectedCalled = true
		connectedMu.Unlock()
	})

	orderCh := make(chan kiteconnect.Order, 1)
	conn.OnOrderUpdate(func(o kiteconnect.Order) {
		orderCh <- o
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go conn.ServeWithContext(ctx)

	select {
	case order := <-orderCh:
		if order.OrderID != "TEST-WS-ORDER-1" {
			t.Errorf("OrderID = %q, want 'TEST-WS-ORDER-1'", order.OrderID)
		}
		if order.TradingSymbol != "INFY" {
			t.Errorf("TradingSymbol = %q, want 'INFY'", order.TradingSymbol)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for order update")
	}

	connectedMu.Lock()
	if !connectedCalled {
		t.Error("expected OnConnect to be called")
	}
	connectedMu.Unlock()

	if receivedAPIKey != "my-api-key" {
		t.Errorf("received APIKey = %q, want 'my-api-key'", receivedAPIKey)
	}
	if receivedAccessToken != "my-access-token" {
		t.Errorf("received AccessToken = %q, want 'my-access-token'", receivedAccessToken)
	}

	if err := conn.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestConn_CloseTerminatesServe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn := ws.NewConn(ws.Config{
		APIKey:      "k",
		AccessToken: "t",
		RootURL:     wsURL,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		conn.ServeWithContext(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	select {
	case <-done:
		// Success: ServeWithContext returned after Close()
	case <-time.After(2 * time.Second):
		t.Fatal("ServeWithContext did not terminate within timeout after Close()")
	}
}
