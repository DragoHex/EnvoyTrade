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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestTicker_ConnectAndOrderUpdate(t *testing.T) {
	connected := make(chan struct{}, 1)
	orderReceived := make(chan Order, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query params
		q := r.URL.Query()
		if q.Get("api_key") != "key1" || q.Get("access_token") != "tok1" {
			t.Errorf("unexpected query params: %s", r.URL.RawQuery)
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// Send an order message
		msg := map[string]any{
			"type": "order",
			"data": map[string]any{
				"order_id":       "ORD_12345",
				"tradingsymbol":  "NIFTY26OCTFUT",
				"status":         "COMPLETE",
				"quantity":       75,
				"average_price":  25000.0,
			},
		}
		bytes, _ := json.Marshal(msg)
		_ = conn.WriteMessage(websocket.TextMessage, bytes)

		// Wait for close
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	ticker := NewTicker("key1", "tok1")
	ticker.SetRootURI(wsURL)

	ticker.OnConnect(func() {
		select {
		case connected <- struct{}{}:
		default:
		}
	})

	ticker.OnOrderUpdate(func(order Order) {
		orderReceived <- order
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ticker.ServeWithContext(ctx)

	// Verify onConnect
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnConnect")
	}

	// Verify onOrderUpdate
	select {
	case o := <-orderReceived:
		if o.OrderID != "ORD_12345" || o.Tradingsymbol != "NIFTY26OCTFUT" || o.Status != OrderStatusComplete {
			t.Errorf("unexpected order: %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnOrderUpdate")
	}

	// Graceful close
	if err := ticker.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestTicker_ErrorCallback(t *testing.T) {
	errReceived := make(chan string, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		msg := map[string]any{
			"type": "error",
			"data": "Rate limit exceeded",
		}
		bytes, _ := json.Marshal(msg)
		_ = conn.WriteMessage(websocket.TextMessage, bytes)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	ticker := NewTicker("k", "t")
	ticker.SetRootURI(wsURL)

	ticker.OnError(func(err error) {
		errReceived <- err.Error()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ticker.ServeWithContext(ctx)

	select {
	case msg := <-errReceived:
		if !strings.Contains(msg, "Rate limit exceeded") {
			t.Errorf("unexpected error: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnError")
	}

	_ = ticker.Close()
}

func TestTicker_CloseCallback(t *testing.T) {
	closeReceived := make(chan int, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"), time.Now().Add(time.Second))
		_ = conn.Close()
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	ticker := NewTicker("k", "t")
	ticker.SetRootURI(wsURL)

	ticker.OnClose(func(code int, reason string) {
		select {
		case closeReceived <- code:
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ticker.ServeWithContext(ctx)

	select {
	case code := <-closeReceived:
		if code != websocket.CloseNormalClosure {
			t.Errorf("expected close normal closure, got %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnClose")
	}

	_ = ticker.Close()
}
