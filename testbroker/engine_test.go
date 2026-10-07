package main

import (
	"testing"
)

func newTestEngine() *OrderEngine {
	cfg := &Config{
		ExecutionMode: "manual", // manual so orders stay OPEN for inspection
		Users: map[string]UserConfig{
			"AB1234": {APIKey: "k1", APISecret: "s1", AccessToken: "t1", Role: "master"},
			"CD5678": {APIKey: "k2", APISecret: "s2", AccessToken: "t2", Role: "follower"},
		},
		Instruments: []Instrument{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", InstrumentToken: 408065, LotSize: 75, TickSize: 0.05, LTP: 25000.0},
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", InstrumentToken: 738561, LotSize: 1, TickSize: 0.05, LTP: 2450.0},
		},
		Validation: ValidationConfig{
			LotSizeCheck:        true,
			MaxQuantityPerOrder: 1800,
			CircuitLimitPct:     10.0,
			AllowedExchanges:    []string{"NFO", "NSE"},
			AllowedProducts:     []string{"NRML", "MIS", "CNC"},
			AllowedOrderTypes:   []string{"MARKET", "LIMIT"},
		},
	}
	return NewOrderEngine(cfg)
}

func TestPlaceOrder_MarketOrder(t *testing.T) {
	eng := newTestEngine()

	resp, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if resp.OrderID == "" {
		t.Fatal("expected non-empty order_id")
	}

	// In manual mode, order should be OPEN
	orders := eng.GetOrders("AB1234")
	if len(orders) != 1 {
		t.Fatalf("len(orders) = %d, want 1", len(orders))
	}
	o := orders[0]
	if o.Status != "OPEN" {
		t.Errorf("status = %q, want OPEN", o.Status)
	}
	if o.TransactionType != "BUY" {
		t.Errorf("transaction_type = %q, want BUY", o.TransactionType)
	}
	if o.Quantity != 75 {
		t.Errorf("quantity = %d, want 75", o.Quantity)
	}
}

func TestPlaceOrder_InstantMode(t *testing.T) {
	eng := newTestEngine()
	eng.config.ExecutionMode = "instant"

	var filled []*Order
	eng.OnTerminal(func(o *Order) { filled = append(filled, o) })

	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        150,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	orders := eng.GetOrders("AB1234")
	if len(orders) != 1 {
		t.Fatalf("len(orders) = %d, want 1", len(orders))
	}
	if orders[0].Status != "COMPLETE" {
		t.Errorf("status = %q, want COMPLETE", orders[0].Status)
	}
	if orders[0].FilledQuantity != 150 {
		t.Errorf("filled_quantity = %d, want 150", orders[0].FilledQuantity)
	}
	if orders[0].AveragePrice != 25000.0 {
		t.Errorf("average_price = %f, want 25000.0", orders[0].AveragePrice)
	}
	if len(filled) != 1 {
		t.Errorf("terminal hook called %d times, want 1", len(filled))
	}
}

func TestPlaceOrder_LotSizeValidation(t *testing.T) {
	eng := newTestEngine()

	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        50, // not a multiple of 75
	})
	if err == nil {
		t.Fatal("expected lot size validation error")
	}
}

func TestPlaceOrder_FreezeQuantity(t *testing.T) {
	eng := newTestEngine()

	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        2000, // exceeds 1800
	})
	if err == nil {
		t.Fatal("expected freeze quantity error")
	}
}

func TestPlaceOrder_InvalidExchange(t *testing.T) {
	eng := newTestEngine()

	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "BSE",
		Tradingsymbol:   "RELIANCE",
		TransactionType: "BUY",
		Product:         "CNC",
		OrderType:       "MARKET",
		Quantity:        1,
	})
	if err == nil {
		t.Fatal("expected invalid exchange error")
	}
}

func TestPlaceOrder_UnknownInstrument(t *testing.T) {
	eng := newTestEngine()

	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NONEXISTENT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        75,
	})
	if err == nil {
		t.Fatal("expected unknown instrument error")
	}
}

func TestPlaceOrder_LimitPriceBand(t *testing.T) {
	eng := newTestEngine()

	// LTP is 25000, circuit is 10%, so valid range is 22500–27500
	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "LIMIT",
		Quantity:        75,
		Price:           30000.0, // way outside band
	})
	if err == nil {
		t.Fatal("expected price band error")
	}
}

func TestUserIsolation(t *testing.T) {
	eng := newTestEngine()

	eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NSE", Tradingsymbol: "RELIANCE",
		TransactionType: "BUY", Product: "CNC", OrderType: "MARKET", Quantity: 1,
	})
	eng.PlaceOrder("CD5678", "regular", OrderParams{
		Exchange: "NSE", Tradingsymbol: "RELIANCE",
		TransactionType: "SELL", Product: "CNC", OrderType: "MARKET", Quantity: 1,
	})

	if len(eng.GetOrders("AB1234")) != 1 {
		t.Error("AB1234 should have 1 order")
	}
	if len(eng.GetOrders("CD5678")) != 1 {
		t.Error("CD5678 should have 1 order")
	}
	if len(eng.GetAllOrders()) != 2 {
		t.Error("total orders should be 2")
	}
}

func TestFillOrder(t *testing.T) {
	eng := newTestEngine()

	var hooked []*Order
	eng.OnTerminal(func(o *Order) { hooked = append(hooked, o) })

	resp, _ := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})

	err := eng.FillOrder(resp.OrderID, 25100.0)
	if err != nil {
		t.Fatalf("FillOrder: %v", err)
	}

	orders := eng.GetOrders("AB1234")
	if orders[0].Status != "COMPLETE" {
		t.Errorf("status = %q, want COMPLETE", orders[0].Status)
	}
	if orders[0].AveragePrice != 25100.0 {
		t.Errorf("avg_price = %f, want 25100", orders[0].AveragePrice)
	}
	if len(hooked) != 1 {
		t.Errorf("terminal hook called %d times, want 1", len(hooked))
	}
}

func TestRejectOrder(t *testing.T) {
	eng := newTestEngine()

	resp, _ := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})

	err := eng.RejectOrder(resp.OrderID, "RMS:Rule: Margin exceeded")
	if err != nil {
		t.Fatalf("RejectOrder: %v", err)
	}

	orders := eng.GetOrders("AB1234")
	if orders[0].Status != "REJECTED" {
		t.Errorf("status = %q, want REJECTED", orders[0].Status)
	}
	if orders[0].StatusMessage != "RMS:Rule: Margin exceeded" {
		t.Errorf("status_message = %q", orders[0].StatusMessage)
	}
}

func TestCancelOrder(t *testing.T) {
	eng := newTestEngine()

	resp, _ := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})

	err := eng.CancelOrder(resp.OrderID)
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}

	orders := eng.GetOrders("AB1234")
	if orders[0].Status != "CANCELLED" {
		t.Errorf("status = %q, want CANCELLED", orders[0].Status)
	}
}

func TestFillOrder_AlreadyTerminal(t *testing.T) {
	eng := newTestEngine()

	resp, _ := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})

	eng.FillOrder(resp.OrderID, 25000)
	err := eng.FillOrder(resp.OrderID, 25000)
	if err == nil {
		t.Fatal("expected error filling already-terminal order")
	}
}

func TestGetOrderHistory(t *testing.T) {
	eng := newTestEngine()

	resp, _ := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	eng.FillOrder(resp.OrderID, 25000)

	history := eng.GetOrderHistory(resp.OrderID)
	if len(history) < 2 {
		t.Fatalf("history len = %d, want >= 2 (OPEN + COMPLETE)", len(history))
	}
	if history[0].Status != "OPEN" {
		t.Errorf("first history status = %q, want OPEN", history[0].Status)
	}
	if history[len(history)-1].Status != "COMPLETE" {
		t.Errorf("last history status = %q, want COMPLETE", history[len(history)-1].Status)
	}
}

func TestUpdateLTP(t *testing.T) {
	eng := newTestEngine()

	err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 26000.0)
	if err != nil {
		t.Fatalf("UpdateLTP: %v", err)
	}

	inst, ok := eng.GetInstrument("NFO", "NIFTY26OCTFUT")
	if !ok {
		t.Fatal("instrument not found")
	}
	if inst.LTP != 26000.0 {
		t.Errorf("LTP = %f, want 26000", inst.LTP)
	}
}

func TestGetPositions(t *testing.T) {
	eng := newTestEngine()
	eng.config.ExecutionMode = "instant"

	// 1. Buy 150 NIFTY @ 25000 (2 lots)
	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 150,
	})
	if err != nil {
		t.Fatalf("buy 150 failed: %v", err)
	}

	net, day := eng.GetPositions("AB1234")
	if len(net) != 1 || len(day) != 1 {
		t.Fatalf("expected 1 position, got net=%d day=%d", len(net), len(day))
	}
	if net[0].Quantity != 150 {
		t.Errorf("expected quantity 150, got %d", net[0].Quantity)
	}
	if net[0].BuyQuantity != 150 || net[0].SellQuantity != 0 {
		t.Errorf("expected buyQty=150 sellQty=0, got %d/%d", net[0].BuyQuantity, net[0].SellQuantity)
	}

	// 2. Sell 75 NIFTY @ 25000 (1 lot) -> partial exit
	_, err = eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "SELL", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	if err != nil {
		t.Fatalf("sell 75 failed: %v", err)
	}

	net, _ = eng.GetPositions("AB1234")
	if len(net) != 1 {
		t.Fatalf("expected 1 position, got %d", len(net))
	}
	if net[0].Quantity != 75 {
		t.Errorf("expected quantity 75, got %d", net[0].Quantity)
	}
	if net[0].BuyQuantity != 150 || net[0].SellQuantity != 75 {
		t.Errorf("expected buyQty=150 sellQty=75, got %d/%d", net[0].BuyQuantity, net[0].SellQuantity)
	}

	// 3. Sell 75 NIFTY @ 25000 (1 lot) -> position fully closed
	_, err = eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "SELL", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	if err != nil {
		t.Fatalf("sell 75 failed: %v", err)
	}

	net, _ = eng.GetPositions("AB1234")
	if len(net) != 1 {
		t.Fatalf("expected 1 position, got %d", len(net))
	}
	if net[0].Quantity != 0 {
		t.Errorf("expected quantity 0 (closed), got %d", net[0].Quantity)
	}
}

func TestGetPositions_MTMCalculation_DynamicValues(t *testing.T) {
	eng := newTestEngine()
	eng.config.ExecutionMode = "instant"

	// 1. Initial State: Buy 150 NIFTY (2 lots) @ 25000
	_, err := eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 150,
	})
	if err != nil {
		t.Fatalf("buy 150 NIFTY failed: %v", err)
	}

	net, _ := eng.GetPositions("AB1234")
	if len(net) != 1 {
		t.Fatalf("expected 1 position, got %d", len(net))
	}
	if net[0].M2M != 0.0 || net[0].Unrealised != 0.0 || net[0].Realised != 0.0 {
		t.Errorf("expected 0 MTM initially, got M2M=%f Unrealised=%f Realised=%f", net[0].M2M, net[0].Unrealised, net[0].Realised)
	}

	// 2. Value Change: LTP increases to 25100 (+100 pts)
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 25100.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}
	net, _ = eng.GetPositions("AB1234")
	expectedUnrealised := 150.0 * (25100.0 - 25000.0) // +15000
	if net[0].Unrealised != expectedUnrealised || net[0].M2M != expectedUnrealised {
		t.Errorf("LTP 25100: expected M2M/Unrealised=%f, got M2M=%f Unrealised=%f", expectedUnrealised, net[0].M2M, net[0].Unrealised)
	}

	// 3. Value Change: LTP drops to 24900 (-100 pts)
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 24900.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}
	net, _ = eng.GetPositions("AB1234")
	expectedUnrealised = 150.0 * (24900.0 - 25000.0) // -15000
	if net[0].Unrealised != expectedUnrealised || net[0].M2M != expectedUnrealised {
		t.Errorf("LTP 24900: expected M2M/Unrealised=%f, got M2M=%f Unrealised=%f", expectedUnrealised, net[0].M2M, net[0].Unrealised)
	}

	// 4. Multi-Position: Sell short 10 RELIANCE @ 2450 (Lot size = 1)
	_, err = eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NSE", Tradingsymbol: "RELIANCE",
		TransactionType: "SELL", Product: "MIS", OrderType: "MARKET", Quantity: 10,
	})
	if err != nil {
		t.Fatalf("sell 10 RELIANCE failed: %v", err)
	}
	// Drop RELIANCE LTP to 2400 (favorable for short position: +50 pts per share)
	if err := eng.UpdateLTP("NSE", "RELIANCE", 2400.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}

	net, _ = eng.GetPositions("AB1234")
	if len(net) != 2 {
		t.Fatalf("expected 2 positions for AB1234, got %d", len(net))
	}
	var totalMtmAB float64
	for _, p := range net {
		totalMtmAB += p.M2M
		if p.Tradingsymbol == "RELIANCE" {
			expectedRelM2M := 10.0 * (2450.0 - 2400.0) // +500
			if p.M2M != expectedRelM2M {
				t.Errorf("RELIANCE short: expected M2M=%f, got %f", expectedRelM2M, p.M2M)
			}
		}
	}
	// Total MTM for AB1234 = NIFTY (-15000) + RELIANCE (+500) = -14500
	if totalMtmAB != -14500.0 {
		t.Errorf("Total MTM AB1234 expected -14500.0, got %f", totalMtmAB)
	}

	// 5. Partial Close: Move NIFTY LTP to 25200 and sell 75 lots (1 lot partial exit with realized profit)
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 25200.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}
	_, err = eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "SELL", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	if err != nil {
		t.Fatalf("sell 75 NIFTY failed: %v", err)
	}

	// Move NIFTY LTP to 25300
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 25300.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}
	net, _ = eng.GetPositions("AB1234")
	for _, p := range net {
		if p.Tradingsymbol == "NIFTY26OCTFUT" {
			expectedRealised := 75.0 * (25200.0 - 25000.0)  // +15000
			expectedUnrealised = 75.0 * (25300.0 - 25000.0) // +22500
			expectedM2M := expectedRealised + expectedUnrealised // +37500
			if p.Realised != expectedRealised {
				t.Errorf("NIFTY realised expected %f, got %f", expectedRealised, p.Realised)
			}
			if p.Unrealised != expectedUnrealised {
				t.Errorf("NIFTY unrealised expected %f, got %f", expectedUnrealised, p.Unrealised)
			}
			if p.M2M != expectedM2M {
				t.Errorf("NIFTY total M2M expected %f, got %f", expectedM2M, p.M2M)
			}
		}
	}

	// 6. Full Close: Sell remaining 75 NIFTY @ 25100
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 25100.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}
	_, err = eng.PlaceOrder("AB1234", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "SELL", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	if err != nil {
		t.Fatalf("sell remaining 75 NIFTY failed: %v", err)
	}
	net, _ = eng.GetPositions("AB1234")
	for _, p := range net {
		if p.Tradingsymbol == "NIFTY26OCTFUT" {
			// Buy: 150 @ 25000 (buyVal = 3750000)
			// Sell: 75 @ 25200 + 75 @ 25100 (sellVal = 1890000 + 1882500 = 3772500)
			// Realised = 3772500 - 3750000 = 22500
			expectedRealised := 22500.0
			expectedUnrealised = 0.0
			expectedM2M := 22500.0
			if p.Quantity != 0 {
				t.Errorf("expected closed position (qty=0), got %d", p.Quantity)
			}
			if p.Realised != expectedRealised {
				t.Errorf("NIFTY fully closed realised expected %f, got %f", expectedRealised, p.Realised)
			}
			if p.Unrealised != expectedUnrealised {
				t.Errorf("NIFTY fully closed unrealised expected %f, got %f", expectedUnrealised, p.Unrealised)
			}
			if p.M2M != expectedM2M {
				t.Errorf("NIFTY fully closed M2M expected %f, got %f", expectedM2M, p.M2M)
			}
		}
	}

	// 7. Account Isolation: CD5678 positions are completely separate
	_, err = eng.PlaceOrder("CD5678", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 150,
	})
	if err != nil {
		t.Fatalf("buy 150 for CD5678 failed: %v", err)
	}
	// CD5678 bought @ 25100. Move LTP to 25400 (+300 pts)
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 25400.0); err != nil {
		t.Fatalf("UpdateLTP failed: %v", err)
	}
	netCD, _ := eng.GetPositions("CD5678")
	if len(netCD) != 1 {
		t.Fatalf("expected 1 position for CD5678, got %d", len(netCD))
	}
	expectedM2M_CD := 150.0 * (25400.0 - 25100.0) // +45000
	if netCD[0].M2M != expectedM2M_CD {
		t.Errorf("CD5678 M2M expected %f, got %f", expectedM2M_CD, netCD[0].M2M)
	}
}

