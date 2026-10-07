//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"testbroker/sdk"
)

func TestE2E_SquareOff(t *testing.T) {
	h := NewHarness(t)

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	t.Run("Follower_SquareOff_SingleAccount_AllPositions", func(t *testing.T) {
		// 1. Follower 01A buys 75 NIFTY26OCTFUT
		fCliA := h.FollowerClient("follow01a")
		_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        75,
			Product:         "NRML",
		})
		if err != nil {
			t.Fatalf("follower 01a place buy order: %v", err)
		}

		// Verify follower 01A has positive net position
		pos, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("follower 01a get positions: %v", err)
		}
		var foundNifty bool
		for _, p := range pos.Net {
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity > 0 {
				foundNifty = true
				break
			}
		}
		if !foundNifty {
			t.Fatalf("expected follower 01a to have open NIFTY26OCTFUT position, got %+v", pos.Net)
		}

		// Also place an order for Master 01 so we can verify Master is NOT touched
		mCli1 := h.MasterClient("master01")
		_, err = mCli1.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        10,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("master 01 place order: %v", err)
		}

		// 2. Call Square-Off for follower 01A
		resp, body, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/square-off", Follow01AID), map[string]any{})
		if err != nil {
			t.Fatalf("square-off request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
		}

		var res map[string]any
		if err := json.Unmarshal(body, &res); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if res["status"] != "completed" {
			t.Errorf("expected status 'completed', got %v", res["status"])
		}

		// 3. Verify Follower 01A position is flattened to 0
		posAfter, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("follower 01a get positions after: %v", err)
		}
		for _, p := range posAfter.Net {
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity != 0 {
				t.Errorf("expected NIFTY26OCTFUT net quantity 0, got %d", p.Quantity)
			}
		}

		// 4. Verify Master 01 positions are untouched
		mPos, err := mCli1.GetPositions()
		if err != nil {
			t.Fatalf("master 01 get positions: %v", err)
		}
		var masterRelianceFound bool
		for _, p := range mPos.Net {
			if p.Tradingsymbol == "RELIANCE" && p.Quantity == 10 {
				masterRelianceFound = true
				break
			}
		}
		if !masterRelianceFound {
			t.Errorf("expected master 01 RELIANCE position to remain open (qty 10)")
		}
	})

	t.Run("Follower_SquareOff_SelectiveSymbols", func(t *testing.T) {
		fCliA := h.FollowerClient("follow01a")
		// Buy NIFTY26OCTFUT and RELIANCE
		_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        75,
			Product:         "NRML",
		})
		if err != nil {
			t.Fatalf("place buy nifty: %v", err)
		}
		_, err = fCliA.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        15,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place buy reliance: %v", err)
		}

		// Square-off only RELIANCE
		reqBody := map[string]any{
			"symbols": []string{"RELIANCE"},
		}
		resp, body, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/square-off", Follow01AID), reqBody)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("square-off selective failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
		}

		pos, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("get positions: %v", err)
		}
		for _, p := range pos.Net {
			if p.Tradingsymbol == "RELIANCE" && p.Quantity != 0 {
				t.Errorf("expected RELIANCE to be 0, got %d", p.Quantity)
			}
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity <= 0 {
				t.Errorf("expected NIFTY26OCTFUT to still be open, got %d", p.Quantity)
			}
		}

		// Flatten follower 01A before next test
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/square-off", Follow01AID), map[string]any{})
	})

	t.Run("Group_SquareOff_MasterCascadesToFollowers", func(t *testing.T) {
		mCli1 := h.MasterClient("master01")
		fCliA := h.FollowerClient("follow01a")

		// First ensure group starts completely flat
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		time.Sleep(100 * time.Millisecond)

		ordersBefore, _ := fCliA.GetOrders()
		prevOrderCount := len(ordersBefore)

		// 1. Place position on Master01
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

		// Wait specifically for follower 01A to receive the newly fanned out BUY order
		var fReceivedNewOrder bool
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			currentOrders, err := fCliA.GetOrders()
			if err == nil && len(currentOrders) > prevOrderCount {
				for _, o := range currentOrders {
					if o.Tradingsymbol == "NIFTY26OCTFUT" && o.TransactionType == "BUY" && o.Status == "COMPLETE" {
						fReceivedNewOrder = true
						break
					}
				}
			}
			if fReceivedNewOrder {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !fReceivedNewOrder {
			t.Fatalf("timed out waiting for follower 01a to receive master fan-out order")
		}

		// 2. Execute Group Square-Off
		resp, body, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("group square-off failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
		}

		var res map[string]any
		if err := json.Unmarshal(body, &res); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if res["status"] != "completed" {
			t.Errorf("expected group status completed, got %v", res["status"])
		}

		// Verify Master01 has 0 net position for NIFTY26OCTFUT
		mPos, err := mCli1.GetPositions()
		if err != nil {
			t.Fatalf("get master positions: %v", err)
		}
		for _, p := range mPos.Net {
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity != 0 {
				t.Errorf("expected Master NIFTY26OCTFUT to be 0, got %d", p.Quantity)
			}
		}

		// Verify Follower01A has 0 net position for NIFTY26OCTFUT
		fPos, err := fCliA.GetPositions()
		if err != nil {
			t.Fatalf("get follower positions: %v", err)
		}
		for _, p := range fPos.Net {
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity != 0 {
				t.Errorf("expected Follower NIFTY26OCTFUT to be 0, got %d", p.Quantity)
			}
		}

		// Sleep briefly to ensure any postbacks from master squareoff order have been processed
		// and DID NOT trigger duplicate follower orders
		time.Sleep(500 * time.Millisecond)

		fPosAfter, _ := fCliA.GetPositions()
		for _, p := range fPosAfter.Net {
			if p.Tradingsymbol == "NIFTY26OCTFUT" && p.Quantity != 0 {
				t.Errorf("follower got corrupted/flipped into position: %d after master postback", p.Quantity)
			}
		}
	})

	t.Run("Group_SquareOff_SkipsDisabledFollower", func(t *testing.T) {
		mCli1 := h.MasterClient("master01")
		fCliB := h.FollowerClient("follow01b") // disabled follower

		// Ensure follow01b is disabled
		_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{"enabled": false})

		// 1. Follower 01B opens a direct position: BUY 50 RELIANCE
		_, err := fCliB.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        50,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place follower buy order: %v", err)
		}

		// Master opens a position
		_, err = mCli1.PlaceOrder("regular", sdk.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			Quantity:        50,
			Product:         "CNC",
		})
		if err != nil {
			t.Fatalf("place master buy order: %v", err)
		}

		// 2. Direct account square-off on disabled follower must return HTTP 400
		resp, _, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/square-off", Follow01BID), map[string]any{})
		if err != nil {
			t.Fatalf("direct square-off call failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request for disabled follower square-off, got %d", resp.StatusCode)
		}

		// 3. Group square-off must NOT square off Follower 01B
		gResp, gBody, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/groups/%s/positions/square-off", Group01ID), map[string]any{})
		if err != nil || gResp.StatusCode != http.StatusOK {
			t.Fatalf("group square-off failed: status=%d, body=%s, err=%v", gResp.StatusCode, string(gBody), err)
		}

		// Follower 01B must still have open position
		bPos, err := fCliB.GetPositions()
		if err != nil {
			t.Fatalf("get follower positions: %v", err)
		}
		var fBStillOpen bool
		for _, p := range bPos.Net {
			if p.Tradingsymbol == "RELIANCE" && p.Quantity == 50 {
				fBStillOpen = true
				break
			}
		}
		if !fBStillOpen {
			t.Errorf("expected disabled follower 01B to retain open position, got %+v", bPos.Net)
		}

		// Cleanup: temporarily re-enable follow01b and square off
		_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{"enabled": true})
		_, _, _ = h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/positions/square-off", Follow01BID), map[string]any{})
		_, _, _ = h.EnvoyAPI(http.MethodPatch, "/api/v1/accounts/"+Follow01BID, map[string]any{"enabled": false})
	})
}

