package dhan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"envoytrade/internal/broker"
	"envoytrade/internal/dhan"
	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan/models"
	"github.com/shopspring/decimal"
)

type fakeDhanAPI struct {
	placedReq       models.OrderRequest
	placeErr        error
	orderID         string
	positions       []models.Position
	positionsErr    error
	orders          []models.OrderResponse
	ordersErr       error
	orderDetails    *models.OrderResponse
	orderDetailsErr error
	cancelErr       error
}

func (f *fakeDhanAPI) PlaceOrder(ctx context.Context, req models.OrderRequest) (*models.OrderStatusResponse, error) {
	f.placedReq = req
	if f.placeErr != nil {
		return nil, f.placeErr
	}
	return &models.OrderStatusResponse{
		OrderID:     f.orderID,
		OrderStatus: "TRANSIT",
	}, nil
}

func (f *fakeDhanAPI) GetPositions(ctx context.Context) ([]models.Position, error) {
	if f.positionsErr != nil {
		return nil, f.positionsErr
	}
	return f.positions, nil
}

func (f *fakeDhanAPI) GetOrders(ctx context.Context) ([]models.OrderResponse, error) {
	if f.ordersErr != nil {
		return nil, f.ordersErr
	}
	return f.orders, nil
}

func (f *fakeDhanAPI) GetOrderByID(ctx context.Context, orderID string) (*models.OrderResponse, error) {
	if f.orderDetailsErr != nil {
		return nil, f.orderDetailsErr
	}
	return f.orderDetails, nil
}

func (f *fakeDhanAPI) CancelOrder(ctx context.Context, orderID string) (*models.OrderStatusResponse, error) {
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	return &models.OrderStatusResponse{
		OrderID:     orderID,
		OrderStatus: "CANCELLED",
	}, nil
}

func setupTestResolver(t *testing.T) *dhan.InstrumentResolver {
	t.Helper()
	resolver := dhan.NewInstrumentResolver(nil)
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_TRADING_SYMBOL,SEM_LOT_UNITS
NSE,E,11536,TATASTEEL,1
NSE,D,45210,NIFTY-Oct2026-25000-CE,25
MCX,M,251200,GOLD-Oct2026,1
`
	if err := resolver.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("setupTestResolver failed: %v", err)
	}
	return resolver
}

func TestBroker_PlaceOrder_Positive_Market(t *testing.T) {
	resolver := setupTestResolver(t)
	api := &fakeDhanAPI{orderID: "dhan-ord-001"}
	b := dhan.NewBroker(api, true, resolver)

	params := broker.OrderParams{
		Exchange:        "NSE",
		Tradingsymbol:   "TATASTEEL",
		TransactionType: "BUY",
		Product:         "CNC",
		OrderType:       "MARKET",
		Quantity:        50,
		Tag:             "idempotent-tag-001",
	}

	resp, err := b.PlaceOrder(context.Background(), "regular", params)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if resp.OrderID != "dhan-ord-001" {
		t.Errorf("got orderID=%s, want dhan-ord-001", resp.OrderID)
	}

	// Verify translated Dhan request
	if api.placedReq.SecurityID != "11536" {
		t.Errorf("SecurityID=%s, want 11536", api.placedReq.SecurityID)
	}
	if api.placedReq.ExchangeSegment != "NSE_EQ" {
		t.Errorf("ExchangeSegment=%s, want NSE_EQ", api.placedReq.ExchangeSegment)
	}
	if api.placedReq.TransactionType != "BUY" {
		t.Errorf("TransactionType=%s, want BUY", api.placedReq.TransactionType)
	}
	if api.placedReq.ProductType != "CNC" {
		t.Errorf("ProductType=%s, want CNC", api.placedReq.ProductType)
	}
	if api.placedReq.OrderType != "MARKET" {
		t.Errorf("OrderType=%s, want MARKET", api.placedReq.OrderType)
	}
	if api.placedReq.CorrelationID != "idempotent-tag-001" {
		t.Errorf("CorrelationID=%s, want idempotent-tag-001", api.placedReq.CorrelationID)
	}
	if api.placedReq.Quantity != 50 {
		t.Errorf("Quantity=%d, want 50", api.placedReq.Quantity)
	}
}

func TestBroker_PlaceOrder_Positive_LimitAndTrigger(t *testing.T) {
	resolver := setupTestResolver(t)
	api := &fakeDhanAPI{orderID: "dhan-limit-002"}
	b := dhan.NewBroker(api, true, resolver)

	params := broker.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY-Oct2026-25000-CE",
		TransactionType: "SELL",
		Product:         "MIS",
		OrderType:       "SL",
		Quantity:        25,
		Price:           150.25,
		TriggerPrice:    148.00,
		Tag:             "tag-sl-002",
	}

	resp, err := b.PlaceOrder(context.Background(), "", params)
	if err != nil {
		t.Fatalf("PlaceOrder Limit/Trigger failed: %v", err)
	}
	if resp.OrderID != "dhan-limit-002" {
		t.Errorf("got orderID=%s, want dhan-limit-002", resp.OrderID)
	}

	if api.placedReq.SecurityID != "45210" {
		t.Errorf("SecurityID=%s, want 45210", api.placedReq.SecurityID)
	}
	if api.placedReq.ExchangeSegment != "NSE_FNO" {
		t.Errorf("ExchangeSegment=%s, want NSE_FNO", api.placedReq.ExchangeSegment)
	}
	if api.placedReq.ProductType != "INTRADAY" {
		t.Errorf("ProductType=%s, want INTRADAY for MIS", api.placedReq.ProductType)
	}
	if api.placedReq.OrderType != "STOP_LOSS" {
		t.Errorf("OrderType=%s, want STOP_LOSS for SL", api.placedReq.OrderType)
	}
	if !api.placedReq.Price.Equal(decimal.NewFromFloat(150.25)) {
		t.Errorf("Price=%s, want 150.25", api.placedReq.Price)
	}
	if !api.placedReq.TriggerPrice.Equal(decimal.NewFromFloat(148.00)) {
		t.Errorf("TriggerPrice=%s, want 148.00", api.placedReq.TriggerPrice)
	}
}

func TestBroker_Unproxied_BlocksPlaceAndCancel(t *testing.T) {
	resolver := setupTestResolver(t)
	api := &fakeDhanAPI{orderID: "unproxied-test"}
	b := dhan.NewBroker(api, false, resolver)

	// Mutating operations should fail with ErrUnproxiedNotAllowed
	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Exchange:      "NSE",
		Tradingsymbol: "TATASTEEL",
		Quantity:      10,
	})
	if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
		t.Errorf("PlaceOrder unproxied got err=%v, want ErrUnproxiedNotAllowed", err)
	}

	_, err = b.CancelOrder(context.Background(), "regular", "ord-1")
	if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
		t.Errorf("CancelOrder unproxied got err=%v, want ErrUnproxiedNotAllowed", err)
	}

	// Read operations should succeed even when unproxied
	positions, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions unproxied failed: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("unexpected positions length: %d", len(positions))
	}
}

func TestBroker_PlaceOrder_NegativeScenarios(t *testing.T) {
	resolver := setupTestResolver(t)

	// 1. Symbol not in resolver
	api := &fakeDhanAPI{}
	b := dhan.NewBroker(api, true, resolver)
	_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Exchange:      "NSE",
		Tradingsymbol: "NON_EXISTENT",
		Quantity:      10,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown symbol, got %v", err)
	}

	// 2. Broker API error (RMS rejection)
	api.placeErr = errors.New("RMS: Margin Insufficient")
	_, err = b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Exchange:      "NSE",
		Tradingsymbol: "TATASTEEL",
		Quantity:      10,
	})
	if err == nil || !strings.Contains(err.Error(), "Margin Insufficient") {
		t.Errorf("expected Margin Insufficient error, got %v", err)
	}

	// 3. Auth expired error
	api.placeErr = errors.New("401 Unauthorized: token expired")
	_, err = b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
		Exchange:      "NSE",
		Tradingsymbol: "TATASTEEL",
		Quantity:      10,
	})
	if !errors.Is(err, domain.ErrAuthExpired) {
		t.Errorf("expected ErrAuthExpired for 401 error, got %v", err)
	}
}

func TestBroker_GetPositions(t *testing.T) {
	resolver := setupTestResolver(t)
	api := &fakeDhanAPI{
		positions: []models.Position{
			{
				TradingSymbol:   "TATASTEEL",
				ExchangeSegment: "NSE_EQ",
				ProductType:     "CNC",
				NetQty:          100,
				BuyAvg:          decimal.NewFromFloat(120.50),
				RealizedProfit:  decimal.NewFromFloat(500.00),
				UnrealizedProfit: decimal.NewFromFloat(250.00),
			},
		},
	}
	b := dhan.NewBroker(api, true, resolver)

	positions, err := b.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("got %d positions, want 1", len(positions))
	}
	p := positions[0]
	if p.Tradingsymbol != "TATASTEEL" || p.Exchange != "NSE" || p.Quantity != 100 {
		t.Errorf("position mismatch: %+v", p)
	}
	if p.AveragePrice != 120.50 {
		t.Errorf("AveragePrice=%f, want 120.50", p.AveragePrice)
	}
	if p.M2M != 250.00 {
		t.Errorf("M2M=%f, want 250.00", p.M2M)
	}
	if p.PnL != 750.00 {
		t.Errorf("PnL=%f, want 750.00", p.PnL)
	}
}

func TestBroker_GetOrders_And_OpenOrders(t *testing.T) {
	resolver := setupTestResolver(t)
	api := &fakeDhanAPI{
		orders: []models.OrderResponse{
			{
				OrderID:         "ord-1",
				TradingSymbol:   "TATASTEEL",
				ExchangeSegment: "NSE_EQ",
				TransactionType: "BUY",
				ProductType:     "CNC",
				OrderType:       "LIMIT",
				OrderStatus:     "TRADED",
				Quantity:        100,
				Price:           decimal.NewFromFloat(120.00),
				CorrelationID:   "tag-1",
			},
			{
				OrderID:         "ord-2",
				TradingSymbol:   "TATASTEEL",
				ExchangeSegment: "NSE_EQ",
				TransactionType: "BUY",
				ProductType:     "CNC",
				OrderType:       "LIMIT",
				OrderStatus:     "PENDING",
				Quantity:        50,
				Price:           decimal.NewFromFloat(119.00),
				CorrelationID:   "tag-2",
			},
		},
	}
	b := dhan.NewBroker(api, true, resolver)

	// 1. GetOrders (returns all 2)
	orders, err := b.GetOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("got %d orders, want 2", len(orders))
	}
	if orders[0].Status != "COMPLETE" { // TRADED mapped to canonical COMPLETE
		t.Errorf("orders[0].Status=%s, want COMPLETE", orders[0].Status)
	}

	// 2. GetOpenOrders (filters for PENDING/TRANSIT only)
	openOrders, err := b.GetOpenOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}
	if len(openOrders) != 1 {
		t.Fatalf("got %d open orders, want 1", len(openOrders))
	}
	if openOrders[0].OrderID != "ord-2" {
		t.Errorf("openOrders[0].OrderID=%s, want ord-2", openOrders[0].OrderID)
	}
}

func TestBroker_CancelOrder(t *testing.T) {
	resolver := setupTestResolver(t)
	api := &fakeDhanAPI{}
	b := dhan.NewBroker(api, true, resolver)

	resp, err := b.CancelOrder(context.Background(), "regular", "ord-to-cancel")
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}
	if resp.OrderID != "ord-to-cancel" {
		t.Errorf("got resp.OrderID=%s, want ord-to-cancel", resp.OrderID)
	}
}

func TestBroker_PlaceOrder_ResolvesNFOTradingSymbolToDhanSecurityID(t *testing.T) {
	resolver := dhan.NewInstrumentResolver(nil)
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_TRADING_SYMBOL,SEM_LOT_UNITS
NSE,D,53376,NIFTY-Nov2026-21000-PE,65
`
	if err := resolver.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("LoadFromReader failed: %v", err)
	}

	api := &fakeDhanAPI{orderID: "dhan-fno-101"}
	b := dhan.NewBroker(api, true, resolver)

	// Placing order with canonical exchange "NFO" should resolve to Dhan security ID "53376" and "NSE_FNO"
	params := broker.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY-Nov2026-21000-PE",
		TransactionType: "BUY",
		Product:         "MIS",
		OrderType:       "MARKET",
		Quantity:        65,
		Tag:             "tag-fno-101",
	}

	resp, err := b.PlaceOrder(context.Background(), "regular", params)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if resp.OrderID != "dhan-fno-101" {
		t.Errorf("got orderID=%s, want dhan-fno-101", resp.OrderID)
	}

	if api.placedReq.SecurityID != "53376" {
		t.Errorf("got SecurityID=%s, want 53376", api.placedReq.SecurityID)
	}
	if api.placedReq.ExchangeSegment != "NSE_FNO" {
		t.Errorf("got ExchangeSegment=%s, want NSE_FNO", api.placedReq.ExchangeSegment)
	}
	if api.placedReq.TradingSymbol != "NIFTY-Nov2026-21000-PE" {
		t.Errorf("got TradingSymbol=%s, want NIFTY-Nov2026-21000-PE", api.placedReq.TradingSymbol)
	}
}

func TestBroker_PlaceOrder_UnresolvedSymbolReturnsNotFound(t *testing.T) {
	resolver := dhan.NewInstrumentResolver(nil)
	api := &fakeDhanAPI{orderID: "should-not-reach"}
	b := dhan.NewBroker(api, true, resolver)

	params := broker.OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "UNKNOWN-OPT",
		TransactionType: "BUY",
		Product:         "MIS",
		OrderType:       "MARKET",
		Quantity:        65,
	}

	_, err := b.PlaceOrder(context.Background(), "regular", params)
	if err == nil {
		t.Fatal("expected error for unknown symbol, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got: %v", err)
	}
}
