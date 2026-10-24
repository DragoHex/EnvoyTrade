package testbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"testbroker/sdk"

	"github.com/shopspring/decimal"
)

const defaultBaseURI = "http://localhost:8089"

// ProxyConfig allows testbroker to accept the same proxy structure as Kite.
type ProxyConfig struct {
	Scheme       string
	Host         string
	Port         int
	ClientID     string
	ClientSecret string
}

// Broker adapts testbroker/sdk.Client to broker.Broker.
type Broker struct {
	client  *sdk.Client
	baseURI string
	proxied bool
}

// NewBroker wraps an initialized sdk.Client.
func NewBroker(client *sdk.Client, baseURI string, proxied bool) *Broker {
	return &Broker{
		client:  client,
		baseURI: baseURI,
		proxied: proxied,
	}
}

// IsProxied reports whether this broker instance was configured with proxy details.
func (b *Broker) IsProxied() bool {
	return b.proxied
}

// NewLiveBroker creates a testbroker adapter instance from account credentials, optional baseURI, and optional proxy config.
// The mock trader accepts the mock proxy config for interface parity, but internally ignores it and makes unproxied calls to baseURI.
func NewLiveBroker(apiKey, accessToken, baseURI string, proxyCfg *ProxyConfig) *Broker {
	if baseURI == "" {
		baseURI = os.Getenv("TESTBROKER_URL")
	}
	if baseURI == "" {
		baseURI = defaultBaseURI
	}

	client := sdk.New(apiKey)
	client.SetAccessToken(accessToken)
	client.SetBaseURI(baseURI)

	proxied := (proxyCfg != nil && proxyCfg.Host != "")
	return NewBroker(client, baseURI, proxied)
}

// BaseURI returns the configured base URI for the mock server.
func (b *Broker) BaseURI() string {
	return b.baseURI
}

// PlaceOrder translates broker.OrderParams into sdk.OrderParams and places the order.
// Mutating order placement requires proxy configuration.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("testbroker place order: %w", domain.ErrUnproxiedNotAllowed)
	}
	resp, err := b.client.PlaceOrder(variety, sdk.OrderParams{
		Exchange:        params.Exchange,
		Tradingsymbol:   params.Tradingsymbol,
		TransactionType: params.TransactionType,
		Product:         params.Product,
		OrderType:       params.OrderType,
		Quantity:        params.Quantity,
		Price:           params.Price,
		TriggerPrice:    params.TriggerPrice,
		Tag:             params.Tag,
	})
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("testbroker place order: %w", err)
	}
	return broker.OrderResponse{OrderID: resp.OrderID}, nil
}

// GetPositions retrieves user net positions from testbroker mapped to broker.Position.
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	pos, err := b.client.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("testbroker get positions: %w", err)
	}
	out := make([]broker.Position, 0, len(pos.Net))
	for _, p := range pos.Net {
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

func toTestBrokerOrder(o sdk.Order) broker.Order {
	raw, _ := json.Marshal(o)
	var t time.Time
	if o.OrderTimestamp != "" {
		if parsed, err := time.Parse("2006-01-02 15:04:05", o.OrderTimestamp); err == nil {
			t = parsed
		} else if parsed, err := time.Parse(time.RFC3339, o.OrderTimestamp); err == nil {
			t = parsed
		}
	}
	return broker.Order{
		OrderID:         o.OrderID,
		Exchange:        o.Exchange,
		Tradingsymbol:   o.Tradingsymbol,
		TransactionType: o.TransactionType,
		Product:         o.Product,
		OrderType:       o.OrderType,
		Status:          o.Status,
		StatusMessage:   o.StatusMessage,
		Quantity:        o.Quantity,
		FilledQuantity:  o.FilledQuantity,
		Price:           decimal.NewFromFloat(o.Price),
		TriggerPrice:    decimal.NewFromFloat(o.TriggerPrice),
		AveragePrice:    decimal.NewFromFloat(o.AveragePrice),
		OrderTimestamp:  t,
		Tag:             o.Tag,
		RawPayload:      raw,
	}
}

// GetOrders retrieves all orders from testbroker mapped to broker.Order.
func (b *Broker) GetOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.client.GetOrders()
	if err != nil {
		return nil, fmt.Errorf("testbroker get orders: %w", err)
	}
	out := make([]broker.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, toTestBrokerOrder(o))
	}
	return out, nil
}

// GetOrderHistory retrieves history for a specific order.
func (b *Broker) GetOrderHistory(ctx context.Context, orderID string) ([]broker.Order, error) {
	orders, err := b.client.GetOrders()
	if err != nil {
		return nil, fmt.Errorf("testbroker get order history: %w", err)
	}
	var out []broker.Order
	for _, o := range orders {
		if o.OrderID == orderID {
			out = append(out, toTestBrokerOrder(o))
		}
	}
	return out, nil
}

// GetOpenOrders retrieves currently open orders from testbroker.
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.client.GetOrders()
	if err != nil {
		return nil, fmt.Errorf("testbroker get orders: %w", err)
	}
	var out []broker.Order
	for _, o := range orders {
		if o.Status == sdk.OrderStatusOpen {
			out = append(out, toTestBrokerOrder(o))
		}
	}
	return out, nil
}

// CancelOrder cancels an open order on testbroker. Mutating cancellation requires proxy configuration.
func (b *Broker) CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("testbroker cancel order: %w", domain.ErrUnproxiedNotAllowed)
	}
	if variety == "" {
		variety = sdk.VarietyRegular
	}
	resp, err := b.client.CancelOrder(variety, orderID, nil)
	if err != nil {
		return broker.OrderResponse{}, fmt.Errorf("testbroker cancel order: %w", err)
	}
	return broker.OrderResponse{OrderID: resp.OrderID}, nil
}

// Factory implements broker.Factory for the testbroker.
type Factory struct {
	baseURI string
}

// NewBrokerFactory creates a broker.Factory for testbroker accounts.
func NewBrokerFactory() broker.Factory {
	return &Factory{
		baseURI: os.Getenv("TESTBROKER_URL"),
	}
}

// CreateBroker builds a testbroker Broker for the account.
func (f *Factory) CreateBroker(ctx context.Context, account broker.BrokerAccount) (broker.Broker, error) {
	var proxyCfg *ProxyConfig
	if account.ProxyHost != "" {
		proxyCfg = &ProxyConfig{
			Scheme:       "https",
			Host:         account.ProxyHost,
			Port:         account.ProxyPort,
			ClientID:     account.ProxyUsername,
			ClientSecret: account.ProxyPassword,
		}
	}
	return NewLiveBroker(account.ApiKey, account.AccessToken, f.baseURI, proxyCfg), nil
}
