package kotak_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"envoytrade/internal/kotak"

	"github.com/DragoHex/gokotakneo/models"
	"github.com/shopspring/decimal"
)

type mockKotakAPI struct {
	placeOrderFunc      func(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error)
	modifyOrderFunc     func(ctx context.Context, params models.OrderModifyParams) (*models.OrderResponse, error)
	cancelOrderFunc     func(ctx context.Context, orderNo string, isAMO bool, tradingSymbol string) (*models.OrderResponse, error)
	getOrdersFunc       func(ctx context.Context) ([]models.Order, error)
	getOrderHistoryFunc func(ctx context.Context, orderNo string) ([]models.Order, error)
	getPositionsFunc    func(ctx context.Context) ([]models.Position, error)
	getHoldingsFunc     func(ctx context.Context) ([]models.Holding, error)
	getLimitsFunc       func(ctx context.Context, seg, exch, prod string) (*models.Limits, error)
}

func (m *mockKotakAPI) PlaceOrder(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error) {
	if m.placeOrderFunc != nil {
		return m.placeOrderFunc(ctx, params)
	}
	return &models.OrderResponse{OrderNumber: "2602160001"}, nil
}

func (m *mockKotakAPI) ModifyOrder(ctx context.Context, params models.OrderModifyParams) (*models.OrderResponse, error) {
	if m.modifyOrderFunc != nil {
		return m.modifyOrderFunc(ctx, params)
	}
	return &models.OrderResponse{OrderNumber: params.OrderNo}, nil
}

func (m *mockKotakAPI) CancelOrder(ctx context.Context, orderNo string, isAMO bool, tradingSymbol string) (*models.OrderResponse, error) {
	if m.cancelOrderFunc != nil {
		return m.cancelOrderFunc(ctx, orderNo, isAMO, tradingSymbol)
	}
	return &models.OrderResponse{OrderNumber: orderNo}, nil
}

func (m *mockKotakAPI) GetOrders(ctx context.Context) ([]models.Order, error) {
	if m.getOrdersFunc != nil {
		return m.getOrdersFunc(ctx)
	}
	return nil, nil
}

func (m *mockKotakAPI) GetOrderHistory(ctx context.Context, orderNo string) ([]models.Order, error) {
	if m.getOrderHistoryFunc != nil {
		return m.getOrderHistoryFunc(ctx, orderNo)
	}
	return nil, nil
}

func (m *mockKotakAPI) GetPositions(ctx context.Context) ([]models.Position, error) {
	if m.getPositionsFunc != nil {
		return m.getPositionsFunc(ctx)
	}
	return nil, nil
}

func (m *mockKotakAPI) GetHoldings(ctx context.Context) ([]models.Holding, error) {
	if m.getHoldingsFunc != nil {
		return m.getHoldingsFunc(ctx)
	}
	return nil, nil
}

func (m *mockKotakAPI) GetLimits(ctx context.Context, seg, exch, prod string) (*models.Limits, error) {
	if m.getLimitsFunc != nil {
		return m.getLimitsFunc(ctx, seg, exch, prod)
	}
	return &models.Limits{}, nil
}

type mockInstrumentLookup struct {
	segment    string
	securityID string
	lotSize    int
	err        error
}

func (m *mockInstrumentLookup) ResolveToSecurityID(exchange, tradingsymbol string) (string, string, int, error) {
	return m.segment, m.securityID, m.lotSize, m.err
}

func TestBroker_Execution(t *testing.T) {
	t.Run("Positive: PlaceOrder Limit Order", func(t *testing.T) {
		var capturedParams models.OrderParams
		api := &mockKotakAPI{
			placeOrderFunc: func(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error) {
				capturedParams = params
				return &models.OrderResponse{OrderNumber: "260216000999"}, nil
			},
		}

		resolver := &mockInstrumentLookup{segment: "nse_fo", securityID: "12345", lotSize: 50}
		b := kotak.NewBroker(api, true, resolver)

		resp, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26FEB24000CE",
			TransactionType: "BUY",
			OrderType:       "LIMIT",
			Product:         "NRML",
			Quantity:        50,
			Price:           125.50,
		})

		if err != nil {
			t.Fatalf("PlaceOrder failed: %v", err)
		}
		if resp.OrderID != "260216000999" {
			t.Errorf("OrderID = %s, want 260216000999", resp.OrderID)
		}
		if capturedParams.Es != "nse_fo" || capturedParams.Pc != "NRML" || capturedParams.Pt != "L" || capturedParams.Tt != "B" {
			t.Errorf("unexpected params: %+v", capturedParams)
		}
		if capturedParams.Pr != "125.5" {
			t.Errorf("Pr = %s, want 125.5", capturedParams.Pr)
		}
	})

	t.Run("Positive: PlaceOrder Market Sell Order", func(t *testing.T) {
		var capturedParams models.OrderParams
		api := &mockKotakAPI{
			placeOrderFunc: func(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error) {
				capturedParams = params
				return &models.OrderResponse{OrderNumber: "260216000888"}, nil
			},
		}

		b := kotak.NewBroker(api, true, nil)
		resp, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "INFY",
			TransactionType: "SELL",
			OrderType:       "MARKET",
			Product:         "MIS",
			Quantity:        10,
		})

		if err != nil {
			t.Fatalf("PlaceOrder failed: %v", err)
		}
		if resp.OrderID != "260216000888" {
			t.Errorf("OrderID = %s, want 260216000888", resp.OrderID)
		}
		if capturedParams.Pt != "MKT" || capturedParams.Pr != "0" || capturedParams.Tt != "S" {
			t.Errorf("unexpected params: %+v", capturedParams)
		}
	})

	t.Run("Negative: Unproxied Not Allowed", func(t *testing.T) {
		api := &mockKotakAPI{}
		b := kotak.NewBroker(api, false, nil)

		_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Tradingsymbol: "INFY",
		})
		if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
			t.Fatalf("expected ErrUnproxiedNotAllowed, got %v", err)
		}
	})

	t.Run("Negative: Session Expired 401", func(t *testing.T) {
		api := &mockKotakAPI{
			placeOrderFunc: func(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error) {
				return nil, errors.New("HTTP 401: unauthorized token expired")
			},
		}
		b := kotak.NewBroker(api, true, nil)

		_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{})
		if !errors.Is(err, domain.ErrAuthExpired) {
			t.Fatalf("expected ErrAuthExpired, got %v", err)
		}
	})

	t.Run("Positive: GetPositions Normalization", func(t *testing.T) {
		api := &mockKotakAPI{
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return []models.Position{
					{
						Product:         "MIS",
						TradingSymbol:   "RELIANCE",
						ExchangeSegment: "nse_cm",
						Quantity:        15,
						FilledBuyQty:    15,
						BuyAmount:       decimal.NewFromFloat(37500.00),
					},
				}, nil
			},
		}

		b := kotak.NewBroker(api, true, nil)
		positions, err := b.GetPositions(context.Background())
		if err != nil {
			t.Fatalf("GetPositions failed: %v", err)
		}
		if len(positions) != 1 {
			t.Fatalf("expected 1 position, got %d", len(positions))
		}
		pos := positions[0]
		if pos.Tradingsymbol != "RELIANCE" || pos.Exchange != "NSE" || pos.Quantity != 15 {
			t.Errorf("unexpected position: %+v", pos)
		}
		if pos.AveragePrice != 2500.00 {
			t.Errorf("AveragePrice = %v, want 2500.00", pos.AveragePrice)
		}
	})

	t.Run("Positive: CancelOrder", func(t *testing.T) {
		api := &mockKotakAPI{
			cancelOrderFunc: func(ctx context.Context, orderNo string, isAMO bool, tradingSymbol string) (*models.OrderResponse, error) {
				return &models.OrderResponse{OrderNumber: orderNo}, nil
			},
		}
		b := kotak.NewBroker(api, true, nil)
		resp, err := b.CancelOrder(context.Background(), "regular", "260216000123")
		if err != nil {
			t.Fatalf("CancelOrder failed: %v", err)
		}
		if resp.OrderID != "260216000123" {
			t.Errorf("OrderID = %s, want 260216000123", resp.OrderID)
		}
	})

	t.Run("Positive: GetOrders, GetPositions and GetOrderHistory handle empty results cleanly", func(t *testing.T) {
		api := &mockKotakAPI{
			getOrdersFunc: func(ctx context.Context) ([]models.Order, error) {
				return []models.Order{}, nil
			},
			getPositionsFunc: func(ctx context.Context) ([]models.Position, error) {
				return []models.Position{}, nil
			},
			getOrderHistoryFunc: func(ctx context.Context, orderNo string) ([]models.Order, error) {
				return []models.Order{}, nil
			},
		}
		b := kotak.NewBroker(api, true, nil)

		orders, err := b.GetOrders(context.Background())
		if err != nil {
			t.Fatalf("GetOrders failed: %v", err)
		}
		if len(orders) != 0 {
			t.Errorf("expected 0 orders, got %d", len(orders))
		}

		positions, err := b.GetPositions(context.Background())
		if err != nil {
			t.Fatalf("GetPositions failed: %v", err)
		}
		if len(positions) != 0 {
			t.Errorf("expected 0 positions, got %d", len(positions))
		}

		history, err := b.GetOrderHistory(context.Background(), "ord123")
		if err != nil {
			t.Fatalf("GetOrderHistory failed: %v", err)
		}
		if len(history) != 0 {
			t.Errorf("expected 0 history items, got %d", len(history))
		}
	})

	t.Run("Negative: 100008 IP Whitelist Rejection is NOT Auth Expired", func(t *testing.T) {
		api := &mockKotakAPI{
			placeOrderFunc: func(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error) {
				return nil, errors.New("kotak-neo api error [100008]: unauthorized")
			},
		}
		b := kotak.NewBroker(api, true, nil)
		_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			Product:         "CNC",
			OrderType:       "LIMIT",
			Quantity:        1,
			Price:           2500,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, domain.ErrAuthExpired) {
			t.Errorf("100008 must NOT be mapped to ErrAuthExpired, got %v", err)
		}
		if !strings.Contains(err.Error(), "ip not whitelisted") {
			t.Errorf("expected 'ip not whitelisted' in error, got %v", err)
		}
	})

	t.Run("Positive: PlaceOrder canonicalizes equity trading symbol with -EQ on nse_cm", func(t *testing.T) {
		var captured models.OrderParams
		api := &mockKotakAPI{
			placeOrderFunc: func(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error) {
				captured = params
				return &models.OrderResponse{OrderNumber: "2602160001"}, nil
			},
		}
		b := kotak.NewBroker(api, true, nil)

		// Case 1: Standard cash equity without suffix -> append -EQ
		_, err := b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			TransactionType: "BUY",
			Product:         "CNC",
			OrderType:       "LIMIT",
			Quantity:        1,
			Price:           2500,
		})
		if err != nil {
			t.Fatalf("PlaceOrder failed: %v", err)
		}
		if captured.Ts != "RELIANCE-EQ" {
			t.Errorf("captured.Ts = %q, want 'RELIANCE-EQ'", captured.Ts)
		}
		if captured.Es != "nse_cm" {
			t.Errorf("captured.Es = %q, want 'nse_cm'", captured.Es)
		}

		// Case 2: Cash equity already with -EQ -> keep RELIANCE-EQ
		_, _ = b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE-EQ",
			TransactionType: "BUY",
			Product:         "CNC",
			OrderType:       "LIMIT",
			Quantity:        1,
			Price:           2500,
		})
		if captured.Ts != "RELIANCE-EQ" {
			t.Errorf("captured.Ts = %q, want 'RELIANCE-EQ'", captured.Ts)
		}

		// Case 3: Other series (e.g. -BE) -> keep as-is
		_, _ = b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NSE",
			Tradingsymbol:   "SUZLON-BE",
			TransactionType: "BUY",
			Product:         "CNC",
			OrderType:       "LIMIT",
			Quantity:        1,
			Price:           50,
		})
		if captured.Ts != "SUZLON-BE" {
			t.Errorf("captured.Ts = %q, want 'SUZLON-BE'", captured.Ts)
		}

		// Case 4: Derivative (NFO) -> untouched
		_, _ = b.PlaceOrder(context.Background(), "regular", broker.OrderParams{
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCTFUT",
			TransactionType: "BUY",
			Product:         "NRML",
			OrderType:       "LIMIT",
			Quantity:        75,
			Price:           25000,
		})
		if captured.Ts != "NIFTY26OCTFUT" {
			t.Errorf("captured.Ts = %q, want 'NIFTY26OCTFUT'", captured.Ts)
		}
		if captured.Es != "nse_fo" {
			t.Errorf("captured.Es = %q, want 'nse_fo'", captured.Es)
		}
	})
}
