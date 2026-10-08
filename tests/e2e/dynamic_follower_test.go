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

func TestE2E_DynamicFollowerAddition_And_FanOut(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	// Clean up any stale NEWFOLLOW01 account from previous runs
	_, _ = h.DB.Exec(context.Background(), `
		DELETE FROM order_events WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'NEWFOLLOW01');
		DELETE FROM follower_orders WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id = 'NEWFOLLOW01');
		DELETE FROM account_positions WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'NEWFOLLOW01');
		DELETE FROM account_holdings WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'NEWFOLLOW01');
		DELETE FROM account_margins WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'NEWFOLLOW01');
		DELETE FROM follow_links WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id = 'NEWFOLLOW01');
		DELETE FROM accounts WHERE broker_user_id = 'NEWFOLLOW01';
	`)

	// -----------------------------------------------------------------
	// 1. Dynamically add a new follower account via EnvoyTrade API
	// -----------------------------------------------------------------
	newFollowerReq := map[string]any{
		"role":            "follower",
		"broker":          "testbroker",
		"brokerAccountId": "NEWFOLLOW01",
		"name":            "Dynamic Follower Test",
		"apiKey":          "key_newfollow01",
		"apiSecret":       "secret_newfollow01",
		"accessToken":     "token_newfollow01",
		"ip":              "148.113.41.50",
		"masterId":        Master01ID,
		"cloneFactor":     "1.0",
		"maxQtyPerOrder":  500,
	}

	resp, body, err := h.EnvoyAPI(http.MethodPost, "/api/v1/accounts", newFollowerReq)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create dynamic follower failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}

	var createdAcc map[string]any
	if err := json.Unmarshal(body, &createdAcc); err != nil {
		t.Fatalf("failed to decode created account response: %v", err)
	}
	newFollowerID, ok := createdAcc["id"].(string)
	if !ok || newFollowerID == "" {
		t.Fatalf("created account response missing valid id: %s", string(body))
	}
	t.Logf("Dynamically created and linked follower account: %s (brokerAccountId: NEWFOLLOW01)", newFollowerID)

	// Verify DB state for new follow link
	var linkEnabled bool
	err = h.DB.QueryRow(ctx, "SELECT enabled FROM follow_links WHERE follower_id = $1", newFollowerID).Scan(&linkEnabled)
	if err != nil {
		t.Fatalf("failed to query follow_links for new follower %s: %v", newFollowerID, err)
	}
	if !linkEnabled {
		t.Fatalf("new follow link is not enabled")
	}

	// -----------------------------------------------------------------
	// 2. Master places order; verify dynamic follower fan-out
	// -----------------------------------------------------------------
	masterCli := h.MasterClient("master01")
	mResp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        100,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order failed: %v", err)
	}
	t.Logf("Master 1 placed order: %s", mResp.OrderID)

	// Verify TestBroker receives order for NEWFOLLOW01
	newFollowerCli := h.FollowerClient("newfollow01")
	var newFollowerOrder *sdk.Order
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		orders, err := newFollowerCli.GetOrders()
		if err == nil {
			for _, o := range orders {
				if o.Tradingsymbol == "CRUDEOIL21SEP26" && o.TransactionType == "BUY" && o.Quantity == 100 {
					newFollowerOrder = &o
					break
				}
			}
		}
		if newFollowerOrder != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if newFollowerOrder == nil {
		t.Fatalf("new follower NEWFOLLOW01 did not receive copy order within timeout")
	}
	t.Logf("New follower NEWFOLLOW01 successfully received fan-out order: %s (qty: %d)", newFollowerOrder.OrderID, newFollowerOrder.Quantity)

	// -----------------------------------------------------------------
	// 3. Verify DB record in follower_orders is NOT DEAD_LETTERED
	// -----------------------------------------------------------------
	var brokerOrderID *string
	var terminalStatus *string
	var lastError *string
	err = h.DB.QueryRow(ctx, `
		SELECT broker_order_id, terminal_status, last_error 
		FROM follower_orders 
		WHERE follower_id = $1 
		ORDER BY id DESC LIMIT 1
	`, newFollowerID).Scan(&brokerOrderID, &terminalStatus, &lastError)
	if err != nil {
		t.Fatalf("failed to query follower_orders for %s: %v", newFollowerID, err)
	}

	if terminalStatus != nil && *terminalStatus == "DEAD_LETTERED" {
		errMsg := ""
		if lastError != nil {
			errMsg = *lastError
		}
		t.Fatalf("follower order was dead-lettered: %s", errMsg)
	}
	if brokerOrderID == nil || *brokerOrderID == "" {
		t.Fatalf("expected broker_order_id to be set on placed follower order, got nil")
	}
	t.Logf("Database confirmed follower order placed: broker_order_id=%s, terminal_status=%v (no channel full regression)", *brokerOrderID, terminalStatus)

	// -----------------------------------------------------------------
	// 4. Disable new follower and verify no further fan-out
	// -----------------------------------------------------------------
	patchResp, patchBody, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+newFollowerID, map[string]any{
		"enabled": false,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("disable follower failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	// Capture count of orders before next master order
	initialOrders, err := newFollowerCli.GetOrders()
	if err != nil {
		t.Fatalf("get orders: %v", err)
	}
	initialCount := len(initialOrders)

	// Master places another order
	mResp2, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        100,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order 2 failed: %v", err)
	}
	t.Logf("Master 1 placed order 2: %s", mResp2.OrderID)

	// Wait 1.5s and assert no new order for disabled follower
	time.Sleep(1500 * time.Millisecond)
	ordersAfter, err := newFollowerCli.GetOrders()
	if err != nil {
		t.Fatalf("get orders after: %v", err)
	}
	if len(ordersAfter) != initialCount {
		t.Fatalf("disabled follower received unexpected order: initial=%d, now=%d", initialCount, len(ordersAfter))
	}
	t.Logf("Disabled dynamic follower received 0 orders as expected")

	// -----------------------------------------------------------------
	// 5. Re-enable follower, refresh token via PATCH, verify fan-out resumes
	// -----------------------------------------------------------------
	reEnableResp, reEnableBody, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+newFollowerID, map[string]any{
		"enabled":     true,
		"accessToken": "token_newfollow01",
	})
	if err != nil || reEnableResp.StatusCode != http.StatusOK {
		t.Fatalf("re-enable follower failed: status=%d, body=%s, err=%v", reEnableResp.StatusCode, string(reEnableBody), err)
	}

	mResp3, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL21SEP26",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        100,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order 3 failed: %v", err)
	}
	t.Logf("Master 1 placed order 3 after re-enabling: %s", mResp3.OrderID)

	var resumedOrder *sdk.Order
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		orders, err := newFollowerCli.GetOrders()
		if err == nil && len(orders) > initialCount {
			resumedOrder = &orders[len(orders)-1]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if resumedOrder == nil {
		t.Fatalf("re-enabled follower did not receive fan-out order")
	}
	t.Logf("Re-enabled follower successfully received resumed fan-out order: %s", resumedOrder.OrderID)
}

