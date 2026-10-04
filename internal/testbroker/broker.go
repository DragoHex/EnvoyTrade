package testbroker

import (
	"context"
	"fmt"
	"os"

	"envoytrade/internal/broker"
	"testbroker/sdk"
)

const defaultBaseURI = "http://localhost:8089"

// Broker adapts testbroker/sdk.Client to broker.Broker.
type Broker struct {
	client  *sdk.Client
	baseURI string
}

// NewBroker wraps an initialized sdk.Client.
func NewBroker(client *sdk.Client, baseURI string) *Broker {
	return &Broker{
		client:  client,
		baseURI: baseURI,
	}
}

// NewLiveBroker creates a testbroker adapter instance from account credentials and optional baseURI.
func NewLiveBroker(apiKey, accessToken, baseURI string) *Broker {
	if baseURI == "" {
		baseURI = os.Getenv("TESTBROKER_URL")
	}
	if baseURI == "" {
		baseURI = defaultBaseURI
	}

	client := sdk.New(apiKey)
	client.SetAccessToken(accessToken)
	client.SetBaseURI(baseURI)

	return NewBroker(client, baseURI)
}

// BaseURI returns the configured base URI for the mock server.
func (b *Broker) BaseURI() string {
	return b.baseURI
}

// PlaceOrder translates broker.OrderParams into sdk.OrderParams and places the order.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
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
	return NewLiveBroker(account.ApiKey, account.AccessToken, f.baseURI), nil
}
