package testbroker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"envoytrade/internal/broker"
)

func TestBroker_PlaceOrder_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders/regular" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token key1:tok1" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		_ = r.ParseForm()
		if r.FormValue("tradingsymbol") != "NIFTY26OCTFUT" || r.FormValue("quantity") != "75" {
			t.Errorf("unexpected form: %v", r.Form)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"order_id":"TB_ORD_999"}}`))
	}))
	defer server.Close()

	mockProxy := &ProxyConfig{Host: "mock-proxy.local", Port: 443}
	b := NewLiveBroker("key1", "tok1", server.URL, mockProxy)

	resp, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        75,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if resp.OrderID != "TB_ORD_999" {
		t.Errorf("expected orderID TB_ORD_999, got %s", resp.OrderID)
	}
}

func TestBroker_PlaceOrder_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error","error_type":"InputException","message":"invalid lot size"}`))
	}))
	defer server.Close()

	mockProxy := &ProxyConfig{Host: "mock-proxy.local", Port: 443}
	b := NewLiveBroker("key1", "tok1", server.URL, mockProxy)

	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Quantity: 10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBroker_Unproxied_BlocksPlaceAndCancel(t *testing.T) {
	b := NewLiveBroker("key1", "tok1", "http://localhost:8089", nil)

	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{Quantity: 10})
	if err == nil {
		t.Fatal("PlaceOrder unproxied expected error, got nil")
	}

	_, err = b.CancelOrder(context.Background(), "regular", "TB-123")
	if err == nil {
		t.Fatal("CancelOrder unproxied expected error, got nil")
	}
}

func TestBroker_DefaultURL(t *testing.T) {
	t.Run("Default Localhost", func(t *testing.T) {
		os.Unsetenv("TESTBROKER_URL")
		b := NewLiveBroker("k", "t", "", nil)
		if b.BaseURI() != "http://localhost:8089" {
			t.Errorf("BaseURI = %s, want http://localhost:8089", b.BaseURI())
		}
	})

	t.Run("Env Override", func(t *testing.T) {
		t.Setenv("TESTBROKER_URL", "http://test-server:9000")
		b := NewLiveBroker("k", "t", "", nil)
		if b.BaseURI() != "http://test-server:9000" {
			t.Errorf("BaseURI = %s, want http://test-server:9000", b.BaseURI())
		}
	})
}
