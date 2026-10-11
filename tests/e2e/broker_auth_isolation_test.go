//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestE2E_BrokerAuthExpiration_DoesNotInvalidateTraderWebSession(t *testing.T) {
	h := NewHarness(t)

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	cleanup := func() {
		_, _ = h.DB.Exec(context.Background(), `
			DELETE FROM order_events WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'BADTOKEN01');
			DELETE FROM follower_orders WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id = 'BADTOKEN01');
			DELETE FROM account_positions WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'BADTOKEN01');
			DELETE FROM account_holdings WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'BADTOKEN01');
			DELETE FROM account_margins WHERE account_id IN (SELECT id FROM accounts WHERE broker_user_id = 'BADTOKEN01');
			DELETE FROM follow_links WHERE follower_id IN (SELECT id FROM accounts WHERE broker_user_id = 'BADTOKEN01');
			DELETE FROM accounts WHERE broker_user_id = 'BADTOKEN01';
			INSERT INTO proxy_ips (ip_address, ip_type, host, port, username, password, valid_from, valid_until, plan)
			VALUES ('148.113.41.52', 'ipv4', 'dc46-mum-01.algoip.in', 443, 'mock_u12', 'mock_p12', now() - interval '1 day', now() + interval '90 days', 'QUARTERLY')
			ON CONFLICT (ip_address) DO NOTHING;
		`)
	}
	t.Cleanup(cleanup)
	cleanup()

	// 1. Verify trader's web session is initially authenticated
	meResp, meBody, err := h.EnvoyAPI(http.MethodGet, "/api/v1/auth/me", nil)
	if err != nil || meResp.StatusCode != http.StatusOK {
		t.Fatalf("initial /auth/me failed: status=%d, body=%s, err=%v", meResp.StatusCode, string(meBody), err)
	}
	t.Log("Trader web session is verified active and authenticated (HTTP 200)")

	// 2. Add follower account attached to Master01 with an INVALID/EXPIRED broker token
	badFollowerReq := map[string]any{
		"role":            "follower",
		"broker":          "testbroker",
		"brokerAccountId": "BADTOKEN01",
		"name":            "Broken Broker Token Follower",
		"apiKey":          "key_badtoken01",
		"apiSecret":       "secret_badtoken01",
		"accessToken":     "token_completely_invalid_and_expired",
		"ip":              "148.113.41.52",
		"masterId":        Master01ID,
		"cloneFactor":     "1.0",
		"maxQtyPerOrder":  500,
	}

	createResp, createBody, err := h.EnvoyAPI(http.MethodPost, "/api/v1/accounts", badFollowerReq)
	if err != nil || createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create bad token follower failed: status=%d, body=%s, err=%v", createResp.StatusCode, string(createBody), err)
	}

	var createdAcc map[string]any
	if err := json.Unmarshal(createBody, &createdAcc); err != nil {
		t.Fatalf("failed to decode created account response: %v", err)
	}
	badFollowerID, _ := createdAcc["id"].(string)
	t.Logf("Created follower %s with expired broker token", badFollowerID)

	// 3. Query group rebalance diff for Group 1.
	// Since BADTOKEN01 is in Group 1 with an invalid broker token, rebalance calculation
	// encounters KiteError [403 TokenException] and returns 401 Unauthorized for the broker operation.
	diffResp, diffBody, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
	if err != nil {
		t.Fatalf("rebalance diff request failed: %v", err)
	}
	if diffResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized from broker token expiration, got %d: %s", diffResp.StatusCode, string(diffBody))
	}
	t.Logf("Rebalance diff correctly returned 401 Unauthorized for broker token error: %s", string(diffBody))

	// 4. CRITICAL ASSERTION: The trader's web session must REMAIN VALID!
	// In the past bug, a 401 from rebalance diff was incorrectly treated as a session logout,
	// wiping the user's session and kicking them to /login.
	// Here, we assert that /auth/me and /groups STILL return 200 with the exact same session cookie!
	meRespAfter, meBodyAfter, err := h.EnvoyAPI(http.MethodGet, "/api/v1/auth/me", nil)
	if err != nil || meRespAfter.StatusCode != http.StatusOK {
		t.Fatalf("REGRESSION BUG: Trader web session was killed by broker 401! status=%d, body=%s, err=%v", meRespAfter.StatusCode, string(meBodyAfter), err)
	}

	var userResp map[string]any
	if err := json.Unmarshal(meBodyAfter, &userResp); err != nil || userResp["user"] == nil {
		t.Fatalf("user object missing from /auth/me after broker 401: %s", string(meBodyAfter))
	}
	t.Log("PASS: Trader web session remains 100% active and authenticated (HTTP 200) after broker token failure!")

	groupsResp, groupsBody, err := h.EnvoyAPI(http.MethodGet, "/api/v1/groups", nil)
	if err != nil || groupsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /groups failed with same session: status=%d, body=%s, err=%v", groupsResp.StatusCode, string(groupsBody), err)
	}
	t.Log("PASS: GET /groups succeeded with active session (HTTP 200)")

	// 5. Clean up bad follower by deactivating / deleting it
	_, _, _ = h.EnvoyAPI(http.MethodDelete, "/api/v1/accounts/"+badFollowerID, nil)
	t.Log("Cleaned up follower account")
}
