package sdk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdmin_ExecutionMode(t *testing.T) {
	currentMode := "instant"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mode" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if r.Method == http.MethodPost {
			var body struct {
				Mode string `json:"mode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			currentMode = body.Mode
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]string{"mode": currentMode},
		})
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetBaseURI(ts.URL)

	// Set mode
	if err := c.SetExecutionMode("manual"); err != nil {
		t.Fatalf("SetExecutionMode failed: %v", err)
	}

	// Get mode
	mode, err := c.GetExecutionMode()
	if err != nil {
		t.Fatalf("GetExecutionMode failed: %v", err)
	}
	if mode != "manual" {
		t.Errorf("mode = %s, want manual", mode)
	}
}

func TestAdmin_SetLTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/instruments/NFO/NIFTY26OCTFUT/ltp" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var body struct {
			LTP float64 `json:"ltp"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.LTP != 25600.0 {
			t.Errorf("LTP = %f, want 25600.0", body.LTP)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"ltp":25600.0}}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetBaseURI(ts.URL)

	if err := c.SetLTP("NFO", "NIFTY26OCTFUT", 25600.0); err != nil {
		t.Fatalf("SetLTP failed: %v", err)
	}
}

func TestAdmin_ManualFillAndRejectAndCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch r.URL.Path {
		case "/api/orders/ORD_1/fill":
			var body struct {
				Price float64 `json:"price"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Price != 25050.0 {
				t.Errorf("price = %f, want 25050.0", body.Price)
			}
		case "/api/orders/ORD_1/reject":
			var body struct {
				Reason string `json:"reason"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Reason != "Margin shortage" {
				t.Errorf("reason = %s, want Margin shortage", body.Reason)
			}
		case "/api/orders/ORD_1/cancel":
			// ok
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		_, _ = w.Write([]byte(`{"status":"success","data":{"message":"ok"}}`))
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetBaseURI(ts.URL)

	if err := c.ManualFill("ORD_1", 25050.0); err != nil {
		t.Errorf("ManualFill failed: %v", err)
	}
	if err := c.ManualReject("ORD_1", "Margin shortage"); err != nil {
		t.Errorf("ManualReject failed: %v", err)
	}
	if err := c.ManualCancel("ORD_1"); err != nil {
		t.Errorf("ManualCancel failed: %v", err)
	}
}

func TestAdmin_GetAllOrdersAndUsers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		switch r.URL.Path {
		case "/api/orders":
			_, _ = w.Write([]byte(`{
				"status": "success",
				"data": [
					{"order_id": "ORD_1", "user_id": "MASTER01"},
					{"order_id": "ORD_2", "user_id": "FOLLOWER01"}
				]
			}`))
		case "/api/users":
			_, _ = w.Write([]byte(`{
				"status": "success",
				"data": [
					{"user_id": "MASTER01", "role": "master", "api_key": "k_m", "access_token": "t_m"},
					{"user_id": "FOLLOWER01", "role": "follower", "api_key": "k_f", "access_token": "t_f"}
				]
			}`))
		case "/api/quick-order":
			_, _ = w.Write([]byte(`{
				"status": "success",
				"data": {"order_id": "ORD_QUICK_100"}
			}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	c := New("test_key")
	c.SetBaseURI(ts.URL)

	// GetAllOrders
	orders, err := c.GetAllOrders()
	if err != nil {
		t.Fatalf("GetAllOrders failed: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}

	// GetUsers
	users, err := c.GetUsers()
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}
	if len(users) != 2 || users[0].UserID != "MASTER01" {
		t.Errorf("unexpected users: %+v", users)
	}

	// PlaceQuickOrder
	qResp, err := c.PlaceQuickOrder("FOLLOWER01", OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: TransactionTypeBuy,
		Product:         ProductNRML,
		OrderType:       OrderTypeMarket,
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("PlaceQuickOrder failed: %v", err)
	}
	if qResp.OrderID != "ORD_QUICK_100" {
		t.Errorf("OrderID = %s, want ORD_QUICK_100", qResp.OrderID)
	}
}
