package instrument

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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
	mu           sync.RWMutex
	capacity     int
	entries      map[string]Info   // key: "EXCHANGE:TRADINGSYMBOL"
	canonicalMap map[string]Info   // key: "CANONICAL_KEY:TARGET_BROKER"
	revSec       map[string]string // key: "EXCHANGE_SEGMENT:SECURITY_ID" -> "EXCHANGE:TRADINGSYMBOL"
	store        Store
	logger       *slog.Logger
	lastAutoLoad time.Time
}

// NewCache constructs a Cache with pre-allocated map capacity.
func NewCache(capacity int, store Store, logger *slog.Logger) *Cache {
	if capacity <= 0 {
		capacity = 10000
	}
	return &Cache{
		capacity:     capacity,
		entries:      make(map[string]Info, capacity),
		canonicalMap: make(map[string]Info, capacity),
		revSec:       make(map[string]string, capacity),
		store:        store,
		logger:       logger,
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
	clear(c.canonicalMap)

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

		// Register canonical cross-broker index
		if contract, err := ParseCanonicalContract(inst.Exchange, inst.Tradingsymbol); err == nil {
			brokerTag := strings.ToLower(strings.TrimSpace(inst.Broker))
			for _, k := range contract.LookupKeys() {
				bKey := k + ":" + brokerTag
				if _, exists := c.canonicalMap[bKey]; !exists {
					c.canonicalMap[bKey] = info
				}
			}
		}

		// Preserve nearest expiry: since ActiveFOInstrumentsForCache orders by expiry ASC,
		// do not overwrite if key already exists.
		if _, exists := c.entries[key]; exists {
			continue
		}

		c.entries[key] = info
		loaded++
	}

	c.lastAutoLoad = time.Now().In(domain.IST)
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

func isUsableForBroker(tb string, info Info) bool {
	switch tb {
	case "kite", "zerodha":
		return info.Tradingsymbol != ""
	case "kotak", "kotakneo":
		return info.Tradingsymbol != ""
	case "dhan":
		return info.SecurityID != ""
	case "testbroker":
		return info.Tradingsymbol != ""
	case "":
		return info.SecurityID != "" || info.Tradingsymbol != ""
	default:
		return info.Tradingsymbol != "" || info.SecurityID != ""
	}
}

func defaultSegmentForBroker(tb, exchange string) string {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	switch tb {
	case "kotak", "kotakneo":
		switch ex {
		case "MCX", "MCX_FO", "MCX_COMM":
			return "mcx_fo"
		case "NFO", "NSE_FO", "NSE_FNO":
			return "nse_fo"
		case "BFO", "BSE_FO", "BSE_FNO":
			return "bse_fo"
		case "BSE", "BSE_CM":
			return "bse_cm"
		default:
			return "nse_cm"
		}
	case "kite", "zerodha":
		switch ex {
		case "MCX", "MCX_FO", "MCX_COMM":
			return "MCX"
		case "NFO", "NSE_FO", "NSE_FNO":
			return "NFO"
		case "BFO", "BSE_FO", "BSE_FNO":
			return "BFO"
		case "BSE", "BSE_CM":
			return "BSE"
		default:
			return "NSE"
		}
	case "dhan":
		switch ex {
		case "MCX", "MCX_FO", "MCX_COMM":
			return "MCX_COMM"
		case "NFO", "NSE_FO", "NSE_FNO":
			return "NSE_FNO"
		case "BFO", "BSE_FO", "BSE_FNO":
			return "BSE_FNO"
		case "BSE", "BSE_CM":
			return "BSE_EQ"
		default:
			return "NSE_EQ"
		}
	default:
		return ex
	}
}

func (c *Cache) enrichContractDay(contract *CanonicalContract) {
	if contract.HasDay || contract.IsEquity {
		return
	}
	// Probe canonicalMap across known brokers for month key
	for _, k := range contract.LookupKeys() {
		for _, b := range []string{"dhan", "kotak", "kite"} {
			if info, exists := c.canonicalMap[k+":"+b]; exists && info.Tradingsymbol != "" {
				if parsed, err := ParseCanonicalContract(contract.Exchange, info.Tradingsymbol); err == nil && parsed.HasDay {
					contract.ExpiryDate = parsed.ExpiryDate
					contract.HasDay = true
					return
				}
			}
		}
	}
}

// ResolveForBroker maps an incoming (exchange, tradingsymbol) from any master broker
// into the target broker's native parameters (ExchangeSegment, SecurityID, LotSize, TargetTradingsymbol).
func (c *Cache) ResolveForBroker(targetBroker, exchange, tradingsymbol string) (string, string, int, string, error) {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	rawSym := strings.TrimSpace(tradingsymbol)
	sym := strings.ToUpper(rawSym)
	tb := strings.ToLower(strings.TrimSpace(targetBroker))

	if ex == "" || sym == "" {
		return "", "", 0, "", fmt.Errorf("%w: empty exchange or symbol", domain.ErrNotFound)
	}

	key := ex + ":" + sym

	// 1. Direct in-memory check
	c.mu.RLock()
	info, ok := c.entries[key]
	if ok && (tb == "" || strings.ToLower(info.Broker) == tb) && isUsableForBroker(tb, info) {
		c.mu.RUnlock()
		exSeg := info.ExchangeSegment
		if exSeg == "" {
			exSeg = defaultSegmentForBroker(tb, ex)
		}
		return exSeg, info.SecurityID, info.LotSize, info.Tradingsymbol, nil
	}

	// 2. Canonical cross-broker in-memory lookup
	var fallbackLot int
	if contract, err := ParseCanonicalContract(ex, rawSym); err == nil {
		c.enrichContractDay(&contract)
		for _, k := range contract.LookupKeys() {
			canonKey := k + ":" + tb
			if cInfo, exists := c.canonicalMap[canonKey]; exists && isUsableForBroker(tb, cInfo) {
				c.mu.RUnlock()
				exSeg := cInfo.ExchangeSegment
				if exSeg == "" {
					exSeg = defaultSegmentForBroker(tb, ex)
				}
				return exSeg, cInfo.SecurityID, cInfo.LotSize, cInfo.Tradingsymbol, nil
			}
			for _, b := range []string{"dhan", "kite", "kotak"} {
				if anyInfo, exists := c.canonicalMap[k+":"+b]; exists && anyInfo.LotSize > 0 && fallbackLot == 0 {
					fallbackLot = anyInfo.LotSize
				}
			}
		}
	}

	// Secondary check for derivative aliases (e.g. NFO vs NSE, BFO vs BSE)
	if ex == "NFO" {
		if info, ok := c.entries["NSE:"+sym]; ok && isUsableForBroker(tb, info) {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, info.Tradingsymbol, nil
		}
	} else if ex == "NSE" {
		if info, ok := c.entries["NFO:"+sym]; ok && isUsableForBroker(tb, info) {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, info.Tradingsymbol, nil
		}
	} else if ex == "BFO" {
		if info, ok := c.entries["BSE:"+sym]; ok && isUsableForBroker(tb, info) {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, info.Tradingsymbol, nil
		}
	} else if ex == "BSE" {
		if info, ok := c.entries["BFO:"+sym]; ok && isUsableForBroker(tb, info) {
			c.mu.RUnlock()
			return info.ExchangeSegment, info.SecurityID, info.LotSize, info.Tradingsymbol, nil
		}
	}
	c.mu.RUnlock()

	// 3. Fall back to store if available
	if c.store != nil {
		// Try direct query
		secID, exSeg, lot, err := c.store.InstrumentSecurityID(context.Background(), ex, rawSym)
		if err == nil && secID != "" && isUsableForBroker(tb, Info{SecurityID: secID, Tradingsymbol: rawSym}) {
			c.mu.Lock()
			if len(c.entries) < c.capacity {
				c.entries[key] = Info{
					LotSize:         lot,
					SecurityID:      secID,
					ExchangeSegment: exSeg,
					Tradingsymbol:   rawSym,
					Broker:          tb,
				}
				if exSeg != "" && secID != "" {
					revKey := strings.ToUpper(strings.TrimSpace(exSeg)) + ":" + strings.TrimSpace(secID)
					c.revSec[revKey] = key
				}
			}
			c.mu.Unlock()
			if exSeg == "" {
				exSeg = defaultSegmentForBroker(tb, ex)
			}
			return exSeg, secID, lot, rawSym, nil
		}

		// Try canonical translated symbol query
		if contract, err := ParseCanonicalContract(ex, rawSym); err == nil {
			c.enrichContractDay(&contract)
			var altSym string
			if tb == "dhan" {
				altSym = contract.ToDhanSymbol()
			} else if tb == "kotak" || tb == "kotakneo" {
				altSym = contract.ToKotakSymbol()
			} else if tb == "kite" || tb == "zerodha" {
				altSym = contract.ToKiteSymbol()
			}

			if altSym != "" {
				if tb == "kite" || tb == "zerodha" {
					lot := fallbackLot
					if lot <= 0 {
						lot, _ = c.store.InstrumentLotSize(context.Background(), ex, altSym)
					}
					if lot <= 0 && altSym != rawSym {
						lot, _ = c.store.InstrumentLotSize(context.Background(), ex, rawSym)
					}
					if lot <= 0 && contract.IsEquity {
						lot = 1
					}
					targetInfo := Info{
						LotSize:         lot,
						ExchangeSegment: defaultSegmentForBroker(tb, ex),
						Tradingsymbol:   altSym,
						Broker:          tb,
					}
					c.mu.Lock()
					if len(c.entries) < c.capacity {
						c.entries[key] = targetInfo
						for _, k := range contract.LookupKeys() {
							c.canonicalMap[k+":"+tb] = targetInfo
						}
					}
					c.mu.Unlock()
					return targetInfo.ExchangeSegment, "", lot, altSym, nil
				}

				if tb == "kotak" || tb == "kotakneo" {
					lot := fallbackLot
					if lot <= 0 {
						lot, _ = c.store.InstrumentLotSize(context.Background(), ex, altSym)
					}
					if lot <= 0 && altSym != rawSym {
						lot, _ = c.store.InstrumentLotSize(context.Background(), ex, rawSym)
					}
					if lot <= 0 && contract.IsEquity {
						lot = 1
					}
					targetInfo := Info{
						LotSize:         lot,
						ExchangeSegment: defaultSegmentForBroker(tb, ex),
						Tradingsymbol:   altSym,
						Broker:          tb,
					}
					c.mu.Lock()
					if len(c.entries) < c.capacity {
						c.entries[key] = targetInfo
						for _, k := range contract.LookupKeys() {
							c.canonicalMap[k+":"+tb] = targetInfo
						}
					}
					c.mu.Unlock()
					return targetInfo.ExchangeSegment, "", lot, altSym, nil
				}

				if tb == "dhan" {
					secID, exSeg, lot, err := c.store.InstrumentSecurityID(context.Background(), ex, altSym)
					if err == nil && secID != "" {
						if exSeg == "" {
							exSeg = defaultSegmentForBroker(tb, ex)
						}
						c.mu.Lock()
						if len(c.entries) < c.capacity {
							targetInfo := Info{
								LotSize:         lot,
								SecurityID:      secID,
								ExchangeSegment: exSeg,
								Tradingsymbol:   altSym,
								Broker:          tb,
							}
							c.entries[key] = targetInfo
							for _, k := range contract.LookupKeys() {
								c.canonicalMap[k+":"+tb] = targetInfo
							}
						}
						c.mu.Unlock()
						return exSeg, secID, lot, altSym, nil
					}
				}
			}
		}
	} else {
		// Standalone algorithmic fallback (store is nil)
		if contract, err := ParseCanonicalContract(ex, rawSym); err == nil {
			c.enrichContractDay(&contract)
			if tb == "kotak" || tb == "kotakneo" {
				lot := fallbackLot
				if lot <= 0 && contract.IsEquity {
					lot = 1
				}
				return defaultSegmentForBroker(tb, ex), "", lot, contract.ToKotakSymbol(), nil
			}
			if tb == "kite" || tb == "zerodha" {
				lot := fallbackLot
				if lot <= 0 && contract.IsEquity {
					lot = 1
				}
				return defaultSegmentForBroker(tb, ex), "", lot, contract.ToKiteSymbol(), nil
			}
		}
	}

	return "", "", 0, "", fmt.Errorf("%w: %s:%s", domain.ErrNotFound, exchange, tradingsymbol)
}

// ResolveToSecurityID maps (exchange, tradingsymbol) to Dhan's (ExchangeSegment, SecurityID, lotSize).
func (c *Cache) ResolveToSecurityID(exchange, tradingsymbol string) (string, string, int, error) {
	exSeg, secID, lot, _, err := c.ResolveForBroker("dhan", exchange, tradingsymbol)
	return exSeg, secID, lot, err
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

// Size aliases Count for telemetry and status reporting.
func (c *Cache) Size() int {
	return c.Count()
}

// Capacity returns the maximum pre-allocated capacity of the cache.
func (c *Cache) Capacity() int {
	return c.capacity
}

// LastAutoLoad returns the timestamp of the last successful AutoLoadActiveFO.
func (c *Cache) LastAutoLoad() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastAutoLoad
}

