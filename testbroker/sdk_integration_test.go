package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"testbroker/sdk"
)

func TestSDK_IntegrationWithRealServer(t *testing.T) {
	// Configure testbroker server
	cfg := &Config{
		Port:          0, // random free port
		PostbackURL:   "http://127.0.0.1:9999/null-callback",
		ExecutionMode: "instant",
		Users: map[string]UserConfig{
			"MASTER01": {APIKey: "k_master", APISecret: "s_master", AccessToken: "t_master", Role: "master"},
			"FOLLOWER01": {APIKey: "k_follower", APISecret: "s_follower", AccessToken: "t_follower", Role: "follower"},
		},
		Instruments: []Instrument{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", InstrumentToken: 408065, LotSize: 75, TickSize: 0.05, LTP: 25000.0},
		},
		Validation: ValidationConfig{
			LotSizeCheck:        true,
			MaxQuantityPerOrder: 1800,
			CircuitLimitPct:     10.0,
			AllowedExchanges:    []string{"NFO"},
			AllowedProducts:     []string{"NRML", "MIS"},
			AllowedOrderTypes:   []string{"MARKET", "LIMIT"},
		},
		tokenIndex: map[string]string{
			"k_master:t_master":     "MASTER01",
			"k_follower:t_follower": "FOLLOWER01",
		},
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
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

	serverHTTPURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	serverWSURL := fmt.Sprintf("ws://127.0.0.1:%d", port)

	time.Sleep(50 * time.Millisecond)

	// 1. Initialize SDK Client for Master
	client := sdk.New("k_master")
	client.SetAccessToken("t_master")
	client.SetBaseURI(serverHTTPURL)

	// 2. Initialize SDK Ticker for Master
	ticker := sdk.NewTicker("k_master", "t_master")
	ticker.SetRootURI(serverWSURL)

	connected := make(chan struct{}, 1)
	orderStream := make(chan sdk.Order, 5)

	ticker.OnConnect(func() {
		connected <- struct{}{}
	})
	ticker.OnOrderUpdate(func(o sdk.Order) {
		orderStream <- o
	})

	tickerCtx, cancelTicker := context.WithCancel(context.Background())
	defer cancelTicker()

	go ticker.ServeWithContext(tickerCtx)

	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SDK ticker to connect to real server")
	}

	// 3. Test Admin API: change mode to manual, place order, manually fill
	if err := client.SetExecutionMode("manual"); err != nil {
		t.Fatalf("SetExecutionMode failed: %v", err)
	}

	orderResp, err := client.PlaceOrder(sdk.VarietyRegular, sdk.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: sdk.TransactionTypeBuy,
		Product:         sdk.ProductNRML,
		OrderType:       sdk.OrderTypeMarket,
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	orderID := orderResp.OrderID
	if orderID == "" {
		t.Fatal("expected non-empty orderID")
	}

	// In manual mode, order should be OPEN in order book
	orders, err := client.GetOrders()
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}
	if len(orders) != 1 || orders[0].Status != sdk.OrderStatusOpen {
		t.Fatalf("expected 1 OPEN order, got: %+v", orders)
	}

	// 4. Manually fill via Admin API
	if err := client.ManualFill(orderID, 25050.0); err != nil {
		t.Fatalf("ManualFill failed: %v", err)
	}

	// 5. Verify Ticker received COMPLETE order update
	select {
	case o := <-orderStream:
		if o.OrderID != orderID || o.Status != sdk.OrderStatusComplete || o.AveragePrice != 25050.0 {
			t.Errorf("unexpected streamed order update: %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for filled order on SDK ticker")
	}

	// 6. Verify order history
	history, err := client.GetOrderHistory(orderID)
	if err != nil {
		t.Fatalf("GetOrderHistory failed: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected >= 2 history entries, got %d", len(history))
	}
	if history[0].Status != sdk.OrderStatusOpen || history[len(history)-1].Status != sdk.OrderStatusComplete {
		t.Errorf("unexpected history: %+v", history)
	}

	// 7. Test Admin API: SetLTP and read via client.GetLTP
	if err := client.SetLTP("NFO", "NIFTY26OCTFUT", 26200.0); err != nil {
		t.Fatalf("SetLTP failed: %v", err)
	}
	quotes, err := client.GetLTP("NFO:NIFTY26OCTFUT")
	if err != nil {
		t.Fatalf("GetLTP failed: %v", err)
	}
	if quotes["NFO:NIFTY26OCTFUT"].LastPrice != 26200.0 {
		t.Errorf("LTP = %f, want 26200.0", quotes["NFO:NIFTY26OCTFUT"].LastPrice)
	}

	// 8. Test clean close
	if err := ticker.Close(); err != nil {
		t.Errorf("Ticker Close failed: %v", err)
	}
}
