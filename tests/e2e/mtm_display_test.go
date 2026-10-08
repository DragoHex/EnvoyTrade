//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"testbroker/sdk"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

func TestMTMDisplay_BrokerMarginsAndBreakdown(t *testing.T) {
	h := NewHarness(t)

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	// 1. Follower 01A places MIS order (10 RELIANCE) and NRML order (75 NIFTY26OCTFUT)
	fCliA := h.FollowerClient("follow01a")

	_, err := fCliA.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        10,
		Product:         "MIS",
	})
	if err != nil {
		t.Fatalf("follower 01a place RELIANCE MIS: %v", err)
	}

	_, err = fCliA.PlaceOrder("regular", sdk.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        75,
		Product:         "NRML",
	})
	if err != nil {
		t.Fatalf("follower 01a place NIFTY NRML: %v", err)
	}

	// 2. Update LTP for both to create known distinct MTM values
	// RELIANCE base is 2500 -> move to 2550 (+50 * 10 = +500 MTM)
	if err := h.AdminSDK.SetLTP("NSE", "RELIANCE", 2550.0); err != nil {
		t.Fatalf("set RELIANCE LTP: %v", err)
	}

	// NIFTY base is 25000 -> move to 25020 (+20 * 75 = +1500 MTM)
	if err := h.AdminSDK.SetLTP("NFO", "NIFTY26OCTFUT", 25020.0); err != nil {
		t.Fatalf("set NIFTY LTP: %v", err)
	}

	// Verify testbroker's /user/margins has utilised M2M = 2500 (1000 + 1500)
	mar, err := fCliA.GetUserMargins()
	if err != nil {
		t.Fatalf("get follower margins: %v", err)
	}
	realised, _ := mar.Equity.Utilised["m2m_realised"].(float64)
	unrealised, _ := mar.Equity.Utilised["m2m_unrealised"].(float64)
	expectedTotal := realised + unrealised
	if expectedTotal != 2500.0 {
		t.Fatalf("testbroker utilised MTM = %f, want 2500.0", expectedTotal)
	}

	// 3. Trigger portfolio sync for follower 01A
	resp, _, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/actions", Follow01AID), map[string]string{
		"type": "sync_positions",
	})
	if err != nil || (resp != nil && resp.StatusCode != http.StatusAccepted) {
		t.Fatalf("trigger sync failed: resp=%v err=%v", resp, err)
	}

	// Allow brief time for async processing if any
	time.Sleep(300 * time.Millisecond)

	// 4. Query /api/v1/accounts/:id/orders
	resp, body, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/accounts/%s/orders", Follow01AID), nil)
	if err != nil {
		t.Fatalf("get orders error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get orders status = %d, want 200: %s", resp.StatusCode, string(body))
	}

	var ordersDetail struct {
		Summary struct {
			TotalMtm     string            `json:"totalMtm"`
			MtmBreakdown map[string]string `json:"mtmBreakdown"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(body, &ordersDetail); err != nil {
		t.Fatalf("unmarshal ordersDetail: %v", err)
	}

	if ordersDetail.Summary.TotalMtm != "2500" && ordersDetail.Summary.TotalMtm != "2500.00" && ordersDetail.Summary.TotalMtm != "2500.0000" {
		t.Errorf("orders summary totalMtm = %s, want 2500", ordersDetail.Summary.TotalMtm)
	}
	if ordersDetail.Summary.MtmBreakdown["MIS"] != "1000" && ordersDetail.Summary.MtmBreakdown["MIS"] != "1000.00" && ordersDetail.Summary.MtmBreakdown["MIS"] != "1000.0000" {
		t.Errorf("orders summary mtmBreakdown[MIS] = %s, want 1000", ordersDetail.Summary.MtmBreakdown["MIS"])
	}
	if ordersDetail.Summary.MtmBreakdown["NRML"] != "1500" && ordersDetail.Summary.MtmBreakdown["NRML"] != "1500.00" && ordersDetail.Summary.MtmBreakdown["NRML"] != "1500.0000" {
		t.Errorf("orders summary mtmBreakdown[NRML] = %s, want 1500", ordersDetail.Summary.MtmBreakdown["NRML"])
	}

	// 5. Query /api/v1/groups/:id and verify follower row breakdown
	resp, body, err = h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/groups/%s", Group01ID), nil)
	if err != nil {
		t.Fatalf("get group detail error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get group detail status = %d, want 200: %s", resp.StatusCode, string(body))
	}

	var groupDetail struct {
		Followers []struct {
			AccountID    string            `json:"accountId"`
			TotalMtm     string            `json:"totalMtm"`
			MtmBreakdown map[string]string `json:"mtmBreakdown"`
		} `json:"followers"`
	}
	if err := json.Unmarshal(body, &groupDetail); err != nil {
		t.Fatalf("unmarshal groupDetail: %v", err)
	}

	var foundFollower bool
	for _, f := range groupDetail.Followers {
		if f.AccountID == Follow01AID {
			foundFollower = true
			if f.TotalMtm != "2500" && f.TotalMtm != "2500.00" && f.TotalMtm != "2500.0000" {
				t.Errorf("group detail follower totalMtm = %s, want 2500", f.TotalMtm)
			}
			if f.MtmBreakdown["MIS"] != "1000" && f.MtmBreakdown["MIS"] != "1000.00" && f.MtmBreakdown["MIS"] != "1000.0000" {
				t.Errorf("group detail follower mtmBreakdown[MIS] = %s, want 1000", f.MtmBreakdown["MIS"])
			}
			if f.MtmBreakdown["NRML"] != "1500" && f.MtmBreakdown["NRML"] != "1500.00" && f.MtmBreakdown["NRML"] != "1500.0000" {
				t.Errorf("group detail follower mtmBreakdown[NRML] = %s, want 1500", f.MtmBreakdown["NRML"])
			}
			break
		}
	}
	if !foundFollower {
		t.Errorf("follower %s not found in group detail", Follow01AID)
	}
}

func TestRealizedM2M_BrokerSourceOfTruth(t *testing.T) {
	h := NewHarness(t)

	// Ensure execution mode is instant
	if err := h.AdminSDK.SetExecutionMode("instant"); err != nil {
		t.Fatalf("set execution mode: %v", err)
	}

	// Use official gokiteconnect Client pointing to TestBroker for Follower 01B
	kcClient := h.KiteConnectClient("follow01b")

	// 1. Follower 01B places a BUY order for 10 RELIANCE (MIS) at 2450.0
	if err := h.AdminSDK.SetLTP("NSE", "RELIANCE", 2450.0); err != nil {
		t.Fatalf("set RELIANCE LTP: %v", err)
	}

	_, err := kcClient.PlaceOrder("regular", kiteconnect.OrderParams{
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		TransactionType: "BUY",
		OrderType:       "MARKET",
		Quantity:        10,
		Product:         "MIS",
	})
	if err != nil {
		t.Fatalf("kcClient place BUY order: %v", err)
	}

	// 2. Set LTP to 2550.0 and close the position via SELL order (+100 * 10 = +1000 realized profit)
	if err := h.AdminSDK.SetLTP("NSE", "RELIANCE", 2550.0); err != nil {
		t.Fatalf("set RELIANCE LTP: %v", err)
	}

	_, err = kcClient.PlaceOrder("regular", kiteconnect.OrderParams{
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		TransactionType: "SELL",
		OrderType:       "MARKET",
		Quantity:        10,
		Product:         "MIS",
	})
	if err != nil {
		t.Fatalf("kcClient place SELL order: %v", err)
	}

	// 3. Fetch margins directly using gokiteconnect Client: GetUserMargins()
	margins, err := kcClient.GetUserMargins()
	if err != nil {
		t.Fatalf("kcClient.GetUserMargins(): %v", err)
	}

	// Verify gokiteconnect retrieved utilised.m2m_realised directly from broker API
	if margins.Equity.Used.M2MRealised != 1000.0 {
		t.Fatalf("gokiteconnect margins.Equity.Used.M2MRealised = %f, want 1000.0", margins.Equity.Used.M2MRealised)
	}

	// Also verify segment margins via gokiteconnect GetUserSegmentMargins("equity")
	segMargin, err := kcClient.GetUserSegmentMargins("equity")
	if err != nil {
		t.Fatalf("kcClient.GetUserSegmentMargins(\"equity\"): %v", err)
	}
	if segMargin.Used.M2MRealised != 1000.0 {
		t.Fatalf("gokiteconnect segMargin.Used.M2MRealised = %f, want 1000.0", segMargin.Used.M2MRealised)
	}

	// 4. Trigger EnvoyTrade portfolio sync for Follower 01B
	resp, _, err := h.EnvoyAPI(http.MethodPost, fmt.Sprintf("/api/v1/accounts/%s/actions", Follow01BID), map[string]string{
		"type": "sync_positions",
	})
	if err != nil || (resp != nil && resp.StatusCode != http.StatusAccepted) {
		t.Fatalf("trigger sync failed: resp=%v err=%v", resp, err)
	}

	time.Sleep(300 * time.Millisecond)

	// 5. Query /api/v1/accounts/:id/orders and assert RealizedPnl is sourced from broker M2M Realised
	resp, body, err := h.EnvoyAPI(http.MethodGet, fmt.Sprintf("/api/v1/accounts/%s/orders", Follow01BID), nil)
	if err != nil {
		t.Fatalf("get orders error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get orders status = %d, want 200: %s", resp.StatusCode, string(body))
	}

	var ordersDetail struct {
		Summary struct {
			RealizedPnl string `json:"realizedPnl"`
			TotalMtm    string `json:"totalMtm"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(body, &ordersDetail); err != nil {
		t.Fatalf("unmarshal ordersDetail: %v", err)
	}

	if ordersDetail.Summary.RealizedPnl != "1000" && ordersDetail.Summary.RealizedPnl != "1000.00" && ordersDetail.Summary.RealizedPnl != "1000.0000" {
		t.Errorf("orders summary realizedPnl = %s, want 1000", ordersDetail.Summary.RealizedPnl)
	}
}
