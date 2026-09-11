package kite_test

import (
	"context"
	"errors"
	"testing"

	"envoytrade/internal/broker"
	"envoytrade/internal/kite"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

type fakeKiteAPI struct {
	placedVariety kiteconnect.OrderParams
	variety       string
	orderID       string
	placeErr      error
	orders        kiteconnect.Orders
	ordersErr     error
	history       []kiteconnect.Order
	historyErr    error
}

func (f *fakeKiteAPI) PlaceOrder(variety string, orderParams kiteconnect.OrderParams) (kiteconnect.OrderResponse, error) {
	f.variety = variety
	f.placedVariety = orderParams
	if f.placeErr != nil {
		return kiteconnect.OrderResponse{}, f.placeErr
	}
	return kiteconnect.OrderResponse{OrderID: f.orderID}, nil
}

func (f *fakeKiteAPI) GetOrders() (kiteconnect.Orders, error) {
	if f.ordersErr != nil {
		return nil, f.ordersErr
	}
	return f.orders, nil
}

func (f *fakeKiteAPI) GetOrderHistory(orderID string) ([]kiteconnect.Order, error) {
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history, nil
}

func TestBroker_PlaceOrder_TranslatesAndDelegates(t *testing.T) {
	api := &fakeKiteAPI{orderID: "kite-123"}
	b := kite.NewBroker(api)

	params := broker.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26SEP24000CE",
		TransactionType: "BUY",
		Product:         "NRML",
		OrderType:       "MARKET",
		Quantity:        50,
		Tag:             "idempotent-tag-1",
	}

	resp, err := b.PlaceOrder(context.Background(), "", params)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if resp.OrderID != "kite-123" {
		t.Errorf("OrderID = %q, want %q", resp.OrderID, "kite-123")
	}
	if api.variety != kiteconnect.VarietyRegular {
		t.Errorf("variety = %q, want %q", api.variety, kiteconnect.VarietyRegular)
	}
	if api.placedVariety.Tradingsymbol != params.Tradingsymbol {
		t.Errorf("Tradingsymbol = %q, want %q", api.placedVariety.Tradingsymbol, params.Tradingsymbol)
	}
	if api.placedVariety.Quantity != params.Quantity {
		t.Errorf("Quantity = %d, want %d", api.placedVariety.Quantity, params.Quantity)
	}
	if api.placedVariety.Tag != params.Tag {
		t.Errorf("Tag = %q, want %q", api.placedVariety.Tag, params.Tag)
	}
}

func TestBroker_PlaceOrder_PropagatesError(t *testing.T) {
	api := &fakeKiteAPI{placeErr: errors.New("insufficient funds")}
	b := kite.NewBroker(api)

	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{})
	if err == nil {
		t.Fatal("PlaceOrder want error, got nil")
	}
}

func TestBroker_GetOrders_And_History(t *testing.T) {
	api := &fakeKiteAPI{
		orders:  kiteconnect.Orders{{OrderID: "o1"}},
		history: []kiteconnect.Order{{OrderID: "o1", Status: "COMPLETE"}},
	}
	b := kite.NewBroker(api)

	orders, err := b.GetOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if len(orders) != 1 || orders[0].OrderID != "o1" {
		t.Fatalf("unexpected orders: %v", orders)
	}

	hist, err := b.GetOrderHistory(context.Background(), "o1")
	if err != nil {
		t.Fatalf("GetOrderHistory: %v", err)
	}
	if len(hist) != 1 || hist[0].Status != "COMPLETE" {
		t.Fatalf("unexpected history: %v", hist)
	}
}

func TestRESTClientFor_BuildsClient(t *testing.T) {
	cfg := kite.ProxyConfig{
		Scheme:       "http",
		Host:         "proxy.example.com",
		Port:         8080,
		ClientID:     "user",
		ClientSecret: "pass",
	}
	client, err := kite.RESTClientFor(cfg)
	if err != nil {
		t.Fatalf("RESTClientFor: %v", err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}

	badCfg := kite.ProxyConfig{}
	if _, err := kite.RESTClientFor(badCfg); err == nil {
		t.Fatal("RESTClientFor with empty host want error, got nil")
	}
}
