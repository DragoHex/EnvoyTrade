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

func TestE2E_GroupImbalance(t *testing.T) {
	h := NewHarness(t)

	// Ensure execution mode is instant on testbroker
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	getGroupHasImbalance := func(t *testing.T, groupID string) (bool, bool) {
		t.Helper()
		resp, body, err := h.EnvoyAPI(http.MethodGet, "/api/v1/groups", nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get groups failed: %v", err)
		}
		var list []struct {
			ID           string `json:"id"`
			HasImbalance bool   `json:"hasImbalance"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			t.Fatalf("unmarshal groups: %v", err)
		}
		for _, g := range list {
			if g.ID == groupID {
				return g.HasImbalance, true
			}
		}
		return false, false
	}

	getGroupDetailHasImbalance := func(t *testing.T, groupID string) bool {
		t.Helper()
		resp, body, err := h.EnvoyAPI(http.MethodGet, "/api/v1/groups/"+groupID, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get group detail failed: status=%d, err=%v", resp.StatusCode, err)
		}
		var detail struct {
			ID           string `json:"id"`
			HasImbalance bool   `json:"hasImbalance"`
		}
		if err := json.Unmarshal(body, &detail); err != nil {
			t.Fatalf("unmarshal group detail: %v", err)
		}
		return detail.HasImbalance
	}

	t.Run("Equilibrium_CleanState", func(t *testing.T) {
		// 1. Ensure all positions in Group01 are squared off
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)

		// 2. Fetch group diff
		dResp, dBody, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
		if err != nil || dResp.StatusCode != http.StatusOK {
			t.Fatalf("get diff failed: %v, body=%s", err, string(dBody))
		}
		var diff domain.GroupRebalanceDiff
		if err := json.Unmarshal(dBody, &diff); err != nil {
			t.Fatalf("unmarshal diff: %v", err)
		}
		if diff.FollowersWithDrift != 0 {
			t.Fatalf("expected 0 followers with drift, got %d", diff.FollowersWithDrift)
		}

		// 3. Verify hasImbalance is false in both summary and detail
		hasImb, found := getGroupHasImbalance(t, Group01ID)
		if !found {
			t.Fatalf("group %s not found in groups summary", Group01ID)
		}
		if hasImb {
			t.Errorf("expected group hasImbalance = false for flat state, got true")
		}
		if detailImb := getGroupDetailHasImbalance(t, Group01ID); detailImb {
			t.Errorf("expected group detail hasImbalance = false for flat state, got true")
		}
	})

	t.Run("RoguePosition_ActiveImbalance", func(t *testing.T) {
		fCliA := h.FollowerClient("follow01a") // enabled follower

		// Create rogue position directly on follower 01a: BUY 20 RELIANCE
		_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        20,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place rogue order on follow01a: %v", err)
		}

		// Query diff to trigger evaluation
		dResp, dBody, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
		if err != nil || dResp.StatusCode != http.StatusOK {
			t.Fatalf("get diff: %v", err)
		}
		var diff domain.GroupRebalanceDiff
		_ = json.Unmarshal(dBody, &diff)
		if diff.FollowersWithDrift == 0 {
			t.Fatalf("expected followers with drift > 0, got 0")
		}

		// Verify hasImbalance = true in summary and detail
		hasImb, _ := getGroupHasImbalance(t, Group01ID)
		if !hasImb {
			t.Errorf("expected group hasImbalance = true after rogue position on enabled follower, got false")
		}
		if detailImb := getGroupDetailHasImbalance(t, Group01ID); !detailImb {
			t.Errorf("expected group detail hasImbalance = true after rogue position, got false")
		}

		// Cleanup: square off group
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("SelectiveRebalance_PreservesImbalanceWhenOtherDriftsRemain", func(t *testing.T) {
		fCliA := h.FollowerClient("follow01a")
		fCliD := h.FollowerClient("follow01d")

		// Create rogue positions on BOTH Follower 01A and Follower 01D
		_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        10,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place fA rogue: %v", err)
		}
		_, err = fCliD.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        75,
			Product:         "NRML",
		})
		if err != nil {
			t.Fatalf("place fD rogue: %v", err)
		}

		// Trigger diff
		_, _, _ = h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)

		// Selective rebalance: ONLY rebalance Follower 01A
		rebalResp, rebalBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance", Group01ID), map[string]any{
			"follower_ids": []string{Follow01AID},
		})
		if err != nil || rebalResp.StatusCode != http.StatusOK {
			t.Fatalf("selective rebalance failed: status=%d, body=%s, err=%v", rebalResp.StatusCode, string(rebalBody), err)
		}

		// Group must STILL have hasImbalance = true because Follower 01D is still drifting!
		hasImb, _ := getGroupHasImbalance(t, Group01ID)
		if !hasImb {
			t.Errorf("expected group hasImbalance = true while Follower 01D remains drifting, got false")
		}

		// Now rebalance remaining Follower 01D
		rebalResp2, rebalBody2, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance", Group01ID), map[string]any{
			"follower_ids": []string{Follow01DID},
		})
		if err != nil || rebalResp2.StatusCode != http.StatusOK {
			t.Fatalf("second rebalance failed: status=%d, body=%s, err=%v", rebalResp2.StatusCode, string(rebalBody2), err)
		}

		// Group hasImbalance must now be false!
		hasImbAfter, _ := getGroupHasImbalance(t, Group01ID)
		if hasImbAfter {
			t.Errorf("expected group hasImbalance = false after resolving all drifts, got true")
		}

		// Cleanup
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("DisabledFollower_DriftDoesNotTriggerActiveImbalance", func(t *testing.T) {
		fCliB := h.FollowerClient("follow01b")

		// Ensure Follow01B is disabled
		_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{"enabled": false})

		// Ensure group is flat
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)

		// Place rogue position on disabled Follower 01B: BUY 15 RELIANCE
		_, err := fCliB.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        15,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place order on disabled follower: %v", err)
		}

		// Group diff must report Follower 01B with enabled = false
		dResp, dBody, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)
		if err != nil || dResp.StatusCode != http.StatusOK {
			t.Fatalf("diff failed: %v", err)
		}
		var diff domain.GroupRebalanceDiff
		_ = json.Unmarshal(dBody, &diff)

		var fFound bool
		for _, d := range diff.Drifts {
			if d.AccountID.String() == Follow01BID {
				fFound = true
				if d.Enabled {
					t.Errorf("expected follow01b to have enabled=false in diff")
				}
			}
		}
		if !fFound {
			t.Fatalf("expected follow01b to be present in diff")
		}

		// Group hasImbalance must remain FALSE because only a disabled follower has drift
		hasImb, _ := getGroupHasImbalance(t, Group01ID)
		if hasImb {
			t.Errorf("expected group hasImbalance = false when only disabled follower has drift, got true")
		}
		if detailImb := getGroupDetailHasImbalance(t, Group01ID); detailImb {
			t.Errorf("expected group detail hasImbalance = false when only disabled follower has drift, got true")
		}
	})

	t.Run("DisabledAccountEnabled_FlipsActiveImbalance", func(t *testing.T) {
		// Follower 01B currently has 15 RELIANCE and is disabled (from previous subtest).
		// Verify initial state: hasImbalance is false.
		hasImbBefore, _ := getGroupHasImbalance(t, Group01ID)
		if hasImbBefore {
			t.Fatalf("precondition failed: group hasImbalance should be false before enabling")
		}

		// Enable Follower 01B via PATCH /api/v1/accounts/{id}
		patchResp, patchBody, err := h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{
			"enabled": true,
		})
		if err != nil || patchResp.StatusCode != http.StatusOK {
			t.Fatalf("enable follower failed: status=%d, body=%s, err=%v", patchResp.StatusCode, string(patchBody), err)
		}

		// Allow background ComputeGroupDiff to execute and update group imbalance
		var flippedToTrue bool
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(100 * time.Millisecond)
			hasImb, _ := getGroupHasImbalance(t, Group01ID)
			if hasImb {
				flippedToTrue = true
				break
			}
		}

		if !flippedToTrue {
			t.Fatalf("expected group hasImbalance to flip to true after enabling drifting follower 01b")
		}

		if detailImb := getGroupDetailHasImbalance(t, Group01ID); !detailImb {
			t.Errorf("expected group detail hasImbalance to be true, got false")
		}

		// Now rebalance Follower 01B since it is now enabled
		rebalResp, rebalBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance", Group01ID), map[string]any{
			"follower_ids": []string{Follow01BID},
		})
		if err != nil || rebalResp.StatusCode != http.StatusOK {
			t.Fatalf("rebalance enabled follower failed: status=%d, body=%s, err=%v", rebalResp.StatusCode, string(rebalBody), err)
		}

		// Group hasImbalance must now be false
		hasImbAfter, _ := getGroupHasImbalance(t, Group01ID)
		if hasImbAfter {
			t.Errorf("expected group hasImbalance = false after rebalancing newly enabled follower, got true")
		}

		// Cleanup: restore Follow01B to disabled state
		_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{"enabled": false})
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("SquareOffGroup_ClearsImbalance", func(t *testing.T) {
		fCliA := h.FollowerClient("follow01a")

		// Create rogue position on follow01a
		_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        15,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place order: %v", err)
		}

		// Trigger diff to set hasImbalance = true
		_, _, _ = h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s/positions/rebalance/diff", Group01ID), nil)

		hasImb, _ := getGroupHasImbalance(t, Group01ID)
		if !hasImb {
			t.Fatalf("expected hasImbalance = true before square-off")
		}

		// Execute full group square-off
		sqResp, sqBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		if err != nil || sqResp.StatusCode != http.StatusOK {
			t.Fatalf("square-off group failed: status=%d, body=%s, err=%v", sqResp.StatusCode, string(sqBody), err)
		}

		// Verify hasImbalance is reset to false immediately
		hasImbAfter, _ := getGroupHasImbalance(t, Group01ID)
		if hasImbAfter {
			t.Errorf("expected group hasImbalance = false immediately after full group square-off, got true")
		}
		if detailImb := getGroupDetailHasImbalance(t, Group01ID); detailImb {
			t.Errorf("expected group detail hasImbalance = false after full group square-off, got true")
		}
	})
}
