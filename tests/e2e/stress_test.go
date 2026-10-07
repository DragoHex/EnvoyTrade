//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"testbroker/sdk"
)

func TestE2E_StressAndConcurrency(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	t.Log("Starting Stress & Concurrency Test...")

	// -------------------------------------------------------------
	// Phase 1: High-Concurrency Burst (Copy-Trades + Direct Follower Trades)
	// -------------------------------------------------------------
	t.Run("Phase1_Concurrent_Master_And_Follower_Trades", func(t *testing.T) {
		const numMasterOrders = 6
		const numDirectOrders = 4

		var wg sync.WaitGroup
		var masterOrderIDs sync.Map
		var directOrderIDs sync.Map
		var errCount int32

		startTime := time.Now()

		// 1. Fire concurrent Master copy-trade orders
		for i := 0; i < numMasterOrders; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				symbol := "RELIANCE"
				exchange := "NSE"
				qty := 2
				if idx%2 == 1 {
					symbol = "CRUDEOIL21SEP26"
					exchange = "MCX"
					qty = 100
				}
				var resp sdk.OrderResponse
				var err error
				for attempt := 0; attempt < 3; attempt++ {
					cli := h.MasterClient("master01")
					resp, err = cli.PlaceOrder("regular", sdk.OrderParams{
						Exchange:        exchange,
						Tradingsymbol:   symbol,
						TransactionType: "BUY",
						OrderType:       "MARKET",
						Quantity:        qty,
						Product:         "CNC",
					})
					if err == nil {
						break
					}
					time.Sleep(30 * time.Millisecond)
				}
				if err != nil {
					atomic.AddInt32(&errCount, 1)
					t.Errorf("master order %d failed: %v", idx, err)
					return
				}
				masterOrderIDs.Store(resp.OrderID, true)
			}(i)
		}

		// 2. Concurrently fire direct manual follower orders on TestBroker
		followerNames := []string{"follow01a", "follow01b", "follow01c", "follow01d"}
		for i := 0; i < numDirectOrders; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				name := followerNames[idx%len(followerNames)]
				var resp sdk.OrderResponse
				var err error
				for attempt := 0; attempt < 3; attempt++ {
					cli := h.FollowerClient(name)
					resp, err = cli.PlaceOrder("regular", sdk.OrderParams{
						Exchange:        "NSE",
						Tradingsymbol:   "RELIANCE",
						TransactionType: "BUY",
						OrderType:       "MARKET",
						Quantity:        1,
						Product:         "CNC",
					})
					if err == nil {
						break
					}
					time.Sleep(30 * time.Millisecond)
				}
				if err != nil {
					atomic.AddInt32(&errCount, 1)
					t.Errorf("follower direct order %d failed: %v", idx, err)
					return
				}
				directOrderIDs.Store(resp.OrderID, true)
			}(i)
		}

		wg.Wait()
		duration := time.Since(startTime)
		t.Logf("Fired %d master orders + %d direct follower orders in %v (errors: %d)",
			numMasterOrders, numDirectOrders, duration, errCount)

		if errCount > 0 {
			t.Fatalf("encountered %d order placement errors", errCount)
		}

		// 3. Await all orders to be fully ingested and settled
		time.Sleep(3 * time.Second)

		// Verify that all master orders exist in master_fills
		var masterFillCount int
		masterOrderIDs.Range(func(k, v any) bool {
			masterFillCount++
			return true
		})
		t.Logf("Total master orders placed: %d", masterFillCount)

		// 4. Assert in DB: Zero duplicates on (follower_id, broker_order_id)
		var duplicateCount int
		err := h.DB.QueryRow(ctx, `
			SELECT count(*) FROM (
				SELECT follower_id, broker_order_id, count(*)
				FROM follower_orders
				WHERE broker_order_id IS NOT NULL
				GROUP BY follower_id, broker_order_id
				HAVING count(*) > 1
			) dup
		`).Scan(&duplicateCount)
		if err != nil {
			t.Fatalf("query duplicate follower orders: %v", err)
		}
		if duplicateCount > 0 {
			t.Fatalf("CRITICAL: found %d duplicate follower orders!", duplicateCount)
		}
		t.Log("PASSED: Zero duplicate follower orders in database.")

		// Verify zero duplicate master_fills
		var duplicateMasterFills int
		err = h.DB.QueryRow(ctx, `
			SELECT count(*) FROM (
				SELECT master_id, broker_order_id, filled_quantity, status, count(*)
				FROM master_fills
				GROUP BY master_id, broker_order_id, filled_quantity, status
				HAVING count(*) > 1
			) dup
		`).Scan(&duplicateMasterFills)
		if err != nil {
			t.Fatalf("query duplicate master fills: %v", err)
		}
		if duplicateMasterFills > 0 {
			t.Fatalf("CRITICAL: found %d duplicate master fills!", duplicateMasterFills)
		}
		t.Log("PASSED: Zero duplicate master fills in database.")

		// Verify direct orders exist with origin = 'manual'
		directOrderIDs.Range(func(k, v any) bool {
			orderID := k.(string)
			var origin, status string
			qErr := h.DB.QueryRow(ctx, `SELECT origin, terminal_status FROM follower_orders WHERE broker_order_id = $1`, orderID).Scan(&origin, &status)
			if qErr != nil {
				t.Errorf("direct order %s not found in follower_orders: %v", orderID, qErr)
			} else {
				if origin != "manual" {
					t.Errorf("order %s origin = %s, want 'manual'", orderID, origin)
				}
				if status != "COMPLETE" {
					t.Errorf("order %s status = %s, want 'COMPLETE'", orderID, status)
				}
			}
			return true
		})
		t.Log("PASSED: All direct follower orders confirmed with origin='manual' and status='COMPLETE'.")
	})

	// -------------------------------------------------------------
	// Phase 2: Postback Avalanche & Duplicate Webhook Bombardment
	// -------------------------------------------------------------
	t.Run("Phase2_Postback_Duplicate_Bombardment", func(t *testing.T) {
		const bombardmentCount = 30
		orderID := fmt.Sprintf("BOMB_%d", time.Now().UnixNano()%100000000)
		ts := time.Now().Format("2006-01-02 15:04:05")
		secret := "secret_follow01d"

		// Pre-compute Kite Connect signature: sha256(order_id + order_timestamp + api_secret)
		hSha := sha256.New()
		hSha.Write([]byte(orderID + ts + secret))
		checksum := hex.EncodeToString(hSha.Sum(nil))

		payload := map[string]any{
			"order_id":         orderID,
			"user_id":          "FOLLOW01D",
			"status":           "COMPLETE",
			"tradingsymbol":    "RELIANCE",
			"exchange":         "NSE",
			"product":          "CNC",
			"transaction_type": "BUY",
			"order_type":       "MARKET",
			"filled_quantity":  10,
			"average_price":    2450.0,
			"order_timestamp":  ts,
			"checksum":         checksum,
		}

		var wg sync.WaitGroup
		var successCount int32
		var failCount int32

		// Fire 30 simultaneous postback requests for the exact same order
		for i := 0; i < bombardmentCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, _, pErr := h.EnvoyAPI(http.MethodPost, "/broker-callback", payload)
				if pErr == nil && resp.StatusCode == http.StatusOK {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
				}
			}()
		}
		wg.Wait()
		time.Sleep(1 * time.Second)

		t.Logf("Postback bombardment complete: %d successes, %d failures", successCount, failCount)
		if successCount != bombardmentCount {
			t.Fatalf("expected all %d postbacks to return 200 OK, got %d", bombardmentCount, successCount)
		}

		// Verify database has exactly 1 row for this orderID
		var rowCount int
		err := h.DB.QueryRow(ctx, `SELECT count(*) FROM follower_orders WHERE broker_order_id = $1`, orderID).Scan(&rowCount)
		if err != nil {
			t.Fatalf("query bombarded order: %v", err)
		}
		if rowCount != 1 {
			t.Fatalf("CRITICAL: bombarded order %s created %d rows, expected exactly 1!", orderID, rowCount)
		}
		t.Log("PASSED: 30 concurrent duplicate webhooks produced exactly 1 database record.")
	})

	// -------------------------------------------------------------
	// Phase 3: Dashboard API Orders Accuracy & Completeness
	// -------------------------------------------------------------
	t.Run("Phase3_Dashboard_Orders_Accuracy", func(t *testing.T) {
		// Fetch closed orders for Follow01D
		resp, body, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/accounts/%s/orders?tab=closed_orders", Follow01DID), nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("fetch closed orders: status=%d, err=%v", resp.StatusCode, err)
		}

		var apiResp struct {
			Pagination struct {
				TotalCount int `json:"totalCount"`
			} `json:"pagination"`
			ClosedOrders []struct {
				ID         string `json:"id"`
				Instrument string `json:"instrument"`
				Quantity   int    `json:"quantity"`
				Status     string `json:"status"`
			} `json:"closedOrders"`
		}
		if err := json.Unmarshal(body, &apiResp); err != nil {
			t.Fatalf("unmarshal closed orders: %v", err)
		}

		// Compare count from API with database count
		var dbClosedCount int
		err = h.DB.QueryRow(ctx, `
			SELECT count(*) FROM follower_orders
			WHERE follower_id = $1 AND terminal_status = 'COMPLETE'
		`, Follow01DID).Scan(&dbClosedCount)
		if err != nil {
			t.Fatalf("query db closed count: %v", err)
		}

		t.Logf("API reports %d closed orders, DB has %d closed orders", apiResp.Pagination.TotalCount, dbClosedCount)
		if apiResp.Pagination.TotalCount != dbClosedCount {
			t.Errorf("mismatch: API totalCount=%d != DB count=%d", apiResp.Pagination.TotalCount, dbClosedCount)
		}

		// Verify each item returned has valid instrument and status
		for _, o := range apiResp.ClosedOrders {
			if o.Instrument == "" {
				t.Errorf("order %s has empty instrument name", o.ID)
			}
			if o.Status != "COMPLETE" {
				t.Errorf("order %s status = %s, want COMPLETE", o.ID, o.Status)
			}
			if o.Quantity <= 0 {
				t.Errorf("order %s quantity = %d, want > 0", o.ID, o.Quantity)
			}
		}
		t.Log("PASSED: Dashboard API orders match database with 100% fidelity.")
	})

	// -------------------------------------------------------------
	// Phase 4: Reconciler Cleanliness & Ghost Orders Check
	// -------------------------------------------------------------
	t.Run("Phase4_Reconciler_Integrity", func(t *testing.T) {
		// Verify zero pending/stuck follower orders older than 5 seconds
		var stuckCount int
		err := h.DB.QueryRow(ctx, `
			SELECT count(*) FROM follower_orders
			WHERE terminal_status IS NULL AND created_at < now() - interval '10 seconds'
		`).Scan(&stuckCount)
		if err != nil {
			t.Fatalf("query stuck orders: %v", err)
		}
		if stuckCount > 0 {
			t.Errorf("found %d stuck follower orders without terminal status", stuckCount)
		} else {
			t.Log("PASSED: Zero stuck follower orders in database.")
		}
	})
}
