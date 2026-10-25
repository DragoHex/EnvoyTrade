//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"testbroker/sdk"
)

func TestE2E_MasterStopCopyFlow(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant initially
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	// Enable Follower 01B so all 4 followers in Group 01 are active
	resp, body, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{
		"enabled": true,
	})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("enable Follower 01B failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}

	masterCli := h.MasterClient("master01")
	fCliA := h.FollowerClient("follow01a")
	fCliB := h.FollowerClient("follow01b")
	fCliC := h.FollowerClient("follow01c")
	fCliD := h.FollowerClient("follow01d")

	// Helper to count total orders across all 4 followers
	getFollowerOrderCounts := func() map[string]int {
		counts := make(map[string]int)
		for name, cli := range map[string]*sdk.Client{
			"follow01a": fCliA,
			"follow01b": fCliB,
			"follow01c": fCliC,
			"follow01d": fCliD,
		} {
			orders, err := cli.GetOrders()
			if err != nil {
				t.Fatalf("get %s orders: %v", name, err)
			}
			counts[name] = len(orders)
		}
		return counts
	}

	// -----------------------------------------------------------------
	// Phase 1: Master is Active -> Verify trades are copied to followers
	// -----------------------------------------------------------------
	t.Log("=== Phase 1: Master Active -> Copy Trade Fan-Out ===")

	// Verify master is active in DB
	var masterActive bool
	err = h.DB.QueryRow(ctx, "SELECT active FROM accounts WHERE id = $1", Master01ID).Scan(&masterActive)
	if err != nil || !masterActive {
		t.Fatalf("expected Master01 to be active in DB, got active=%v, err=%v", masterActive, err)
	}

	initialCounts := getFollowerOrderCounts()

	// Master places 400 qty order
	order1Resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        400,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order 1 failed: %v", err)
	}
	t.Logf("Master 1 placed order: %s", order1Resp.OrderID)

	// Wait for all 4 followers to receive their copy orders
	deadline := time.Now().Add(5 * time.Second)
	allReceived := false
	for time.Now().Before(deadline) {
		currentCounts := getFollowerOrderCounts()
		if currentCounts["follow01a"] > initialCounts["follow01a"] &&
			currentCounts["follow01b"] > initialCounts["follow01b"] &&
			currentCounts["follow01c"] > initialCounts["follow01c"] &&
			currentCounts["follow01d"] > initialCounts["follow01d"] {
			allReceived = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !allReceived {
		t.Fatalf("Phase 1 failed: not all followers received copy orders within timeout. Counts: %+v vs initial: %+v",
			getFollowerOrderCounts(), initialCounts)
	}

	// Validate proportional lot sizing for followers:
	// Crude Oil lot size = 100.
	// Master = 400
	// Follow01A (ratio 1.0)  = 400
	// Follow01B (ratio 0.5)  = 200
	// Follow01C (ratio 0.75) = 300
	// Follow01D (ratio 1.0)  = 400
	ordersA, _ := fCliA.GetOrders()
	ordersB, _ := fCliB.GetOrders()
	ordersC, _ := fCliC.GetOrders()
	ordersD, _ := fCliD.GetOrders()
	t.Logf("Follower quantities received: A=%d, B=%d, C=%d, D=%d",
		ordersA[len(ordersA)-1].Quantity,
		ordersB[len(ordersB)-1].Quantity,
		ordersC[len(ordersC)-1].Quantity,
		ordersD[len(ordersD)-1].Quantity,
	)

	if ordersD[len(ordersD)-1].Quantity != 400 ||
		ordersC[len(ordersC)-1].Quantity != 300 ||
		ordersB[len(ordersB)-1].Quantity != 200 ||
		ordersA[len(ordersA)-1].Quantity != 400 {
		t.Errorf("unexpected follower quantities: A=%d (want 400), B=%d (want 200), C=%d (want 300), D=%d (want 400)",
			ordersA[len(ordersA)-1].Quantity,
			ordersB[len(ordersB)-1].Quantity,
			ordersC[len(ordersC)-1].Quantity,
			ordersD[len(ordersD)-1].Quantity,
		)
	}

	// Verify master fill record in DB
	masterFills, err := h.GetMasterFills(ctx, Master01ID)
	if err != nil || len(masterFills) == 0 {
		t.Fatalf("load master fills: %v", err)
	}
	if masterFills[0].DispatchState != "dispatched" {
		t.Errorf("master fill dispatch_state = %s, want dispatched", masterFills[0].DispatchState)
	}
	t.Log("Phase 1 PASS: Master order successfully copied to all 4 followers.")

	// -----------------------------------------------------------------
	// Phase 2: Stop copy for the group (Simulate clicking "Stop Copy")
	// -----------------------------------------------------------------
	t.Log("=== Phase 2: Stop Copy for Master (active: false) ===")

	patchResp, patchBody, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Master01ID, map[string]any{
		"active": false,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch master active=false failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	var patchData map[string]any
	if err := json.Unmarshal(patchBody, &patchData); err != nil {
		t.Fatalf("parse patch response: %v", err)
	}
	if patchData["active"] != false {
		t.Fatalf("expected response active=false, got: %s", string(patchBody))
	}

	// Verify DB state
	err = h.DB.QueryRow(ctx, "SELECT active FROM accounts WHERE id = $1", Master01ID).Scan(&masterActive)
	if err != nil || masterActive {
		t.Fatalf("expected master active=false in DB, got: %v, err=%v", masterActive, err)
	}
	t.Log("Phase 2 PASS: Master copy stopped successfully (active=false in API and DB).")

	// -----------------------------------------------------------------
	// Phase 3: Master trades while stopped -> Validate copy is blocked
	// -----------------------------------------------------------------
	t.Log("=== Phase 3: Master Trades While Stopped -> Verify Copy Blocked ===")

	countsBeforeBlockedTrade := getFollowerOrderCounts()

	// Master places second order while stopped
	order2Resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        400,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order 2 failed: %v", err)
	}
	t.Logf("Master 1 placed second order while stopped: %s", order2Resp.OrderID)

	// Wait for master fill to be ingested into master_fills table
	masterFillIngested := false
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		fills, err := h.GetMasterFills(ctx, Master01ID)
		if err == nil {
			for _, f := range fills {
				if f.BrokerOrderID == order2Resp.OrderID {
					masterFillIngested = true
					break
				}
			}
		}
		if masterFillIngested {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !masterFillIngested {
		t.Fatalf("master fill for order %s was not ingested into master_fills table", order2Resp.OrderID)
	}

	// Verify that followers did NOT receive any new orders
	time.Sleep(1500 * time.Millisecond)
	countsAfterBlockedTrade := getFollowerOrderCounts()
	for name, count := range countsAfterBlockedTrade {
		if count != countsBeforeBlockedTrade[name] {
			t.Fatalf("Phase 3 failed: follower %s received order while master was stopped! Count before: %d, after: %d",
				name, countsBeforeBlockedTrade[name], count)
		}
	}

	// Verify in DB that no follower_orders rows exist for the second master fill
	var blockedMasterFillID int64
	err = h.DB.QueryRow(ctx, "SELECT id, dispatch_state FROM master_fills WHERE broker_order_id = $1", order2Resp.OrderID).Scan(&blockedMasterFillID, &masterFills[0].DispatchState)
	if err != nil {
		t.Fatalf("query blocked master fill: %v", err)
	}

	var followerOrdersCount int
	err = h.DB.QueryRow(ctx, "SELECT count(*) FROM follower_orders WHERE master_fill_id = $1", blockedMasterFillID).Scan(&followerOrdersCount)
	if err != nil {
		t.Fatalf("count follower orders: %v", err)
	}
	if followerOrdersCount != 0 {
		t.Fatalf("Phase 3 failed: expected 0 follower_orders for inactive master fill, got %d", followerOrdersCount)
	}
	t.Log("Phase 3 PASS: Copy orders were strictly blocked. Zero follower orders created.")

	// -----------------------------------------------------------------
	// Phase 4: Re-activate copy (Start Copy) -> Verify copying resumes
	// -----------------------------------------------------------------
	t.Log("=== Phase 4: Re-activate Copy (active: true) -> Verify Resumption ===")

	patchResp, patchBody, err = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Master01ID, map[string]any{
		"active": true,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch master active=true failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	err = h.DB.QueryRow(ctx, "SELECT active FROM accounts WHERE id = $1", Master01ID).Scan(&masterActive)
	if err != nil || !masterActive {
		t.Fatalf("expected master active=true in DB, got: %v, err=%v", masterActive, err)
	}

	countsBeforeResumeTrade := getFollowerOrderCounts()

	// Master places third order
	order3Resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        400,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order 3 failed: %v", err)
	}
	t.Logf("Master 1 placed third order after resumption: %s", order3Resp.OrderID)

	// Wait for followers to receive resumed copy orders
	deadline = time.Now().Add(5 * time.Second)
	resumedReceived := false
	for time.Now().Before(deadline) {
		counts := getFollowerOrderCounts()
		if counts["follow01a"] > countsBeforeResumeTrade["follow01a"] &&
			counts["follow01b"] > countsBeforeResumeTrade["follow01b"] &&
			counts["follow01c"] > countsBeforeResumeTrade["follow01c"] &&
			counts["follow01d"] > countsBeforeResumeTrade["follow01d"] {
			resumedReceived = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !resumedReceived {
		t.Fatalf("Phase 4 failed: followers did not resume receiving copy orders after re-activation")
	}

	t.Log("Phase 4 PASS: Copy trades successfully resumed after master re-activation.")
}

func TestE2E_TurnCopyOn5SecondsAfterOrderPlaced(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	// Enable Follower 01B so all 4 followers in Group 01 are active
	resp, body, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{
		"enabled": true,
	})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("enable Follower 01B failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}


	masterCli := h.MasterClient("master01")
	fCliA := h.FollowerClient("follow01a")
	fCliB := h.FollowerClient("follow01b")
	fCliC := h.FollowerClient("follow01c")
	fCliD := h.FollowerClient("follow01d")

	getFollowerOrderCounts := func() map[string]int {
		counts := make(map[string]int)
		for name, cli := range map[string]*sdk.Client{
			"follow01a": fCliA,
			"follow01b": fCliB,
			"follow01c": fCliC,
			"follow01d": fCliD,
		} {
			orders, err := cli.GetOrders()
			if err != nil {
				t.Fatalf("get %s orders: %v", name, err)
			}
			counts[name] = len(orders)
		}
		return counts
	}

	// Step 1: Ensure master copy is STOPPED
	t.Log("=== Step 1: Ensure master copy is STOPPED (active: false) ===")
	patchResp, patchBody, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Master01ID, map[string]any{
		"active": false,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("stop copy failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	var masterActive bool
	err = h.DB.QueryRow(ctx, "SELECT active FROM accounts WHERE id = $1", Master01ID).Scan(&masterActive)
	if err != nil || masterActive {
		t.Fatalf("expected master active=false in DB, got: %v, err=%v", masterActive, err)
	}
	t.Log("Master copy confirmed STOPPED (active=false)")

	countsAtStart := getFollowerOrderCounts()

	// Step 2: Master places an order on broker end while copy is stopped
	t.Log("=== Step 2: Master places order while copy is stopped ===")
	tOrderPlaced := time.Now()
	order1Resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        400,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order while stopped failed: %v", err)
	}
	t.Logf("Order placed on broker end at %s (OrderID: %s)", tOrderPlaced.Format("15:04:05.000"), order1Resp.OrderID)

	// Step 3: Wait 5 seconds before turning copy on
	t.Log("=== Step 3: Waiting 5 seconds before turning copy on ===")
	time.Sleep(5 * time.Second)
	tTurnOn := time.Now()
	t.Logf("5 seconds elapsed (TurnOn Time: %s, Delta: %v)", tTurnOn.Format("15:04:05.000"), tTurnOn.Sub(tOrderPlaced).Round(time.Millisecond))

	// Step 4: Turn copy ON (5 seconds after order was placed on broker end)
	t.Log("=== Step 4: Turn copy ON (PATCH active: true) ===")
	patchResp, patchBody, err = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Master01ID, map[string]any{
		"active": true,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("start copy failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	err = h.DB.QueryRow(ctx, "SELECT active FROM accounts WHERE id = $1", Master01ID).Scan(&masterActive)
	if err != nil || !masterActive {
		t.Fatalf("expected master active=true in DB, got: %v, err=%v", masterActive, err)
	}
	t.Log("Master copy turned ON (active=true in API and DB)")

	// Step 5: Wait 3 seconds and verify that the 5-second-old blocked order was NOT copied
	t.Log("=== Step 5: Verify that the past blocked order was NOT retroactively copied ===")
	time.Sleep(3 * time.Second)

	countsAfterTurnOn := getFollowerOrderCounts()
	for name, count := range countsAfterTurnOn {
		if count != countsAtStart[name] {
			t.Fatalf("Order placed while stopped was incorrectly copied after turning copy ON! Follower: %s, Before: %d, After: %d",
				name, countsAtStart[name], count)
		}
	}

	var blockedFillID int64
	err = h.DB.QueryRow(ctx, "SELECT id FROM master_fills WHERE broker_order_id = $1", order1Resp.OrderID).Scan(&blockedFillID)
	if err != nil {
		t.Fatalf("query blocked master fill: %v", err)
	}

	var followerOrdersForBlockedFill int
	err = h.DB.QueryRow(ctx, "SELECT count(*) FROM follower_orders WHERE master_fill_id = $1", blockedFillID).Scan(&followerOrdersForBlockedFill)
	if err != nil {
		t.Fatalf("count follower orders for blocked fill: %v", err)
	}
	if followerOrdersForBlockedFill != 0 {
		t.Fatalf("expected 0 follower_orders for blocked fill %s, got %d", order1Resp.OrderID, followerOrdersForBlockedFill)
	}
	t.Log("PASS: Follower order count remained 0 for the order placed while stopped.")

	// Step 6: Master places a NEW order while copy is active -> verify this new order IS copied
	t.Log("=== Step 6: Master places new order while active -> Verify it IS copied ===")
	countsBeforeNewOrder := getFollowerOrderCounts()
	order2Resp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        400,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order 2 failed: %v", err)
	}
	t.Logf("New order placed while active: %s", order2Resp.OrderID)

	deadline := time.Now().Add(5 * time.Second)
	newOrderCopied := false
	for time.Now().Before(deadline) {
		counts := getFollowerOrderCounts()
		if counts["follow01a"] > countsBeforeNewOrder["follow01a"] &&
			counts["follow01b"] > countsBeforeNewOrder["follow01b"] &&
			counts["follow01c"] > countsBeforeNewOrder["follow01c"] &&
			counts["follow01d"] > countsBeforeNewOrder["follow01d"] {
			newOrderCopied = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !newOrderCopied {
		t.Fatalf("New order placed while active was NOT copied to followers")
	}
	t.Log("PASS: New order placed after turning copy ON was successfully copied to all followers!")
}

