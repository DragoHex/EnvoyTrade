package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func setupTestServer() (http.Handler, *OrderEngine, *Config) {
	cfg := &Config{
		ExecutionMode: "manual",
		Users: map[string]UserConfig{
			"MASTER01": {APIKey: "key_master", APISecret: "sec_master", AccessToken: "tok_master", Role: "master"},
			"FOLLOWER01": {APIKey: "key_f1", APISecret: "sec_f1", AccessToken: "tok_f1", Role: "follower"},
		},
		Instruments: []Instrument{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", InstrumentToken: 408065, LotSize: 75, TickSize: 0.05, LTP: 25000.0},
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", InstrumentToken: 738561, LotSize: 1, TickSize: 0.05, LTP: 2450.0},
		},
		Validation: ValidationConfig{
			LotSizeCheck:        true,
			MaxQuantityPerOrder: 1800,
			CircuitLimitPct:     10.0,
			AllowedExchanges:    []string{"NFO", "NSE"},
			AllowedProducts:     []string{"NRML", "MIS", "CNC"},
			AllowedOrderTypes:   []string{"MARKET", "LIMIT"},
		},
	}
	cfg.tokenIndex = map[string]string{
		"key_master:tok_master": "MASTER01",
		"key_f1:tok_f1":         "FOLLOWER01",
	}
	eng := NewOrderEngine(cfg)
	hub := NewWSHub(cfg)
	postback := NewPostbackDispatcher("http://localhost:8080/broker-callback", cfg.Users)
	router := NewRouter(cfg, eng, hub, postback)
	return router, eng, cfg
}

func TestREST_PlaceOrder_Form(t *testing.T) {
	router, _, _ := setupTestServer()

	formData := url.Values{
		"exchange":         {"NFO"},
		"tradingsymbol":    {"NIFTY26OCTFUT"},
		"transaction_type": {"BUY"},
		"product":          {"NRML"},
		"order_type":       {"MARKET"},
		"quantity":         {"75"},
		"tag":              {"test_tag_1"},
	}

	req := httptest.NewRequest(http.MethodPost, "/orders/regular", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "token key_master:tok_master")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != "success" || resp.Data.OrderID == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestREST_PlaceOrder_JSON(t *testing.T) {
	router, _, _ := setupTestServer()

	body := `{
		"exchange": "NFO",
		"tradingsymbol": "NIFTY26OCTFUT",
		"transaction_type": "BUY",
		"product": "NRML",
		"order_type": "MARKET",
		"quantity": 150
	}`

	req := httptest.NewRequest(http.MethodPost, "/orders/regular", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token key_master:tok_master")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != "success" || resp.Data.OrderID == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestREST_PlaceOrder_AuthFailed(t *testing.T) {
	router, _, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/orders/regular", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token bad_key:bad_token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestREST_GetOrders(t *testing.T) {
	router, eng, _ := setupTestServer()

	// Place order directly in engine for MASTER01
	_, err := eng.PlaceOrder("MASTER01", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", TransactionType: "BUY",
		Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	req.Header.Set("Authorization", "token key_master:tok_master")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Status string  `json:"status"`
		Data   []Order `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != "success" || len(resp.Data) != 1 {
		t.Fatalf("expected 1 order, got %d", len(resp.Data))
	}
	if resp.Data[0].Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("expected symbol NIFTY26OCTFUT, got %s", resp.Data[0].Tradingsymbol)
	}
}

func TestREST_GetOrderHistory(t *testing.T) {
	router, eng, _ := setupTestServer()

	orderResp, _ := eng.PlaceOrder("MASTER01", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", TransactionType: "BUY",
		Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	_ = eng.FillOrder(orderResp.OrderID, 25000.0)

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderResp.OrderID, nil)
	req.Header.Set("Authorization", "token key_master:tok_master")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Status string  `json:"status"`
		Data   []Order `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Data) < 2 {
		t.Fatalf("expected at least 2 history entries, got %d", len(resp.Data))
	}
}

func TestREST_CancelOrder(t *testing.T) {
	router, eng, _ := setupTestServer()

	orderResp, _ := eng.PlaceOrder("MASTER01", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", TransactionType: "BUY",
		Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})

	req := httptest.NewRequest(http.MethodDelete, "/orders/regular/"+orderResp.OrderID, nil)
	req.Header.Set("Authorization", "token key_master:tok_master")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	order, _ := eng.GetOrder(orderResp.OrderID)
	if order.Status != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %s", order.Status)
	}
}

func TestREST_PositionsAndMargins(t *testing.T) {
	router, _, _ := setupTestServer()

	// Positions
	reqPos := httptest.NewRequest(http.MethodGet, "/portfolio/positions", nil)
	reqPos.Header.Set("Authorization", "token key_master:tok_master")
	recPos := httptest.NewRecorder()
	router.ServeHTTP(recPos, reqPos)
	if recPos.Code != http.StatusOK {
		t.Fatalf("portfolio/positions expected 200, got %d", recPos.Code)
	}

	// Margins
	reqMar := httptest.NewRequest(http.MethodGet, "/user/margins", nil)
	reqMar.Header.Set("Authorization", "token key_master:tok_master")
	recMar := httptest.NewRecorder()
	router.ServeHTTP(recMar, reqMar)
	if recMar.Code != http.StatusOK {
		t.Fatalf("user/margins expected 200, got %d", recMar.Code)
	}
}

func TestREST_SessionToken(t *testing.T) {
	router, _, _ := setupTestServer()

	formData := url.Values{
		"api_key":       {"key_master"},
		"request_token": {"any_token"},
		"checksum":      {"any_checksum"},
	}
	req := httptest.NewRequest(http.MethodPost, "/session/token", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			AccessToken string `json:"access_token"`
			UserID      string `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.AccessToken != "tok_master" || resp.Data.UserID != "MASTER01" {
		t.Errorf("unexpected session data: %+v", resp.Data)
	}
}

func TestREST_QuoteLTP(t *testing.T) {
	router, _, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/quote/ltp?i=NFO:NIFTY26OCTFUT", nil)
	req.Header.Set("Authorization", "token key_master:tok_master")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Status string `json:"status"`
		Data   map[string]struct {
			InstrumentToken int     `json:"instrument_token"`
			LastPrice       float64 `json:"last_price"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	q, ok := resp.Data["NFO:NIFTY26OCTFUT"]
	if !ok || q.LastPrice != 25000.0 {
		t.Errorf("expected 25000.0, got %+v", q)
	}
}

func TestREST_AdminAPI_UsersAndOrders(t *testing.T) {
	router, eng, _ := setupTestServer()

	// 1. GET /api/users
	reqUsers := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	recUsers := httptest.NewRecorder()
	router.ServeHTTP(recUsers, reqUsers)
	if recUsers.Code != http.StatusOK {
		t.Fatalf("/api/users expected 200, got %d", recUsers.Code)
	}

	// 2. Place an order via /api/quick-order
	quickBody := `{
		"user_id": "MASTER01",
		"exchange": "NFO",
		"tradingsymbol": "NIFTY26OCTFUT",
		"transaction_type": "BUY",
		"product": "NRML",
		"order_type": "MARKET",
		"quantity": 75
	}`
	reqQuick := httptest.NewRequest(http.MethodPost, "/api/quick-order", strings.NewReader(quickBody))
	reqQuick.Header.Set("Content-Type", "application/json")
	recQuick := httptest.NewRecorder()
	router.ServeHTTP(recQuick, reqQuick)
	if recQuick.Code != http.StatusOK {
		t.Fatalf("/api/quick-order expected 200, got %d: %s", recQuick.Code, recQuick.Body.String())
	}

	var qResp struct {
		Status string `json:"status"`
		Data   struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recQuick.Body.Bytes(), &qResp)
	orderID := qResp.Data.OrderID

	// 3. GET /api/orders
	reqOrders := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	recOrders := httptest.NewRecorder()
	router.ServeHTTP(recOrders, reqOrders)
	if recOrders.Code != http.StatusOK {
		t.Fatalf("/api/orders expected 200, got %d", recOrders.Code)
	}

	// 4. POST /api/orders/:order_id/fill
	fillBody := `{"price": 25050.0}`
	reqFill := httptest.NewRequest(http.MethodPost, "/api/orders/"+orderID+"/fill", strings.NewReader(fillBody))
	reqFill.Header.Set("Content-Type", "application/json")
	recFill := httptest.NewRecorder()
	router.ServeHTTP(recFill, reqFill)
	if recFill.Code != http.StatusOK {
		t.Fatalf("/api/orders/fill expected 200, got %d", recFill.Code)
	}

	order, _ := eng.GetOrder(orderID)
	if order.Status != "COMPLETE" || order.AveragePrice != 25050.0 {
		t.Errorf("fill did not apply correctly: %+v", order)
	}

	// 5. POST /api/instruments/:exchange/:symbol/ltp
	ltpBody := `{"ltp": 25200.0}`
	reqLTP := httptest.NewRequest(http.MethodPost, "/api/instruments/NFO/NIFTY26OCTFUT/ltp", strings.NewReader(ltpBody))
	reqLTP.Header.Set("Content-Type", "application/json")
	recLTP := httptest.NewRecorder()
	router.ServeHTTP(recLTP, reqLTP)
	if recLTP.Code != http.StatusOK {
		t.Fatalf("/api/instruments/ltp expected 200, got %d", recLTP.Code)
	}

	inst, _ := eng.GetInstrument("NFO", "NIFTY26OCTFUT")
	if inst.LTP != 25200.0 {
		t.Errorf("expected updated LTP 25200.0, got %f", inst.LTP)
	}

	// 6. Execution mode get/set
	reqModeSet := httptest.NewRequest(http.MethodPost, "/api/mode", strings.NewReader(`{"mode":"delayed"}`))
	reqModeSet.Header.Set("Content-Type", "application/json")
	recModeSet := httptest.NewRecorder()
	router.ServeHTTP(recModeSet, reqModeSet)
	if recModeSet.Code != http.StatusOK {
		t.Fatalf("set mode expected 200, got %d", recModeSet.Code)
	}

	reqModeGet := httptest.NewRequest(http.MethodGet, "/api/mode", nil)
	recModeGet := httptest.NewRecorder()
	router.ServeHTTP(recModeGet, reqModeGet)
	if !strings.Contains(recModeGet.Body.String(), "delayed") {
		t.Errorf("expected mode delayed, got %s", recModeGet.Body.String())
	}
}

func TestREST_GetPositions_TotalMTM_Calculation(t *testing.T) {
	router, eng, _ := setupTestServer()
	eng.config.ExecutionMode = "instant"

	// 1. Master buys 150 NIFTY @ 25000
	_, err := eng.PlaceOrder("MASTER01", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 150,
	})
	if err != nil {
		t.Fatalf("place order master: %v", err)
	}

	// 2. Follower buys 75 NIFTY @ 25000
	_, err = eng.PlaceOrder("FOLLOWER01", "regular", OrderParams{
		Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT",
		TransactionType: "BUY", Product: "NRML", OrderType: "MARKET", Quantity: 75,
	})
	if err != nil {
		t.Fatalf("place order follower: %v", err)
	}

	// 3. Update LTP: NIFTY rises to 25150 (+150 pts)
	if err := eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 25150.0); err != nil {
		t.Fatalf("update LTP: %v", err)
	}

	// 4. Master sells short 10 RELIANCE @ 2450
	_, err = eng.PlaceOrder("MASTER01", "regular", OrderParams{
		Exchange: "NSE", Tradingsymbol: "RELIANCE",
		TransactionType: "SELL", Product: "MIS", OrderType: "MARKET", Quantity: 10,
	})
	if err != nil {
		t.Fatalf("place short order master: %v", err)
	}

	// 5. Update LTP: RELIANCE falls to 2420 (+30 pts gain on short)
	if err := eng.UpdateLTP("NSE", "RELIANCE", 2420.0); err != nil {
		t.Fatalf("update LTP RELIANCE: %v", err)
	}

	type posResp struct {
		Status string `json:"status"`
		Data   struct {
			Net []Position `json:"net"`
		} `json:"data"`
	}

	// Helper to fetch positions via REST
	getPositionsViaREST := func(token string) []Position {
		req := httptest.NewRequest(http.MethodGet, "/portfolio/positions", nil)
		req.Header.Set("Authorization", "token "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /portfolio/positions failed with status %d: %s", rec.Code, rec.Body.String())
		}
		var resp posResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal positions: %v", err)
		}
		return resp.Data.Net
	}

	// Calculate total MTM from positions
	calcTotalMtm := func(positions []Position) float64 {
		total := 0.0
		for _, p := range positions {
			total += p.M2M
		}
		return total
	}

	// Validate Scenario A: Bullish NIFTY, Bearish RELIANCE
	masterPositions := getPositionsViaREST("key_master:tok_master")
	if len(masterPositions) != 2 {
		t.Fatalf("expected 2 positions for master, got %d", len(masterPositions))
	}
	masterTotalMTM := calcTotalMtm(masterPositions)
	// Expected Master: 150 * (+150) + 10 * (+30) = 22500 + 300 = 22800.0
	expectedMasterMTM := 22800.0
	if masterTotalMTM != expectedMasterMTM {
		t.Errorf("Master Total MTM = %f, want %f", masterTotalMTM, expectedMasterMTM)
	}

	followerPositions := getPositionsViaREST("key_f1:tok_f1")
	if len(followerPositions) != 1 {
		t.Fatalf("expected 1 position for follower, got %d", len(followerPositions))
	}
	followerTotalMTM := calcTotalMtm(followerPositions)
	// Expected Follower: 75 * (+150) = 11250.0
	expectedFollowerMTM := 11250.0
	if followerTotalMTM != expectedFollowerMTM {
		t.Errorf("Follower Total MTM = %f, want %f", followerTotalMTM, expectedFollowerMTM)
	}

	// Validate Scenario B: Adverse price movements
	// NIFTY drops to 24900 (-100 pts from entry 25000)
	// RELIANCE rises to 2500 (-50 pts against short entry 2450)
	_ = eng.UpdateLTP("NFO", "NIFTY26OCTFUT", 24900.0)
	_ = eng.UpdateLTP("NSE", "RELIANCE", 2500.0)

	masterPositionsB := getPositionsViaREST("key_master:tok_master")
	masterTotalMTMB := calcTotalMtm(masterPositionsB)
	// Expected Master: 150 * (-100) + 10 * (-50) = -15000 + -500 = -15500.0
	expectedMasterMTMB := -15500.0
	if masterTotalMTMB != expectedMasterMTMB {
		t.Errorf("Adverse Master Total MTM = %f, want %f", masterTotalMTMB, expectedMasterMTMB)
	}

	followerPositionsB := getPositionsViaREST("key_f1:tok_f1")
	followerTotalMTMB := calcTotalMtm(followerPositionsB)
	// Expected Follower: 75 * (-100) = -7500.0
	expectedFollowerMTMB := -7500.0
	if followerTotalMTMB != expectedFollowerMTMB {
		t.Errorf("Adverse Follower Total MTM = %f, want %f", followerTotalMTMB, expectedFollowerMTMB)
	}
}

