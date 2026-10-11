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

func TestE2E_FollowerActivationFlow_And_CopyTradeScenarios(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	cleanup := func() {
		_, _ = h.DB.Exec(context.Background(), `
			DELETE FROM order_events WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01'));
			DELETE FROM follower_orders WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01'));
			DELETE FROM account_positions WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01'));
			DELETE FROM account_holdings WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01'));
			DELETE FROM account_margins WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01'));
			DELETE FROM follow_links WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01'));
			DELETE FROM accounts WHERE broker_user_id IN ('NEWFOLLOW01', 'ACTFLOW01');
		`)
	}
	t.Cleanup(cleanup)
	cleanup()

	// -----------------------------------------------------------------
	// Setup: Create a new follower account attached to Master01
	// -----------------------------------------------------------------
	newFollowerReq := map[string]any{
		"role":            "follower",
		"broker":          "testbroker",
		"brokerAccountId": "NEWFOLLOW01",
		"name":            "Activation Flow Follower",
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
		t.Fatalf("create follower failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}

	var createdAcc map[string]any
	if err := json.Unmarshal(body, &createdAcc); err != nil {
		t.Fatalf("failed to decode created account response: %v", err)
	}
	followerID, ok := createdAcc["id"].(string)
	if !ok || followerID == "" {
		t.Fatalf("created account response missing valid id: %s", string(body))
	}
	t.Logf("Created follower: %s (NEWFOLLOW01)", followerID)

	masterCli := h.MasterClient("master01")
	followerCli := h.FollowerClient("newfollow01")

	// Helper to count follower orders received in testbroker
	countFollowerOrders := func() int {
		orders, err := followerCli.GetOrders()
		if err != nil {
			t.Fatalf("get follower orders: %v", err)
		}
		return len(orders)
	}

	// -----------------------------------------------------------------
	// Scenario 1: Deactivate account from Accounts page (active: false)
	// Expectation:
	// - API response contains active=false, enabled=false
	// - DB has accounts.active=false, follow_links.enabled=false
	// - Master order is NOT copied to this follower
	// -----------------------------------------------------------------
	t.Log("=== Scenario 1: Deactivate from Accounts page ===")
	patchResp, patchBody, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+followerID, map[string]any{
		"active": false,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch active=false failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	var s1Resp map[string]any
	_ = json.Unmarshal(patchBody, &s1Resp)
	if s1Resp["active"] != false || s1Resp["enabled"] != false {
		t.Fatalf("expected response {active: false, enabled: false}, got: %s", string(patchBody))
	}

	// Verify DB state
	var accActive, linkEnabled bool
	err = h.DB.QueryRow(ctx, "SELECT a.active, COALESCE(f.enabled, false) FROM accounts a LEFT JOIN follow_links f ON f.follower_id = a.id WHERE a.id = $1", followerID).Scan(&accActive, &linkEnabled)
	if err != nil || accActive != false || linkEnabled != false {
		t.Fatalf("DB state check failed: accActive=%v (want false), linkEnabled=%v (want false), err=%v", accActive, linkEnabled, err)
	}

	// Master places order
	initialCount := countFollowerOrders()
	_, err = masterCli.PlaceOrder("regular", sdk.OrderParams{
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

	time.Sleep(1500 * time.Millisecond)
	if countFollowerOrders() != initialCount {
		t.Fatalf("Scenario 1 failed: deactivated follower received unexpected order")
	}
	t.Log("Scenario 1 PASS: follower received 0 orders while deactivated")

	// -----------------------------------------------------------------
	// Scenario 2: Reactivate account from Accounts page (active: true)
	// Expectation:
	// - API response contains active=true
	// - DB has accounts.active=true, follow_links.enabled=false (SAFETY FIRST)
	// - Master order is NOT copied to this follower
	// -----------------------------------------------------------------
	t.Log("=== Scenario 2: Reactivate from Accounts page (Safety First) ===")
	patchResp, patchBody, err = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+followerID, map[string]any{
		"active": true,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch active=true failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	var s2Resp map[string]any
	_ = json.Unmarshal(patchBody, &s2Resp)
	if s2Resp["active"] != true {
		t.Fatalf("expected response {active: true}, got: %s", string(patchBody))
	}

	err = h.DB.QueryRow(ctx, "SELECT a.active, COALESCE(f.enabled, false) FROM accounts a LEFT JOIN follow_links f ON f.follower_id = a.id WHERE a.id = $1", followerID).Scan(&accActive, &linkEnabled)
	if err != nil || accActive != true || linkEnabled != false {
		t.Fatalf("DB state check failed: accActive=%v (want true), linkEnabled=%v (want false), err=%v", accActive, linkEnabled, err)
	}

	initialCount = countFollowerOrders()
	_, err = masterCli.PlaceOrder("regular", sdk.OrderParams{
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

	time.Sleep(1500 * time.Millisecond)
	if countFollowerOrders() != initialCount {
		t.Fatalf("Scenario 2 failed: reactivated follower with copy OFF received unexpected order")
	}
	t.Log("Scenario 2 PASS: follower received 0 orders (copy stayed paused)")

	// -----------------------------------------------------------------
	// Scenario 3: Enable copy from Dashboard (enabled: true)
	// Expectation:
	// - API response contains enabled=true, active=true
	// - DB has accounts.active=true, follow_links.enabled=true
	// - Master order IS copied to this follower!
	// -----------------------------------------------------------------
	t.Log("=== Scenario 3: Enable copy from Dashboard ===")
	patchResp, patchBody, err = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+followerID, map[string]any{
		"enabled":     true,
		"accessToken": "token_newfollow01",
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch enabled=true failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	var s3Resp map[string]any
	_ = json.Unmarshal(patchBody, &s3Resp)
	if s3Resp["enabled"] != true || s3Resp["active"] != true {
		t.Fatalf("expected response {enabled: true, active: true}, got: %s", string(patchBody))
	}

	err = h.DB.QueryRow(ctx, "SELECT a.active, COALESCE(f.enabled, false) FROM accounts a LEFT JOIN follow_links f ON f.follower_id = a.id WHERE a.id = $1", followerID).Scan(&accActive, &linkEnabled)
	if err != nil || accActive != true || linkEnabled != true {
		t.Fatalf("DB state check failed: accActive=%v (want true), linkEnabled=%v (want true), err=%v", accActive, linkEnabled, err)
	}

	initialCount = countFollowerOrders()
	_, err = masterCli.PlaceOrder("regular", sdk.OrderParams{
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

	deadline := time.Now().Add(5 * time.Second)
	orderReceived := false
	for time.Now().Before(deadline) {
		if countFollowerOrders() > initialCount {
			orderReceived = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !orderReceived {
		t.Fatalf("Scenario 3 failed: enabled follower did not receive copy order within timeout")
	}
	t.Log("Scenario 3 PASS: follower received copy trade successfully")

	// -----------------------------------------------------------------
	// Scenario 4: Pause copy from Dashboard (enabled: false)
	// Expectation:
	// - API response contains enabled=false
	// - DB has accounts.active=true, follow_links.enabled=false
	// - Master order is NOT copied to this follower
	// -----------------------------------------------------------------
	t.Log("=== Scenario 4: Pause copy from Dashboard ===")
	patchResp, patchBody, err = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+followerID, map[string]any{
		"enabled": false,
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch enabled=false failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	err = h.DB.QueryRow(ctx, "SELECT a.active, COALESCE(f.enabled, false) FROM accounts a LEFT JOIN follow_links f ON f.follower_id = a.id WHERE a.id = $1", followerID).Scan(&accActive, &linkEnabled)
	if err != nil || accActive != true || linkEnabled != false {
		t.Fatalf("DB state check failed: accActive=%v (want true), linkEnabled=%v (want false), err=%v", accActive, linkEnabled, err)
	}

	initialCount = countFollowerOrders()
	_, err = masterCli.PlaceOrder("regular", sdk.OrderParams{
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

	time.Sleep(1500 * time.Millisecond)
	if countFollowerOrders() != initialCount {
		t.Fatalf("Scenario 4 failed: paused follower received unexpected order")
	}
	t.Log("Scenario 4 PASS: follower stayed active for sync, but received 0 copy orders")

	// -----------------------------------------------------------------
	// Scenario 5: Account is Inactive on Accounts page (active: false),
	//             trader toggles Copy ON from Dashboard (enabled: true)
	// Expectation:
	// - Toggling copy ON automatically activates account (active: true)
	// - Both active=true and enabled=true in DB
	// - Follower receives copy orders
	// -----------------------------------------------------------------
	t.Log("=== Scenario 5: Auto-activation from Dashboard Copy ON ===")
	// First deactivate account
	_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+followerID, map[string]any{
		"active": false,
	})

	// Trader turns Copy ON from dashboard
	patchResp, patchBody, err = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+followerID, map[string]any{
		"enabled":     true,
		"accessToken": "token_newfollow01",
	})
	if err != nil || patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch enabled=true on inactive follower failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
	}

	var s5Resp map[string]any
	_ = json.Unmarshal(patchBody, &s5Resp)
	if s5Resp["enabled"] != true || s5Resp["active"] != true {
		t.Fatalf("expected response {enabled: true, active: true}, got: %s", string(patchBody))
	}

	err = h.DB.QueryRow(ctx, "SELECT a.active, COALESCE(f.enabled, false) FROM accounts a LEFT JOIN follow_links f ON f.follower_id = a.id WHERE a.id = $1", followerID).Scan(&accActive, &linkEnabled)
	if err != nil || accActive != true || linkEnabled != true {
		t.Fatalf("DB state check failed: accActive=%v (want true), linkEnabled=%v (want true), err=%v", accActive, linkEnabled, err)
	}

	initialCount = countFollowerOrders()
	_, err = masterCli.PlaceOrder("regular", sdk.OrderParams{
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

	deadline = time.Now().Add(5 * time.Second)
	orderReceived = false
	for time.Now().Before(deadline) {
		if countFollowerOrders() > initialCount {
			orderReceived = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !orderReceived {
		t.Fatalf("Scenario 5 failed: auto-activated follower did not receive copy order within timeout")
	}
	t.Log("Scenario 5 PASS: follower was auto-activated and successfully copied the trade!")
}
