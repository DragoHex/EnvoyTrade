package broker

import (
	"context"
	"errors"
	"testing"
)

type dummyBroker struct {
	name string
}

func (d *dummyBroker) PlaceOrder(ctx context.Context, variety string, params OrderParams) (OrderResponse, error) {
	return OrderResponse{OrderID: d.name + "_123"}, nil
}

func TestRegistry_RegisterAndCreate(t *testing.T) {
	reg := NewRegistry()

	reg.Register("kite", FactoryFunc(func(ctx context.Context, acc BrokerAccount) (Broker, error) {
		return &dummyBroker{name: "kite"}, nil
	}))
	reg.Register("testbroker", FactoryFunc(func(ctx context.Context, acc BrokerAccount) (Broker, error) {
		return &dummyBroker{name: "testbroker"}, nil
	}))

	ctx := context.Background()

	t.Run("Create Kite Broker", func(t *testing.T) {
		b, err := reg.Create(ctx, BrokerAccount{Broker: "kite"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, _ := b.PlaceOrder(ctx, "regular", OrderParams{})
		if resp.OrderID != "kite_123" {
			t.Errorf("expected kite_123, got %s", resp.OrderID)
		}
	})

	t.Run("Create TestBroker", func(t *testing.T) {
		b, err := reg.Create(ctx, BrokerAccount{Broker: "testbroker"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, _ := b.PlaceOrder(ctx, "regular", OrderParams{})
		if resp.OrderID != "testbroker_123" {
			t.Errorf("expected testbroker_123, got %s", resp.OrderID)
		}
	})

	t.Run("Case Insensitivity", func(t *testing.T) {
		for _, name := range []string{"TestBroker", "TESTBROKER", "testbroker", "  testbroker  "} {
			b, err := reg.Create(ctx, BrokerAccount{Broker: name})
			if err != nil {
				t.Errorf("failed for %q: %v", name, err)
			}
			if b == nil {
				t.Errorf("nil broker for %q", name)
			}
		}
	})

	t.Run("Default Fallback to zerodha", func(t *testing.T) {
		reg.Register("zerodha", FactoryFunc(func(ctx context.Context, acc BrokerAccount) (Broker, error) {
			return &dummyBroker{name: "zerodha"}, nil
		}))

		b, err := reg.Create(ctx, BrokerAccount{Broker: ""})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, _ := b.PlaceOrder(ctx, "regular", OrderParams{})
		if resp.OrderID != "zerodha_123" {
			t.Errorf("expected zerodha_123, got %s", resp.OrderID)
		}
	})

	t.Run("Unsupported Broker", func(t *testing.T) {
		_, err := reg.Create(ctx, BrokerAccount{Broker: "some_unsupported_broker"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrUnsupportedBroker) {
			t.Errorf("expected ErrUnsupportedBroker, got %v", err)
		}
	})
}
