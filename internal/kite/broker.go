package kite

import (
	"context"
	"fmt"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// API narrows the methods needed from *kiteconnect.Client so tests can
// provide a fake implementation.
type API interface {
	PlaceOrder(variety string, orderParams kiteconnect.OrderParams) (kiteconnect.OrderResponse, error)
	GetOrders() (kiteconnect.Orders, error)
	GetOrderHistory(orderID string) ([]kiteconnect.Order, error)
	GetPositions() (kiteconnect.Positions, error)
	GetHoldings() (kiteconnect.Holdings, error)
	GetUserMargins() (kiteconnect.AllMargins, error)
	CancelOrder(variety string, orderID string, parentOrderID *string) (kiteconnect.OrderResponse, error)
}

// Broker adapts Zerodha's Kite Connect Client to the broker-agnostic
// broker.Broker interface (PLAN.md §1).
type Broker struct {
	api     API
	proxied bool
}

// NewBroker wraps an API implementation (e.g. *kiteconnect.Client).
func NewBroker(api API, proxied bool) *Broker {
	return &Broker{api: api, proxied: proxied}
}

// IsProxied reports whether this broker instance is routed through a dedicated proxy.
func (b *Broker) IsProxied() bool {
	return b.proxied
}

// PlaceOrder translates broker.OrderParams to kiteconnect.OrderParams
// and delegates to the underlying API. Order placement must go via proxy.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("kite place order: %w", domain.ErrUnproxiedNotAllowed)
	}
	if variety == "" {
		variety = kiteconnect.VarietyRegular
	}
	p := kiteconnect.OrderParams{
		Exchange:        params.Exchange,
		Tradingsymbol:   params.Tradingsymbol,
		TransactionType: params.TransactionType,
		Product:         params.Product,
		OrderType:       params.OrderType,
		Quantity:        params.Quantity,
		Price:           params.Price,
		TriggerPrice:    params.TriggerPrice,
		Tag:             params.Tag,
	}
	if p.OrderType == kiteconnect.OrderTypeMarket || p.OrderType == kiteconnect.OrderTypeSLM {
		p.MarketProtection = kiteconnect.MarketProtectionAuto
	}

	resp, err := b.api.PlaceOrder(variety, p)
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("kite place order: %w", err)
	}
	return broker.OrderResponse{OrderID: resp.OrderID}, nil
}

// GetOrders retrieves all orders from the broker account.
func (b *Broker) GetOrders(ctx context.Context) ([]kiteconnect.Order, error) {
	orders, err := b.api.GetOrders()
	if err != nil {
		return nil, fmt.Errorf("kite get orders: %w", err)
	}
	return orders, nil
}

// GetOrderHistory retrieves transition history for an order.
func (b *Broker) GetOrderHistory(ctx context.Context, orderID string) ([]kiteconnect.Order, error) {
	history, err := b.api.GetOrderHistory(orderID)
	if err != nil {
		return nil, fmt.Errorf("kite get order history: %w", err)
	}
	return history, nil
}

// GetPositions retrieves user net positions mapped to broker-agnostic Position types.
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	positions, err := b.api.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("kite get positions: %w", err)
	}
	out := make([]broker.Position, 0, len(positions.Net))
	for _, p := range positions.Net {
		posMtm := p.PnL
		if posMtm == 0 && p.M2M != 0 {
			posMtm = p.M2M
		}
		out = append(out, broker.Position{
			Exchange:      p.Exchange,
			Tradingsymbol: p.Tradingsymbol,
			Product:       p.Product,
			Quantity:      p.Quantity,
			AveragePrice:  p.AveragePrice,
			LastPrice:     p.LastPrice,
			M2M:           posMtm,
			PnL:           p.PnL,
		})
	}
	return out, nil
}

// GetOpenOrders retrieves all open or trigger-pending orders.
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.api.GetOrders()
	if err != nil {
		return nil, fmt.Errorf("kite get orders: %w", err)
	}
	var out []broker.Order
	for _, o := range orders {
		if o.Status == "OPEN" || o.Status == "TRIGGER PENDING" {
			out = append(out, broker.Order{
				OrderID:        o.OrderID,
				Exchange:       o.Exchange,
				Tradingsymbol:  o.TradingSymbol,
				Status:         o.Status,
				Quantity:       int(o.Quantity),
				FilledQuantity: int(o.FilledQuantity),
			})
		}
	}
	return out, nil
}

// CancelOrder cancels an open order on the broker. Cancellation must go via proxy.
func (b *Broker) CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("kite cancel order: %w", domain.ErrUnproxiedNotAllowed)
	}
	if variety == "" {
		variety = kiteconnect.VarietyRegular
	}
	resp, err := b.api.CancelOrder(variety, orderID, nil)
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("kite cancel order: %w", err)
	}
	return broker.OrderResponse{OrderID: resp.OrderID}, nil
}

// GetHoldings retrieves user equity holdings.
func (b *Broker) GetHoldings(ctx context.Context) (kiteconnect.Holdings, error) {
	holdings, err := b.api.GetHoldings()
	if err != nil {
		return nil, fmt.Errorf("kite get holdings: %w", err)
	}
	return holdings, nil
}

// GetUserMargins retrieves equity and commodity user margins.
func (b *Broker) GetUserMargins(ctx context.Context) (kiteconnect.AllMargins, error) {
	margins, err := b.api.GetUserMargins()
	if err != nil {
		return kiteconnect.AllMargins{}, fmt.Errorf("kite get user margins: %w", err)
	}
	return margins, nil
}
