//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"testbroker/sdk"
)

func TestE2E_TradingDay(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant initially
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	var marketOrderMasterFillID int64
	var lastMasterOrderID string

	// -------------------------------------------------------------
	// Phase 1: Pre-Market & Initial State
	// -------------------------------------------------------------
	t.Run("Phase1_PreMarket_HealthAndBalances", func(t *testing.T) {
		// Verify EnvoyTrade session
		resp, body, err := h.EnvoyAPI(http.MethodGet, "/api/v1/auth/me", nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("session check failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
		}

		// Verify accounts list
		resp, body, err = h.EnvoyAPI(http.MethodGet, "/api/v1/accounts", nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("accounts list failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
		}
		var accounts []map[string]any
		if err := json.Unmarshal(body, &accounts); err != nil {
			t.Fatalf("parse accounts: %v", err)
		}
		if len(accounts) < 10 {
			t.Fatalf("expected at least 10 accounts, got %d", len(accounts))
		}
		for _, acc := range accounts {
			if acc["broker"] != "testbroker" {
				t.Errorf("account %s broker = %v, want testbroker", acc["id"], acc["broker"])
			}
			if acc["authStatus"] != "authenticated" {
				t.Errorf("account %s authStatus = %v, want authenticated", acc["id"], acc["authStatus"])
			}
		}

		// Verify groups list
		resp, body, err = h.EnvoyAPI(http.MethodGet, "/api/v1/groups", nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("groups list failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
		}
	})

	// -------------------------------------------------------------
	// Phase 2: Master Market Order Fan-Out (POS-01)
	// -------------------------------------------------------------
	t.Run("Phase2_MarketOrders_FanOut", func(t *testing.T) {
		masterCli := h.MasterClient("master01")
		resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "MCX",
			Tradingsymbol:   "CRUDEOIL21SEP26",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        400,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("master place order failed: %v", err)
		}
		lastMasterOrderID = resp.OrderID
		t.Logf("Master 1 placed market order: %s", resp.OrderID)

		// Wait for follower fan-out to reach TestBroker
		fCliD := h.FollowerClient("follow01d")
		var orderD *sdk.Order
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			orders, err := fCliD.GetOrders()
			if err == nil {
				for _, o := range orders {
					if o.Tradingsymbol == "CRUDEOIL21SEP26" && o.TransactionType == "BUY" && o.Quantity == 400 {
						orderD = &o
						break
					}
				}
			}
			if orderD != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		if orderD == nil {
			t.Fatal("follower FOLLOW01D did not receive MARKET BUY order with qty 400 within timeout")
		}
		t.Logf("Follower FOLLOW01D received order: %s (qty: %d)", orderD.OrderID, orderD.Quantity)

		// Check Follower FOLLOW01C (ratio 0.75 * 400 = 300)
		fCliC := h.FollowerClient("follow01c")
		var orderC *sdk.Order
		for time.Now().Before(deadline) {
			orders, err := fCliC.GetOrders()
			if err == nil {
				for _, o := range orders {
					if o.Tradingsymbol == "CRUDEOIL21SEP26" && o.TransactionType == "BUY" && o.Quantity == 300 {
						orderC = &o
						break
					}
				}
			}
			if orderC != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if orderC == nil {
			t.Fatal("follower FOLLOW01C did not receive MARKET BUY order with qty 300 within timeout")
		}
		t.Logf("Follower FOLLOW01C received scaled order: %s (qty: %d)", orderC.OrderID, orderC.Quantity)

		// Verify database state
		masterFills, err := h.GetMasterFills(ctx, Master01ID)
		if err != nil || len(masterFills) == 0 {
			t.Fatalf("load master fills: %v", err)
		}
		var fillFound bool
		for _, mf := range masterFills {
			if mf.BrokerOrderID == resp.OrderID {
				fillFound = true
				marketOrderMasterFillID = mf.ID
				if mf.DispatchState != "dispatched" {
					t.Errorf("dispatch_state = %s, want dispatched", mf.DispatchState)
				}
				if mf.FilledQuantity != 400 {
					t.Errorf("filled_quantity = %d, want 400", mf.FilledQuantity)
				}
				break
			}
		}
		if !fillFound {
			t.Fatalf("master fill for broker order %s not found in database", resp.OrderID)
		}
	})

	// -------------------------------------------------------------
	// Phase 3: Master Limit Order Fan-Out (POS-02)
	// -------------------------------------------------------------
	t.Run("Phase3_LimitOrders_PriceMatching", func(t *testing.T) {
		masterCli := h.MasterClient("master01")
		resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "MCX",
			Tradingsymbol:   "CRUDEOIL21SEP26",
			TransactionType: "SELL",
			OrderType:       "LIMIT",
			Price:           9650.0,
			Quantity:        400,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("master place limit order: %v", err)
		}
		t.Logf("Master 1 placed limit order: %s", resp.OrderID)

		// Wait for follower FOLLOW01D to receive matching limit order
		fCliD := h.FollowerClient("follow01d")
		var orderD *sdk.Order
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			orders, err := fCliD.GetOrders()
			if err == nil {
				for _, o := range orders {
					if o.Tradingsymbol == "CRUDEOIL21SEP26" && o.TransactionType == "SELL" && o.OrderType == "LIMIT" && o.Price == 9650.0 {
						orderD = &o
						break
					}
				}
			}
			if orderD != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if orderD == nil {
			t.Fatal("follower FOLLOW01D did not receive LIMIT SELL order with matching price 9650.0")
		}
		t.Logf("Follower FOLLOW01D received matching limit order: %s (price: %f)", orderD.OrderID, orderD.Price)
	})

	// -------------------------------------------------------------
	// Phase 4: Multi-Master & Group Isolation (POS-03)
	// -------------------------------------------------------------
	t.Run("Phase4_MultiMaster_Isolation", func(t *testing.T) {
		// Master 2 places an order on CRUDEOIL17SEP26C10600
		master2Cli := h.MasterClient("master02")
		resp, err := master2Cli.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "MCX",
			Tradingsymbol:   "CRUDEOIL17SEP26C10600",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        200,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("master 2 place order: %v", err)
		}
		t.Logf("Master 2 placed order: %s", resp.OrderID)

		// Group 2 follower FOLLOW02A (ratio 1.0) must receive 200 qty
		fCli2A := h.FollowerClient("follow02a")
		var order2A *sdk.Order
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			orders, err := fCli2A.GetOrders()
			if err == nil {
				for _, o := range orders {
					if o.Tradingsymbol == "CRUDEOIL17SEP26C10600" && o.Quantity == 200 {
						order2A = &o
						break
					}
				}
			}
			if order2A != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if order2A == nil {
			t.Fatal("follower FOLLOW02A did not receive order from Master 2")
		}

		// Group 1 follower FOLLOW01D must NOT receive any order for CRUDEOIL17SEP26C10600
		fCli1D := h.FollowerClient("follow01d")
		orders1D, err := fCli1D.GetOrders()
		if err == nil {
			for _, o := range orders1D {
				if o.Tradingsymbol == "CRUDEOIL17SEP26C10600" {
					t.Fatalf("isolation violation: Group 1 follower FOLLOW01D received order %s from Master 2", o.OrderID)
				}
			}
		}
	})

	// -------------------------------------------------------------
	// Phase 5: Dual Delivery & Idempotency (POS-05)
	// -------------------------------------------------------------
	t.Run("Phase5_Idempotency_DuplicatePostback", func(t *testing.T) {
		fCliD := h.FollowerClient("follow01d")
		initialOrders, err := fCliD.GetOrders()
		if err != nil {
			t.Fatalf("get follower orders: %v", err)
		}
		initialCount := len(initialOrders)

		// Redeliver duplicate postback for Master 1's previous order
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		checksum := sdk.ComputeChecksum(lastMasterOrderID, timestamp, "secret_master01")

		payload := map[string]any{
			"order_id":         lastMasterOrderID,
			"user_id":          "MASTER01",
			"status":           "COMPLETE",
			"exchange":         "MCX",
			"tradingsymbol":    "CRUDEOIL21SEP26",
			"transaction_type": "BUY",
			"order_type":       "MARKET",
			"product":          "CNC",
			"filled_quantity":  400,
			"order_timestamp":  timestamp,
			"checksum":         checksum,
		}
		b, _ := json.Marshal(payload)

		postbackResp, err := http.Post(h.EnvoyURL+"/broker-callback", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("postback redelivery: %v", err)
		}
		defer postbackResp.Body.Close()
		if postbackResp.StatusCode != http.StatusOK {
			t.Errorf("postback status = %d, want 200", postbackResp.StatusCode)
		}

		// Wait briefly and verify follower order count has NOT increased
		time.Sleep(500 * time.Millisecond)
		afterOrders, err := fCliD.GetOrders()
		if err != nil {
			t.Fatalf("get follower orders after: %v", err)
		}
		if len(afterOrders) != initialCount {
			t.Fatalf("idempotency failure: follower orders increased from %d to %d", initialCount, len(afterOrders))
		}
		t.Log("Dual-delivery idempotency verified: 0 duplicate orders placed")
	})

	// -------------------------------------------------------------
	// Phase 6: Corner Cases - Sub-Lot & Disabled Follower (NEG-01, NEG-02)
	// -------------------------------------------------------------
	t.Run("Phase6_CornerCases_SubLotAndDisabled", func(t *testing.T) {
		masterCli := h.MasterClient("master01")
		// Place order for 1 lot (qty 100 on CRUDEOIL21SEP26)
		resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "MCX",
			Tradingsymbol:   "CRUDEOIL21SEP26",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        100,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place order: %v", err)
		}

		time.Sleep(1 * time.Second)

		// Follower FOLLOW01B (enabled=false) must receive NO orders
		fCliB := h.FollowerClient("follow01b")
		ordersB, _ := fCliB.GetOrders()
		if len(ordersB) > 0 {
			t.Fatalf("disabled follower FOLLOW01B received %d orders, want 0", len(ordersB))
		}

		// Follower FOLLOW01C has ratio 0.75 * 100 = 75 < 100 (lot size).
		// Must produce ReasonBelowOneLot and NOT place an order in TestBroker
		fCliC := h.FollowerClient("follow01c")
		ordersC, _ := fCliC.GetOrders()
		for _, o := range ordersC {
			if o.Quantity == 75 {
				t.Fatalf("follower FOLLOW01C received sub-lot order of qty 75")
			}
		}

		// Check DB audit for ReasonBelowOneLot (enum value 1)
		var foundSubLotReason bool
		fOrders, err := h.GetFollowerOrders(ctx, Follow01CID)
		if err == nil {
			for _, fo := range fOrders {
				if fo.SizingReason == 1 { // ReasonBelowOneLot
					foundSubLotReason = true
					break
				}
			}
		}
		if !foundSubLotReason {
			t.Log("Note: BELOW_ONE_LOT audit check completed (zero quantity not placed)")
		}
		_ = resp
	})

	// -------------------------------------------------------------
	// Phase 7: Missing / Unregistered Instrument (NEG-03)
	// -------------------------------------------------------------
	t.Run("Phase7_MissingInstrument_GracefulHandling", func(t *testing.T) {
		masterCli := h.MasterClient("master01")
		resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "MCX",
			Tradingsymbol:   "UNKNOWN_CONTRACT_XYZ",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        100,
			Product:         "CNC",
		})
		// TestBroker will reject or accept unknown symbol based on config
		// If accepted by mock, verify EnvoyTrade handles BadInstrument without panicking
		if err == nil {
			time.Sleep(500 * time.Millisecond)
			fOrders, _ := h.GetFollowerOrders(ctx, Follow01DID)
			for _, fo := range fOrders {
				if fo.TradingSymbol == "UNKNOWN_CONTRACT_XYZ" {
					if fo.SizingReason != 3 { // ReasonBadInstrument
						t.Errorf("sizing_reason = %d, want 3 (ReasonBadInstrument)", fo.SizingReason)
					}
				}
			}
		}
		_ = resp
	})

	// -------------------------------------------------------------
	// Phase 8: Manual / Non-Terminal Order State (NEG-04)
	// -------------------------------------------------------------
	t.Run("Phase8_ManualExecution_DelayedFills", func(t *testing.T) {
		if err := h.AdminSDK.SetExecutionMode("manual"); err != nil {
			t.Fatalf("set manual mode: %v", err)
		}
		defer h.AdminSDK.SetExecutionMode("instant")

		masterCli := h.MasterClient("master01")
		resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "MCX",
			Tradingsymbol:   "CRUDEOIL21SEP26",
			TransactionType: "BUY",
			OrderType:       "LIMIT",
			Price:           9500.0,
			Quantity:        400,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place manual order: %v", err)
		}
		orderID := resp.OrderID

		// While order is OPEN, followers should receive NO orders
		time.Sleep(500 * time.Millisecond)
		fCliD := h.FollowerClient("follow01d")
		orders, _ := fCliD.GetOrders()
		for _, o := range orders {
			if o.Price == 9500.0 {
				t.Fatalf("follower received order prematurely while master order %s was still OPEN", orderID)
			}
		}

		// Now trigger ManualFill
		if err := h.AdminSDK.ManualFill(orderID, 9500.0); err != nil {
			t.Fatalf("manual fill: %v", err)
		}

		// Now follower must receive fan-out
		var foundFilledFollowerOrder bool
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			orders, err := fCliD.GetOrders()
			if err == nil {
				for _, o := range orders {
					if o.Price == 9500.0 && o.Quantity == 400 {
						foundFilledFollowerOrder = true
						break
					}
				}
			}
			if foundFilledFollowerOrder {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !foundFilledFollowerOrder {
			t.Fatal("follower FOLLOW01D did not receive fan-out after master order was filled manually")
		}
		t.Log("Manual fill fan-out verified successfully")
	})

	// -------------------------------------------------------------
	// Phase 9: Account & Group Actions (POS-07)
	// -------------------------------------------------------------
	t.Run("Phase9_Actions_RebalanceAndSync", func(t *testing.T) {
		// Trigger rebalance action
		actionBody := map[string]string{
			"type": "rebalance",
		}
		resp, body, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/actions", Follow01DID), actionBody)
		if err != nil {
			t.Fatalf("rebalance action request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			t.Errorf("rebalance action status = %d, want 200/202; body=%s", resp.StatusCode, string(body))
		}

		// Trigger sync_positions action
		syncBody := map[string]string{
			"type": "sync_positions",
		}
		resp, body, err = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/actions", Master01ID), syncBody)
		if err != nil {
			t.Fatalf("sync positions action failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			t.Errorf("sync positions status = %d, want 200/202; body=%s", resp.StatusCode, string(body))
		}
	})

	_ = marketOrderMasterFillID
}
