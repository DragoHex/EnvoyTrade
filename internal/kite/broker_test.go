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
	positions     kiteconnect.Positions
	positionsErr  error
	holdings      kiteconnect.Holdings
	holdingsErr   error
	margins       kiteconnect.AllMargins
	marginsErr    error
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

func (f *fakeKiteAPI) GetPositions() (kiteconnect.Positions, error) {
	if f.positionsErr != nil {
		return kiteconnect.Positions{}, f.positionsErr
	}
	return f.positions, nil
}

func (f *fakeKiteAPI) GetHoldings() (kiteconnect.Holdings, error) {
	if f.holdingsErr != nil {
		return nil, f.holdingsErr
	}
	return f.holdings, nil
}

func (f *fakeKiteAPI) GetUserMargins() (kiteconnect.AllMargins, error) {
	if f.marginsErr != nil {
		return kiteconnect.AllMargins{}, f.marginsErr
	}
	return f.margins, nil
}

func (f *fakeKiteAPI) CancelOrder(variety string, orderID string, parentOrderID *string) (kiteconnect.OrderResponse, error) {
	if f.placeErr != nil {
		return kiteconnect.OrderResponse{}, f.placeErr
	}
	return kiteconnect.OrderResponse{OrderID: orderID}, nil
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
	if api.placedVariety.MarketProtection != kiteconnect.MarketProtectionAuto {
		t.Errorf("MarketProtection for MARKET = %v, want %v", api.placedVariety.MarketProtection, kiteconnect.MarketProtectionAuto)
	}
}

func TestBroker_PlaceOrder_LimitOrder_MapsPriceAndTriggerPrice(t *testing.T) {
	api := &fakeKiteAPI{orderID: "kite-limit-1"}
	b := kite.NewBroker(api)

	params := broker.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26OCT11000CE",
		TransactionType: "SELL",
		Product:         "NRML",
		OrderType:       "LIMIT",
		Quantity:        1,
		Price:           26.5,
		TriggerPrice:    25.0,
		Tag:             "idempotent-tag-limit",
	}

	resp, err := b.PlaceOrder(context.Background(), "", params)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if resp.OrderID != "kite-limit-1" {
		t.Errorf("OrderID = %q, want kite-limit-1", resp.OrderID)
	}
	if api.placedVariety.Price != 26.5 {
		t.Errorf("Price = %v, want 26.5", api.placedVariety.Price)
	}
	if api.placedVariety.TriggerPrice != 25.0 {
		t.Errorf("TriggerPrice = %v, want 25.0", api.placedVariety.TriggerPrice)
	}
	if api.placedVariety.MarketProtection != 0 {
		t.Errorf("MarketProtection for LIMIT = %v, want 0", api.placedVariety.MarketProtection)
	}
}

func TestBroker_PlaceOrder_SLMOrder_SetsMarketProtectionAuto(t *testing.T) {
	api := &fakeKiteAPI{orderID: "kite-slm-1"}
	b := kite.NewBroker(api)

	params := broker.OrderParams{
		Exchange:        "MCX",
		Tradingsymbol:   "CRUDEOIL26OCT11000CE",
		TransactionType: "SELL",
		Product:         "NRML",
		OrderType:       "SL-M",
		Quantity:        1,
		TriggerPrice:    25.0,
	}

	_, err := b.PlaceOrder(context.Background(), "", params)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if api.placedVariety.MarketProtection != kiteconnect.MarketProtectionAuto {
		t.Errorf("MarketProtection for SL-M = %v, want %v", api.placedVariety.MarketProtection, kiteconnect.MarketProtectionAuto)
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

func TestBroker_GetPositions_Delegates(t *testing.T) {
	api := &fakeKiteAPI{
		positions: kiteconnect.Positions{
			Net: []kiteconnect.Position{
				{Tradingsymbol: "NIFTY26SEP24000CE", Product: "NRML", Quantity: 50, M2M: 250.5},
			},
		},
	}
	b := kite.NewBroker(api)

	pos, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(pos) != 1 || pos[0].Tradingsymbol != "NIFTY26SEP24000CE" {
		t.Fatalf("unexpected positions: %+v", pos)
	}
}

func TestBroker_GetHoldings_Delegates(t *testing.T) {
	api := &fakeKiteAPI{
		holdings: kiteconnect.Holdings{
			{Tradingsymbol: "INFY", Quantity: 100, AveragePrice: 1500.0},
		},
	}
	b := kite.NewBroker(api)

	h, err := b.GetHoldings(context.Background())
	if err != nil {
		t.Fatalf("GetHoldings: %v", err)
	}
	if len(h) != 1 || h[0].Tradingsymbol != "INFY" {
		t.Fatalf("unexpected holdings: %+v", h)
	}
}

func TestBroker_GetUserMargins_Delegates(t *testing.T) {
	api := &fakeKiteAPI{
		margins: kiteconnect.AllMargins{
			Equity: kiteconnect.Margins{
				Available: kiteconnect.AvailableMargins{Cash: 50000.0, LiveBalance: 120000.0},
			},
		},
	}
	b := kite.NewBroker(api)

	m, err := b.GetUserMargins(context.Background())
	if err != nil {
		t.Fatalf("GetUserMargins: %v", err)
	}
	if m.Equity.Available.Cash != 50000.0 {
		t.Fatalf("unexpected margins: %+v", m)
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
