package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrUnsupportedBroker is returned when no broker factory is registered for the requested broker name.
var ErrUnsupportedBroker = errors.New("unsupported broker")

// BrokerAccount carries the minimal credentials and proxy details required to instantiate a Broker.
type BrokerAccount struct {
	ID              string
	Role            string
	Broker          string
	BrokerAccountID string
	ApiKey          string
	ApiSecret       string
	AccessToken     string
	IPAddress       string
	ProxyHost       string
	ProxyPort       int
	ProxyUsername   string
	ProxyPassword   string
}

// Factory instantiates a Broker for a given account.
type Factory interface {
	CreateBroker(ctx context.Context, account BrokerAccount) (Broker, error)
}

// FactoryFunc is an adapter allowing the use of ordinary functions as broker factories.
type FactoryFunc func(ctx context.Context, account BrokerAccount) (Broker, error)

// CreateBroker calls f(ctx, account).
func (f FactoryFunc) CreateBroker(ctx context.Context, account BrokerAccount) (Broker, error) {
	return f(ctx, account)
}

// Registry dynamically manages broker factories keyed by broker name (case-insensitive).
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry initializes an empty Broker registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// Register maps a broker name (e.g. "kite", "zerodha", "testbroker") to a Factory.
func (r *Registry) Register(name string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(strings.TrimSpace(name))
	r.factories[key] = f
}

// Create resolves the Factory matching account.Broker and constructs a Broker.
// An empty account.Broker defaults to "zerodha".
func (r *Registry) Create(ctx context.Context, account BrokerAccount) (Broker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name := strings.ToLower(strings.TrimSpace(account.Broker))
	if name == "" {
		name = "zerodha"
	}

	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedBroker, account.Broker)
	}

	return factory.CreateBroker(ctx, account)
}
