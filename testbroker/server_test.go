package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServer_E2E_Flow(t *testing.T) {
	// 1. Setup mock receiver for postbacks (simulating EnvoyTrade /broker-callback)
	postbackReceived := make(chan string, 5)
	postbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if id, ok := body["order_id"].(string); ok {
			postbackReceived <- id
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer postbackServer.Close()

	// 2. Configure testbroker
	cfg := &Config{
		Port:          0, // random free port
		PostbackURL:   postbackServer.URL,
		ExecutionMode: "instant",
		Users: map[string]UserConfig{
			"MASTER01": {APIKey: "k_m", APISecret: "s_m", AccessToken: "t_m", Role: "master"},
		},
		Instruments: []Instrument{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", InstrumentToken: 408065, LotSize: 75, TickSize: 0.05, LTP: 25000.0},
		},
		Validation: ValidationConfig{
			LotSizeCheck:        true,
			MaxQuantityPerOrder: 1800,
			CircuitLimitPct:     10.0,
			AllowedExchanges:    []string{"NFO"},
			AllowedProducts:     []string{"NRML"},
			AllowedOrderTypes:   []string{"MARKET"},
		},
		tokenIndex: map[string]string{
			"k_m:t_m": "MASTER01",
		},
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	cfg.Port = port

	go func() {
		_ = srv.httpServer.Serve(listener)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	time.Sleep(50 * time.Millisecond) // wait for server to start

	// 3. Connect WebSocket client (simulating EnvoyTrade MasterTicker)
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/?api_key=k_m&access_token=t_m", port)
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial ws: %v", err)
	}
	defer wsConn.Close()

	wsMsgChan := make(chan string, 5)
	go func() {
		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			var parsed struct {
				Type string `json:"type"`
				Data struct {
					OrderID string `json:"order_id"`
					Status  string `json:"status"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg, &parsed); err == nil && parsed.Type == "order" {
				wsMsgChan <- parsed.Data.OrderID + ":" + parsed.Data.Status
			}
		}
	}()

	// 4. Place an order via Kite REST API (simulating trader or EnvoyTrade placement)
	formData := url.Values{
		"exchange":         {"NFO"},
		"tradingsymbol":    {"NIFTY26OCTFUT"},
		"transaction_type": {"BUY"},
		"product":          {"NRML"},
		"order_type":       {"MARKET"},
		"quantity":         {"75"},
	}
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/orders/regular", port), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "token k_m:t_m")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PlaceOrder HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var placeResp struct {
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&placeResp)
	orderID := placeResp.Data.OrderID

	// 5. Verify postback received
	select {
	case pbOrderID := <-postbackReceived:
		if pbOrderID != orderID {
			t.Errorf("postback orderID = %s, want %s", pbOrderID, orderID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for postback")
	}

	// 6. Verify WebSocket received
	select {
	case wsData := <-wsMsgChan:
		expected := orderID + ":COMPLETE"
		if wsData != expected {
			t.Errorf("ws received %s, want %s", wsData, expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ws message")
	}
}
