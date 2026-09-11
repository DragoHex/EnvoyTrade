package kite

import (
	"context"
	"fmt"

	"envoytrade/internal/broker"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// API narrows the methods needed from *kiteconnect.Client so tests can
// provide a fake implementation.
type API interface {
	PlaceOrder(variety string, orderParams kiteconnect.OrderParams) (kiteconnect.OrderResponse, error)
	GetOrders() (kiteconnect.Orders, error)
	GetOrderHistory(orderID string) ([]kiteconnect.Order, error)
}

// Broker adapts Zerodha's Kite Connect Client to the broker-agnostic
// broker.Broker interface (PLAN.md §1).
type Broker struct {
	api API
}

// NewBroker wraps an API implementation (e.g. *kiteconnect.Client).
func NewBroker(api API) *Broker {
	return &Broker{api: api}
}

// PlaceOrder translates broker.OrderParams to kiteconnect.OrderParams
// and delegates to the underlying API.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
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
		Tag:             params.Tag,
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
