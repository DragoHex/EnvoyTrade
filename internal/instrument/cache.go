package instrument

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"envoytrade/internal/domain"
)

// Store defines what the cache needs from the persistence layer.
type Store interface {
	ActiveFOInstrumentsForCache(ctx context.Context) ([]domain.Instrument, error)
	InstrumentLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error)
	InstrumentSecurityID(ctx context.Context, exchange, tradingsymbol string) (string, string, int, error)
}

// Info holds the cached metadata for a single instrument.
type Info struct {
	LotSize         int
	SecurityID      string
	ExchangeSegment string
	Tradingsymbol   string
	Broker          string
}

// Cache provides a thread-safe, pre-allocated in-memory cache for instrument metadata.
type Cache struct {
	mu       sync.RWMutex
	capacity int
	entries  map[string]Info   // key: "EXCHANGE:TRADINGSYMBOL"
	revSec   map[string]string // key: "EXCHANGE_SEGMENT:SECURITY_ID" -> "EXCHANGE:TRADINGSYMBOL"
	store    Store
	logger   *slog.Logger
}

// NewCache constructs a Cache with pre-allocated map capacity.
func NewCache(capacity int, store Store, logger *slog.Logger) *Cache {
	if capacity <= 0 {
		capacity = 10000
	}
	return &Cache{
		capacity: capacity,
		entries:  make(map[string]Info, capacity),
		revSec:   make(map[string]string, capacity),
		store:    store,
		logger:   logger,
	}
}

func (c *Cache) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// AutoLoadActiveFO queries the database for active F&O instruments and populates the cache.
// It resets the cache without releasing the pre-allocated backing memory.
func (c *Cache) AutoLoadActiveFO(ctx context.Context) error {
	if c.store == nil {
		return errors.New("cache store is nil")
	}

	instruments, err := c.store.ActiveFOInstrumentsForCache(ctx)
	if err != nil {
		return fmt.Errorf("auto-load active fo: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	clear(c.entries)
	clear(c.revSec)

	loaded := 0
	for _, inst := range instruments {
		if len(c.entries) >= c.capacity {
			c.log().Warn("instrument cache reached capacity limit during auto-load", "capacity", c.capacity)
			break
		}
		key := strings.ToUpper(strings.TrimSpace(inst.Exchange)) + ":" + strings.ToUpper(strings.TrimSpace(inst.Tradingsymbol))
		info := Info{
			LotSize:         inst.LotSize,
			SecurityID:      inst.SecurityID,
			ExchangeSegment: inst.ExchangeSegment,
			Tradingsymbol:   inst.Tradingsymbol,
			Broker:          inst.Broker,
		}

		// Always register reverse mapping for this security ID, even if another expiry already claimed the forward key
		if inst.ExchangeSegment != "" && inst.SecurityID != "" {
			revKey := strings.ToUpper(strings.TrimSpace(inst.ExchangeSegment)) + ":" + strings.TrimSpace(inst.SecurityID)
			c.revSec[revKey] = key
		}

		// Preserve nearest expiry: since ActiveFOInstrumentsForCache orders by expiry ASC,
		// do not overwrite if key already exists.
		if _, exists := c.entries[key]; exists {
			continue
		}

		c.entries[key] = info
		loaded++
	}

	c.log().Info("instrument cache auto-loaded", "loaded", loaded, "capacity", c.capacity)
	return nil
}

// ResolveLotSize resolves the canonical lot size for an (exchange, symbol).
// Hits return immediately via RLock; misses fall back to the store and cache the result.
func (c *Cache) ResolveLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error) {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	sym := strings.ToUpper(strings.TrimSpace(tradingsymbol))
	if ex == "" || sym == "" {
		return 0, fmt.Errorf("%w: empty exchange or symbol", domain.ErrNotFound)
	}
	key := ex + ":" + sym

	// 1. Fast RLock check (with derivative alias check)
	c.mu.RLock()
	info, ok := c.entries[key]
	if !ok {
		if ex == "NFO" {
			info, ok = c.entries["NSE:"+sym]
		} else if ex == "NSE" {
			info, ok = c.entries["NFO:"+sym]
		} else if ex == "BFO" {
			info, ok = c.entries["BSE:"+sym]
		} else if ex == "BSE" {
			info, ok = c.entries["BFO:"+sym]
		}
	}
	c.mu.RUnlock()
	if ok {
		if info.LotSize <= 0 {
			return 0, domain.ErrNotFound
		}
		return info.LotSize, nil
	}

	// 2. Fall back to store
	if c.store == nil {
		return 0, domain.ErrNotFound
	}
	lotSize, err := c.store.InstrumentLotSize(ctx, ex, sym)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.mu.Lock()
			if len(c.entries) < c.capacity {
				c.entries[key] = Info{LotSize: 0}
			}
			c.mu.Unlock()
			return 0, domain.ErrNotFound
		}
		return 0, err
	}

	// 3. Cache the resolved lot size
	c.mu.Lock()
	if len(c.entries) < c.capacity {
		c.entries[key] = Info{LotSize: lotSize}
	}
	c.mu.Unlock()

	return lotSize, nil
}

// ResolveToSecurityID maps (exchange, tradingsymbol) to Dhan's (ExchangeSegment, SecurityID, lotSize).
func (c *Cache) ResolveToSecurityID(exchange, tradingsymbol string) (string, string, int, error) {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	rawSym := strings.TrimSpace(tradingsymbol)
	sym := strings.ToUpper(rawSym)
	if ex == "" || sym == "" {
		return "", "", 0, fmt.Errorf("%w: empty exchange or symbol", domain.ErrNotFound)
	}

	key := ex + ":" + sym

	// 1. Fast in-memory check
	c.mu.RLock()
	info, ok := c.entries[key]
	if ok && info.SecurityID != "" {
		c.mu.RUnlock()
		return info.ExchangeSegment, info.SecurityID, info.LotSize, nil
	}

	// Secondary check for derivative aliases (e.g. NFO vs NSE, BFO vs BSE)
	if ex == "NFO" {
		if info, ok := c.entries["NSE:"+sym]; ok && info.SecurityID != "" {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, nil
		}
	} else if ex == "NSE" {
		if info, ok := c.entries["NFO:"+sym]; ok && info.SecurityID != "" {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, nil
		}
	} else if ex == "BFO" {
		if info, ok := c.entries["BSE:"+sym]; ok && info.SecurityID != "" {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, nil
		}
	} else if ex == "BSE" {
		if info, ok := c.entries["BFO:"+sym]; ok && info.SecurityID != "" {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, nil
		}
	}
	c.mu.RUnlock()

	// 2. Fall back to store if available
	if c.store != nil {
		secID, exSeg, lot, err := c.store.InstrumentSecurityID(context.Background(), ex, rawSym)
		if err == nil && secID != "" {
			c.mu.Lock()
			if len(c.entries) < c.capacity {
				c.entries[key] = Info{
					LotSize:         lot,
					SecurityID:      secID,
					ExchangeSegment: exSeg,
					Tradingsymbol:   rawSym,
				}
				if exSeg != "" && secID != "" {
					revKey := strings.ToUpper(strings.TrimSpace(exSeg)) + ":" + strings.TrimSpace(secID)
					c.revSec[revKey] = key
				}
			}
			c.mu.Unlock()
			return exSeg, secID, lot, nil
		}
	}

	return "", "", 0, fmt.Errorf("%w: %s:%s", domain.ErrNotFound, exchange, tradingsymbol)
}

// ResolveToSymbol maps Dhan's (exchangeSegment, securityID) to canonical (exchange, tradingsymbol).
func (c *Cache) ResolveToSymbol(exchangeSegment, securityID string) (string, string, error) {
	seg := strings.ToUpper(strings.TrimSpace(exchangeSegment))
	secID := strings.TrimSpace(securityID)
	if seg == "" || secID == "" {
		return "", "", fmt.Errorf("%w: empty segment or securityId", domain.ErrNotFound)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	revKey := seg + ":" + secID
	if canonKey, ok := c.revSec[revKey]; ok {
		parts := strings.SplitN(canonKey, ":", 2)
		if len(parts) == 2 {
			if info, ok := c.entries[canonKey]; ok && info.Tradingsymbol != "" {
				return parts[0], info.Tradingsymbol, nil
			}
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("%w: %s:%s", domain.ErrNotFound, exchangeSegment, securityID)
}

// Count returns the number of currently cached instruments.
func (c *Cache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Capacity returns the maximum pre-allocated capacity of the cache.
func (c *Cache) Capacity() int {
	return c.capacity
}
