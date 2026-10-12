package broker

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Resolver resolves a Broker for an account ID.
type Resolver interface {
	ResolveBroker(ctx context.Context, accountID uuid.UUID) (Broker, error)
}

// ResolverFunc adapts a regular function to the Resolver interface.
type ResolverFunc func(ctx context.Context, accountID uuid.UUID) (Broker, error)

// ResolveBroker calls f(ctx, accountID).
func (f ResolverFunc) ResolveBroker(ctx context.Context, accountID uuid.UUID) (Broker, error) {
	return f(ctx, accountID)
}

// AccountPool caches instantiated Broker instances per account ID (Tier 2 cache).
// It is fully thread-safe for concurrent reads and writes, avoiding redundant
// credential lookups while allowing underlying HTTP connections to be reused.
type AccountPool struct {
	mu       sync.RWMutex
	brokers  map[uuid.UUID]Broker
	resolver Resolver
}

// NewAccountPool creates a new AccountPool with the given underlying resolver.
func NewAccountPool(resolver Resolver) *AccountPool {
	return &AccountPool{
		brokers:  make(map[uuid.UUID]Broker),
		resolver: resolver,
	}
}

// ResolveBroker returns the cached Broker for accountID, or lazily constructs and caches it.
func (p *AccountPool) ResolveBroker(ctx context.Context, accountID uuid.UUID) (Broker, error) {
	p.mu.RLock()
	b, ok := p.brokers[accountID]
	p.mu.RUnlock()
	if ok {
		return b, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Double-check after acquiring write lock
	if b, ok = p.brokers[accountID]; ok {
		return b, nil
	}

	fresh, err := p.resolver.ResolveBroker(ctx, accountID)
	if err != nil {
		return nil, err
	}
	p.brokers[accountID] = fresh
	return fresh, nil
}

// Evict removes an account's Broker from the cache (e.g. on token refresh or credential update).
func (p *AccountPool) Evict(accountID uuid.UUID) {
	p.mu.Lock()
	delete(p.brokers, accountID)
	p.mu.Unlock()
}

// EvictAll clears all cached brokers.
func (p *AccountPool) EvictAll() {
	p.mu.Lock()
	p.brokers = make(map[uuid.UUID]Broker)
	p.mu.Unlock()
}

// Count returns the number of currently cached brokers.
func (p *AccountPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.brokers)
}
