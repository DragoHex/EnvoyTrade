package kite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	"github.com/shopspring/decimal"
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

// InstrumentLookup resolves broker-specific tradingsymbol for Kite (PLAN.md §1).
type InstrumentLookup interface {
	ResolveForBroker(targetBroker, exchange, tradingsymbol string) (exchangeSegment string, securityID string, lotSize int, targetSymbol string, err error)
}

// Broker adapts Zerodha's Kite Connect Client to the broker-agnostic
// broker.Broker interface (PLAN.md §1).
type Broker struct {
	api      API
	proxied  bool
	resolver InstrumentLookup
}

// NewBroker wraps an API implementation (e.g. *kiteconnect.Client) and optional InstrumentLookup.
func NewBroker(api API, proxied bool, resolver ...InstrumentLookup) *Broker {
	b := &Broker{api: api, proxied: proxied}
	if len(resolver) > 0 {
		b.resolver = resolver[0]
	}
	return b
}

// IsProxied reports whether this broker instance is routed through a dedicated proxy.
func (b *Broker) IsProxied() bool {
	return b.proxied
}

func mapExchangeToKite(ex string) string {
	e := strings.ToUpper(strings.TrimSpace(ex))
	switch e {
	case "MCX", "MCX_FO", "MCX_COMM":
		return kiteconnect.ExchangeMCX
	case "NFO", "NSE_FO", "NSE_FNO":
		return kiteconnect.ExchangeNFO
	case "BFO", "BSE_FO", "BSE_FNO":
		return kiteconnect.ExchangeBFO
	case "BSE", "BSE_CM", "BSE_EQ":
		return kiteconnect.ExchangeBSE
	case "NSE", "NSE_CM", "NSE_EQ":
		return kiteconnect.ExchangeNSE
	default:
		if strings.Contains(e, "MCX") {
			return kiteconnect.ExchangeMCX
		}
		if strings.Contains(e, "BSE") && strings.Contains(e, "FO") {
			return kiteconnect.ExchangeBFO
		}
		if strings.Contains(e, "FO") || strings.Contains(e, "NFO") {
			return kiteconnect.ExchangeNFO
		}
		return e
	}
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

	exchange := mapExchangeToKite(params.Exchange)
	tradingsymbol := params.Tradingsymbol
	if b.resolver != nil && params.Exchange != "" && tradingsymbol != "" {
		if exSeg, _, _, targetSym, err := b.resolver.ResolveForBroker("kite", params.Exchange, tradingsymbol); err == nil {
			if exSeg != "" {
				exchange = mapExchangeToKite(exSeg)
			}
			if targetSym != "" {
				tradingsymbol = targetSym
			}
		}
	}

	// For cash equities in NSE or BSE, strip suffixes like -EQ or -BE
	if exchange == kiteconnect.ExchangeNSE || exchange == kiteconnect.ExchangeBSE {
		tradingsymbol = strings.TrimSuffix(tradingsymbol, "-EQ")
		tradingsymbol = strings.TrimSuffix(tradingsymbol, "-BE")
	}

	p := kiteconnect.OrderParams{
		Exchange:        exchange,
		Tradingsymbol:   tradingsymbol,
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

func toBrokerOrder(o kiteconnect.Order) broker.Order {
	raw, _ := json.Marshal(o)
	return broker.Order{
		OrderID:         o.OrderID,
		Exchange:        o.Exchange,
		Tradingsymbol:   o.TradingSymbol,
		TransactionType: o.TransactionType,
		Product:         o.Product,
		OrderType:       o.OrderType,
		Status:          o.Status,
		StatusMessage:   o.StatusMessage,
		Quantity:        int(o.Quantity),
		FilledQuantity:  int(o.FilledQuantity),
		Price:           decimal.NewFromFloat(o.Price),
		TriggerPrice:    decimal.NewFromFloat(o.TriggerPrice),
		AveragePrice:    decimal.NewFromFloat(o.AveragePrice),
		OrderTimestamp:  o.OrderTimestamp.Time,
		Tag:             o.Tag,
		RawPayload:      raw,
	}
}

// GetOrders retrieves all orders from the broker account mapped to broker-agnostic Order types.
func (b *Broker) GetOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.api.GetOrders()
	if err != nil {
		return nil, fmt.Errorf("kite get orders: %w", err)
	}
	out := make([]broker.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, toBrokerOrder(o))
	}
	return out, nil
}

// GetOrderHistory retrieves transition history for an order mapped to broker-agnostic Order types.
func (b *Broker) GetOrderHistory(ctx context.Context, orderID string) ([]broker.Order, error) {
	history, err := b.api.GetOrderHistory(orderID)
	if err != nil {
		return nil, fmt.Errorf("kite get order history: %w", err)
	}
	out := make([]broker.Order, 0, len(history))
	for _, o := range history {
		out = append(out, toBrokerOrder(o))
	}
	return out, nil
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
			out = append(out, toBrokerOrder(o))
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
