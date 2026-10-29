package dhan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan"
	"github.com/DragoHex/godhan/models"
	"github.com/shopspring/decimal"
)

// API narrows the methods needed from *godhan.Client so tests can provide a fake implementation.
type API interface {
	PlaceOrder(ctx context.Context, req models.OrderRequest) (*models.OrderStatusResponse, error)
	GetPositions(ctx context.Context) ([]models.Position, error)
	GetOrders(ctx context.Context) ([]models.OrderResponse, error)
	GetOrderByID(ctx context.Context, orderID string) (*models.OrderResponse, error)
	CancelOrder(ctx context.Context, orderID string) (*models.OrderStatusResponse, error)
}

// ProxyConfig aliases godhan.ProxyConfig for native forward proxy routing.
type ProxyConfig = godhan.ProxyConfig

// InstrumentLookup maps (exchange, tradingsymbol) to Dhan's (ExchangeSegment, SecurityID, lotSize).
type InstrumentLookup interface {
	ResolveToSecurityID(exchange, tradingsymbol string) (exchangeSegment string, securityID string, lotSize int, err error)
}

// Broker adapts DhanHQ API v2 Client to the broker-agnostic broker.Broker interface (PLAN.md §1).
type Broker struct {
	api      API
	proxied  bool
	resolver InstrumentLookup
}

// NewBroker constructs a Broker wrapping an API implementation and an InstrumentLookup.
func NewBroker(api API, proxied bool, resolver InstrumentLookup) *Broker {
	return &Broker{
		api:      api,
		proxied:  proxied,
		resolver: resolver,
	}
}

// NewLiveBroker constructs a live Dhan Broker client with optional static proxy routing.
func NewLiveBroker(clientID, accessToken string, proxyCfg *ProxyConfig, resolver InstrumentLookup) (*Broker, error) {
	opts := BuildClientOptions(proxyCfg)
	proxied := proxyCfg != nil && proxyCfg.Host != ""
	client := godhan.NewClient(clientID, accessToken, opts...)
	return NewBroker(client, proxied, resolver), nil
}

// IsProxied reports whether this broker instance is routed through a dedicated proxy.
func (b *Broker) IsProxied() bool {
	return b.proxied
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "token expired") || strings.Contains(msg, "invalid token"):
		return fmt.Errorf("%w: %v", domain.ErrAuthExpired, err)
	case strings.Contains(msg, "404") || strings.Contains(msg, "not found"):
		return fmt.Errorf("%w: %v", domain.ErrNotFound, err)
	case strings.Contains(msg, "unreachable") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "timeout"):
		return fmt.Errorf("%w: %v", domain.ErrBrokerUnreachable, err)
	default:
		return err
	}
}

// mapProduct converts canonical/Kite product types to Dhan's ProductType enum.
func mapProduct(product string) string {
	switch strings.ToUpper(strings.TrimSpace(product)) {
	case "CNC":
		return "CNC"
	case "MIS", "INTRADAY":
		return "INTRADAY"
	case "NRML", "MARGIN":
		return "MARGIN"
	case "MTF":
		return "MTF"
	case "CO":
		return "CO"
	case "BO":
		return "BO"
	default:
		return "INTRADAY"
	}
}

// mapOrderType converts canonical order types to Dhan's OrderType enum.
func mapOrderType(orderType string) string {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "MARKET":
		return "MARKET"
	case "LIMIT":
		return "LIMIT"
	case "SL", "STOP_LOSS":
		return "STOP_LOSS"
	case "SL-M", "SLM", "STOP_LOSS_MARKET":
		return "STOP_LOSS_MARKET"
	default:
		return "MARKET"
	}
}

// PlaceOrder translates broker.OrderParams to Dhan's models.OrderRequest and delegates to Dhan API.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("dhan place order: %w", domain.ErrUnproxiedNotAllowed)
	}

	if b.resolver == nil {
		return broker.OrderResponse{}, fmt.Errorf("dhan place order: %w: instrument resolver not configured", domain.ErrNotFound)
	}

	exSegment, secID, _, err := b.resolver.ResolveToSecurityID(params.Exchange, params.Tradingsymbol)
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("dhan place order: %w", err)
	}

	req := models.OrderRequest{
		TransactionType: strings.ToUpper(strings.TrimSpace(params.TransactionType)),
		ExchangeSegment: exSegment,
		ProductType:     mapProduct(params.Product),
		OrderType:       mapOrderType(params.OrderType),
		Validity:        "DAY",
		TradingSymbol:   params.Tradingsymbol,
		SecurityID:      secID,
		Quantity:        int32(params.Quantity),
		CorrelationID:   params.Tag,
	}

	if params.Price > 0 {
		req.Price = decimal.NewFromFloat(params.Price)
	}
	if params.TriggerPrice > 0 {
		req.TriggerPrice = decimal.NewFromFloat(params.TriggerPrice)
	}

	resp, err := b.api.PlaceOrder(ctx, req)
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("dhan place order: %w", translateError(err))
	}

	return broker.OrderResponse{OrderID: resp.OrderID}, nil
}

// toBrokerOrder maps a Dhan OrderResponse into a canonical broker.Order.
func toBrokerOrder(o models.OrderResponse) broker.Order {
	raw, _ := json.Marshal(o)

	status := strings.ToUpper(strings.TrimSpace(o.OrderStatus))
	switch status {
	case "TRADED":
		status = "COMPLETE"
	case "PENDING", "TRANSIT":
		status = "OPEN"
	}

	price, _ := o.Price.Float64()
	triggerPrice, _ := o.TriggerPrice.Float64()
	avgPrice, _ := o.AverageTradedPrice.Float64()

	var ts time.Time
	if o.CreateTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", o.CreateTime); err == nil {
			ts = t
		}
	}

	exchange := "NSE"
	if strings.HasPrefix(o.ExchangeSegment, "BSE") {
		exchange = "BSE"
	} else if strings.HasPrefix(o.ExchangeSegment, "MCX") {
		exchange = "MCX"
	}
	if strings.Contains(o.ExchangeSegment, "FNO") && exchange == "NSE" {
		exchange = "NFO"
	}

	return broker.Order{
		OrderID:         o.OrderID,
		Exchange:        exchange,
		Tradingsymbol:   o.TradingSymbol,
		TransactionType: o.TransactionType,
		Product:         o.ProductType,
		OrderType:       o.OrderType,
		Status:          status,
		StatusMessage:   o.OmsErrorDescription,
		Quantity:        int(o.Quantity),
		FilledQuantity:  int(o.FilledQty),
		Price:           decimal.NewFromFloat(price),
		TriggerPrice:    decimal.NewFromFloat(triggerPrice),
		AveragePrice:    decimal.NewFromFloat(avgPrice),
		OrderTimestamp:  ts,
		Tag:             o.CorrelationID,
		RawPayload:      raw,
	}
}

// GetOrders retrieves all orders placed today from Dhan.
func (b *Broker) GetOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.api.GetOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("dhan get orders: %w", translateError(err))
	}
	out := make([]broker.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, toBrokerOrder(o))
	}
	return out, nil
}

// GetOpenOrders retrieves currently pending or in-transit orders.
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.api.GetOrders(ctx)
	if err != nil {
		return nil, fmt.Errorf("dhan get open orders: %w", translateError(err))
	}
	var out []broker.Order
	for _, o := range orders {
		st := strings.ToUpper(strings.TrimSpace(o.OrderStatus))
		if st == "PENDING" || st == "TRANSIT" || st == "OPEN" {
			out = append(out, toBrokerOrder(o))
		}
	}
	return out, nil
}

// GetOrderHistory retrieves audit status transitions for an order.
func (b *Broker) GetOrderHistory(ctx context.Context, orderID string) ([]broker.Order, error) {
	o, err := b.api.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("dhan get order history: %w", translateError(err))
	}
	if o == nil {
		return []broker.Order{}, nil
	}
	return []broker.Order{toBrokerOrder(*o)}, nil
}

// GetPositions retrieves user net positions mapped to broker-agnostic Position types.
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	positions, err := b.api.GetPositions(ctx)
	if err != nil {
		return nil, fmt.Errorf("dhan get positions: %w", translateError(err))
	}
	out := make([]broker.Position, 0, len(positions))
	for _, p := range positions {
		buyAvg, _ := p.BuyAvg.Float64()
		realized, _ := p.RealizedProfit.Float64()
		unrealized, _ := p.UnrealizedProfit.Float64()

		exchange := "NSE"
		if strings.HasPrefix(p.ExchangeSegment, "BSE") {
			exchange = "BSE"
		} else if strings.HasPrefix(p.ExchangeSegment, "MCX") {
			exchange = "MCX"
		}
		if strings.Contains(p.ExchangeSegment, "FNO") && exchange == "NSE" {
			exchange = "NFO"
		}

		out = append(out, broker.Position{
			Exchange:      exchange,
			Tradingsymbol: p.TradingSymbol,
			Product:       p.ProductType,
			Quantity:      int(p.NetQty),
			AveragePrice:  buyAvg,
			LastPrice:     0,
			M2M:           unrealized,
			PnL:           realized + unrealized,
		})
	}
	return out, nil
}

// CancelOrder cancels a pending order. Cancellation must go via proxy.
func (b *Broker) CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("dhan cancel order: %w", domain.ErrUnproxiedNotAllowed)
	}

	resp, err := b.api.CancelOrder(ctx, orderID)
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("dhan cancel order: %w", translateError(err))
	}

	return broker.OrderResponse{OrderID: resp.OrderID}, nil
}
