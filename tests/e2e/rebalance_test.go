//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"testbroker/sdk"
)

func TestE2E_Rebalance(t *testing.T) {
	h := NewHarness(t)

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	t.Run("Group_Rebalance_DiffAndExecution", func(t *testing.T) {
		mCli1 := h.MasterClient("master01")
		fCliA := h.FollowerClient("follow01a")

		// 1. Flatten all positions in the group first
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)

		// 2. Setup drift scenario:
		// Master places order: 75 NIFTY26OCTFUT
		_, err := mCli1.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        75,
			Product:         "NRML",
		})
		if err != nil {
			t.Fatalf("place master order: %v", err)
		}

		// Wait for follower to receive copy order
		time.Sleep(250 * time.Millisecond)

		// Now intentionally create drift on Follower 01A:
		// Place a rogue position directly on Follower 01A: BUY 15 RELIANCE
		_, err = fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        15,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place follower rogue order: %v", err)
		}

		// Also square off Follower 01A's NIFTY26OCTFUT directly via broker to simulate missed copy / drift
		_, err = fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "SELL",
			OrderType:       "MARKET",
			Quantity:        75,
			Product:         "NRML",
		})
		if err != nil {
			t.Fatalf("close follower nifty position: %v", err)
		}

		// 3. Query Diff via GET /api/v1/groups/{id}/positions/rebalance/diff
		resp, body, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
		if err != nil {
			t.Fatalf("get group diff failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		var diffRes struct {
			GroupID            string                 `json:"group_id"`
			MasterID           string                 `json:"master_id"`
			FollowersEvaluated int                    `json:"followers_evaluated"`
			FollowersWithDrift int                    `json:"followers_with_drift"`
			Drifts             []domain.FollowerDrift `json:"drifts"`
		}
		if err := json.Unmarshal(body, &diffRes); err != nil {
			t.Fatalf("unmarshal diff response: %v", err)
		}

		// Find Follower01A in drifts
		var follow01ADriftFound bool
		for _, fd := range diffRes.Drifts {
			if fd.AccountID.String() == Follow01AID {
				follow01ADriftFound = true
				if len(fd.Symbols) == 0 {
					t.Errorf("expected follower 01a to have drift symbols")
				}
				// Verify both NIFTY (BUY 75) and RELIANCE (SELL 15) are in symbols
				var hasNiftyBuy, hasRelianceSell bool
				for _, s := range fd.Symbols {
					if s.Tradingsymbol == "NIFTY26OCTFUT" && s.Action == "BUY" && s.DriftQty == 75 {
						hasNiftyBuy = true
					}
					if s.Tradingsymbol == "RELIANCE" && s.Action == "SELL" && s.DriftQty == -15 {
						hasRelianceSell = true
					}
				}
				if !hasNiftyBuy {
					t.Errorf("expected NIFTY26OCTFUT BUY 75 drift for follower 01a, got %+v", fd.Symbols)
				}
				if !hasRelianceSell {
					t.Errorf("expected RELIANCE SELL 15 drift for follower 01a, got %+v", fd.Symbols)
				}
			}
		}
		if !follow01ADriftFound {
			t.Fatalf("expected follower 01a in follower drifts list, got %+v", diffRes.Drifts)
		}

		// 4. Execute Group Rebalance via POST /api/v1/groups/{id}/positions/rebalance
		rebalResp, rebalBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance", Group01ID), map[string]any{
			"follower_ids": []string{Follow01AID},
		})
		if err != nil {
			t.Fatalf("post group rebalance failed: %v", err)
		}
		if rebalResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rebalResp.StatusCode, string(rebalBody))
		}

		var rebalResult struct {
			Status string `json:"status"`
			Orders []struct {
				FollowerID    string `json:"follower_id"`
				Tradingsymbol string `json:"tradingsymbol"`
				Status        string `json:"status"`
			} `json:"orders"`
		}
		if err := json.Unmarshal(rebalBody, &rebalResult); err != nil {
			t.Fatalf("unmarshal rebalance result: %v", err)
		}
		if rebalResult.Status != "completed" {
			t.Errorf("expected status 'completed', got %v", rebalResult.Status)
		}

		// 5. Verify Follower 01A positions are now in equilibrium with Master
		time.Sleep(100 * time.Millisecond)
		fPos, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("follower get positions: %v", err)
		}

		for _, p := range fPos.Net {
			if p.Tradingsymbol == "RELIANCE" && p.Quantity != 0 {
				t.Errorf("expected RELIANCE to be closed to 0, got %d", p.Quantity)
			}
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity != 75 {
				t.Errorf("expected NIFTY26OCTFUT to be restored to 75, got %d", p.Quantity)
			}
		}

		// 6. Clean up: square off group
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
	})

	t.Run("Single_Follower_Rebalance_DiffAndExecution", func(t *testing.T) {
		fCliA := h.FollowerClient("follow01a")

		// 1. Flatten all positions in the group first
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)

		// 2. Place rogue position on Follower 01A: BUY 10 RELIANCE
		_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        10,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place rogue order: %v", err)
		}

		// 3. GET /api/v1/accounts/{id}/positions/rebalance/diff
		resp, body, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/accounts/%s/positions/rebalance/diff", Follow01AID), nil)
		if err != nil {
			t.Fatalf("get account diff failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		var accountDiff domain.FollowerDrift
		if err := json.Unmarshal(body, &accountDiff); err != nil {
			t.Fatalf("unmarshal account diff: %v", err)
		}
		if len(accountDiff.Symbols) == 0 {
			t.Errorf("expected drift symbols for follower 01a")
		}

		// 4. POST /api/v1/accounts/{id}/positions/rebalance
		rebalResp, rebalBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/rebalance", Follow01AID), nil)
		if err != nil {
			t.Fatalf("post account rebalance: %v", err)
		}
		if rebalResp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rebalResp.StatusCode, string(rebalBody))
		}

		// 5. Verify Follower 01A position is flattened to 0
		time.Sleep(100 * time.Millisecond)
		fPos, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("get positions: %v", err)
		}
		for _, p := range fPos.Net {
			if p.Tradingsymbol == "RELIANCE" && p.Quantity != 0 {
				t.Errorf("expected RELIANCE net qty 0, got %d", p.Quantity)
			}
		}
	})

	t.Run("Group_Rebalance_EquilibriumAndSelective", func(t *testing.T) {
		fCliA := h.FollowerClient("follow01a")
		fCliB := h.FollowerClient("follow01b")

		// 1. Ensure clean flat state
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)

		// 2. Equilibrium check: diff should report 0 followers with drift
		resp, body, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get diff: status=%d, err=%v", resp.StatusCode, err)
		}
		var cleanDiff domain.GroupRebalanceDiff
		if err := json.Unmarshal(body, &cleanDiff); err != nil {
			t.Fatalf("unmarshal clean diff: %v", err)
		}
		if cleanDiff.FollowersWithDrift != 0 {
			t.Errorf("expected 0 followers with drift, got %d", cleanDiff.FollowersWithDrift)
		}

		// 3. Create rogue positions on BOTH Follower 01A and Follower 01B
		_, err = fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        5,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place fA order: %v", err)
		}
		_, err = fCliB.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        75,
			Product:         "NRML",
		})
		if err != nil {
			t.Fatalf("place fB order: %v", err)
		}

		// 4. Selective rebalance: ONLY select Follower 01A
		rebalResp, rebalBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance", Group01ID), map[string]any{
			"follower_ids": []string{Follow01AID},
		})
		if err != nil || rebalResp.StatusCode != http.StatusOK {
			t.Fatalf("post selective rebalance: status=%d, body=%s, err=%v", rebalResp.StatusCode, string(rebalBody), err)
		}

		time.Sleep(100 * time.Millisecond)

		// 5. Follower 01A RELIANCE should now be 0
		posA, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("get posA: %v", err)
		}
		for _, p := range posA.Net {
			if p.Tradingsymbol == "RELIANCE" && p.Quantity != 0 {
				t.Errorf("expected Follower 01A RELIANCE to be 0, got %d", p.Quantity)
			}
		}

		// 6. Follower 01B NIFTY26OCTFUT should STILL be open (qty 75)
		posB, err := fCliB.GetPositions()
		if err != nil {
			t.Fatalf("get posB: %v", err)
		}
		var niftyStillOpen bool
		for _, p := range posB.Net {
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity == 75 {
				niftyStillOpen = true
				break
			}
		}
		if !niftyStillOpen {
			t.Errorf("expected Follower 01B NIFTY26OCTFUT to remain untouched with qty 75, got %+v", posB.Net)
		}

		// Cleanup
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
	})

	t.Run("Rebalance_SkipsDisabledFollower", func(t *testing.T) {
		mCli1 := h.MasterClient("master01")
		fCliB := h.FollowerClient("follow01b") // disabled follower

		// Ensure follow01b is disabled
		_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{"enabled": false})

		// Master opens position creating drift: BUY 100 INFY
		_, err := mCli1.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "INFY",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        100,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place master buy order: %v", err)
		}

		// 1. Single account rebalance call on disabled follower must return 400 Bad Request
		sResp, _, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/rebalance", Follow01BID), nil)
		if err != nil {
			t.Fatalf("post account rebalance failed: %v", err)
		}
		if sResp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for disabled follower rebalance, got %d", sResp.StatusCode)
		}

		// 2. Group diff must list follow01b with enabled = false
		dResp, dBody, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
		if err != nil || dResp.StatusCode != http.StatusOK {
			t.Fatalf("group diff failed: status=%d, body=%s, err=%v", dResp.StatusCode, string(dBody), err)
		}
		var diffRes struct {
			Drifts []domain.FollowerDrift `json:"drifts"`
		}
		_ = json.Unmarshal(dBody, &diffRes)
		var fFound bool
		for _, d := range diffRes.Drifts {
			if d.AccountID.String() == Follow01BID {
				fFound = true
				if d.Enabled {
					t.Errorf("expected follow01b in diff to have enabled=false")
				}
				break
			}
		}
		if !fFound {
			t.Errorf("expected follow01b to be listed in diff with drift")
		}

		// 3. Group rebalance explicitly requesting follow01b must NOT place any orders on follow01b
		ordersBefore, _ := fCliB.GetOrders()
		gResp, gBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance", Group01ID), map[string]any{
			"follower_ids": []string{Follow01BID},
		})
		if err != nil || gResp.StatusCode != http.StatusOK {
			t.Fatalf("group rebalance failed: status=%d, body=%s, err=%v", gResp.StatusCode, string(gBody), err)
		}

		ordersAfter, _ := fCliB.GetOrders()
		if len(ordersAfter) != len(ordersBefore) {
			t.Errorf("expected 0 new orders on disabled follower, got %d", len(ordersAfter)-len(ordersBefore))
		}

		// Cleanup master position
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/square-off", Master01ID), map[string]any{})
	})
}

