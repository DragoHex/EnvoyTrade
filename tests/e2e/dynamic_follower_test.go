//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
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

func TestE2E_BatchFollowerAddition_And_CloneFactorSizing(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	// Clean up any stale accounts from previous runs and ensure test proxy IPs exist
	_, _ = h.DB.Exec(ctx, `
		DELETE FROM order_events WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03'));
		DELETE FROM follower_orders WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03'));
		DELETE FROM account_positions WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03'));
		DELETE FROM account_holdings WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03'));
		DELETE FROM account_margins WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03'));
		DELETE FROM follow_links WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03'));
		DELETE FROM accounts WHERE broker_user_id IN ('BATCHF01', 'BATCHF02', 'BATCHF03');
		INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, whitelisted_brokers, valid_from, valid_until, plan)
		VALUES
		  ('148.113.41.101', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u101', 'mock_p101', 'TestBroker', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
		  ('148.113.41.102', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u102', 'mock_p102', 'TestBroker', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY'),
		  ('148.113.41.103', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u103', 'mock_p103', 'TestBroker', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY')
		ON CONFLICT (ip_address) DO UPDATE SET whitelisted_brokers = 'TestBroker';
	`)

	// 1. Create 3 unassigned follower accounts
	followers := []struct {
		user string
		ip   string
		name string
	}{
		{user: "BATCHF01", ip: "148.113.41.101", name: "Batch Follower 01"},
		{user: "BATCHF02", ip: "148.113.41.102", name: "Batch Follower 02"},
		{user: "BATCHF03", ip: "148.113.41.103", name: "Batch Follower 03"},
	}

	accountIDs := make(map[string]string)
	for _, f := range followers {
		req := map[string]any{
			"role":            "follower",
			"broker":          "testbroker",
			"brokerAccountId": f.user,
			"name":            f.name,
			"apiKey":          "key_" + strings.ToLower(f.user),
			"apiSecret":       "secret_" + strings.ToLower(f.user),
			"accessToken":     "token_" + strings.ToLower(f.user),
			"ip":              f.ip,
		}
		resp, body, err := h.EnvoyAPI(http.MethodPost, "/api/v1/accounts", req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("create follower %s failed: status=%d, body=%s, err=%v", f.user, resp.StatusCode, string(body), err)
		}
		var created map[string]any
		_ = json.Unmarshal(body, &created)
		accountIDs[f.user] = created["id"].(string)
	}

	// 2. Attach all 3 followers in a single batch call via POST /api/v1/groups/{Group01ID}/followers
	// BATCHF01: cloneFactor = 0.5 (numeric)
	// BATCHF02: cloneFactor = "1.5" (string)
	// BATCHF03: cloneFactor = 2.0 (numeric), maxQtyPerOrder = 120
	batchPayload := []map[string]any{
		{"accountId": accountIDs["BATCHF01"], "cloneFactor": 0.5, "maxQtyPerOrder": 500},
		{"accountId": accountIDs["BATCHF02"], "cloneFactor": "1.5", "maxQtyPerOrder": 500},
		{"accountId": accountIDs["BATCHF03"], "cloneFactor": 2.0, "maxQtyPerOrder": 120},
	}
	resp, body, err := h.EnvoyAPI(http.MethodPost, "/api/v1/groups/"+Group01ID+"/followers", batchPayload)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("batch attach followers failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}

	// 3. Verify PostgreSQL follow_links state
	type linkCheck struct {
		cf     decimal.Decimal
		maxQty *int
	}
	expected := map[string]linkCheck{
		accountIDs["BATCHF01"]: {cf: decimal.RequireFromString("0.5"), maxQty: intPtr(500)},
		accountIDs["BATCHF02"]: {cf: decimal.RequireFromString("1.5"), maxQty: intPtr(500)},
		accountIDs["BATCHF03"]: {cf: decimal.RequireFromString("2.0"), maxQty: intPtr(120)},
	}
	for id, exp := range expected {
		var cfStr string
		var maxQty *int
		var enabled bool
		err := h.DB.QueryRow(ctx, "SELECT clone_factor::text, max_qty_per_order, enabled FROM follow_links WHERE follower_id = $1", id).Scan(&cfStr, &maxQty, &enabled)
		if err != nil {
			t.Fatalf("query follow_link for %s: %v", id, err)
		}
		gotCF, err := decimal.NewFromString(cfStr)
		if err != nil || !gotCF.Equal(exp.cf) || !enabled {
			t.Errorf("follow_link %s mismatch: got cf=%s, enabled=%v; want cf=%s, enabled=true", id, cfStr, enabled, exp.cf.String())
		}
		if exp.maxQty != nil && (maxQty == nil || *maxQty != *exp.maxQty) {
			t.Errorf("follow_link %s maxQty mismatch: got %v, want %v", id, maxQty, *exp.maxQty)
		}
	}

	// 4. Master places order: RELIANCE BUY MARKET 100 Qty (Lot size = 1)
	masterCli := h.MasterClient("master01")
	mResp, err := masterCli.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        100,
		Product:         "CNC",
	})
	if err != nil {
		t.Fatalf("master place order failed: %v", err)
	}
	t.Logf("Master 1 placed order: %s (qty 100)", mResp.OrderID)

	// 5. Verify fan-out quantities:
	// BATCHF01: 100 * 0.5 = 50
	// BATCHF02: 100 * 1.5 = 150
	// BATCHF03: 100 * 2.0 = 200, but capped at maxQty 120 -> 120
	expectedQuantities := map[string]int{
		"BATCHF01": 50,
		"BATCHF02": 150,
		"BATCHF03": 120,
	}

	for user, wantQty := range expectedQuantities {
		cli := h.FollowerClient(strings.ToLower(user))
		var matchedOrder *sdk.Order
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			orders, err := cli.GetOrders()
			if err == nil {
				for _, o := range orders {
					if o.Tradingsymbol == "RELIANCE" && o.TransactionType == "BUY" && o.Quantity == wantQty {
						matchedOrder = &o
						break
					}
				}
			}
			if matchedOrder != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if matchedOrder == nil {
			t.Fatalf("follower %s did not receive fan-out order with quantity %d within timeout", user, wantQty)
		}
		t.Logf("Follower %s received fan-out order: %s with expected quantity: %d", user, matchedOrder.OrderID, matchedOrder.Quantity)
	}
}

func intPtr(i int) *int {
	return &i
}

