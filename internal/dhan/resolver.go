package dhan

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan"
)

type resolvedSecurity struct {
	ExchangeSegment string
	SecurityID      string
	LotSize         int
}

type canonicalSymbol struct {
	Exchange      string
	Tradingsymbol string
}

// InstrumentResolver maintains in-memory lookup maps between EnvoyTrade canonical symbols
// and DhanHQ exchangeSegment/securityID tuples.
type InstrumentResolver struct {
	mu          sync.RWMutex
	symbolToSec map[string]resolvedSecurity
	secToSymbol map[string]canonicalSymbol
	logger      *slog.Logger
	scripURL    string
}

// NewInstrumentResolver creates a new initialized InstrumentResolver.
func NewInstrumentResolver(logger *slog.Logger) *InstrumentResolver {
	return &InstrumentResolver{
		symbolToSec: make(map[string]resolvedSecurity),
		secToSymbol: make(map[string]canonicalSymbol),
		logger:      logger,
		scripURL:    godhan.DefaultScripMasterURL,
	}
}

// SetScripURL overrides the default Dhan scrip master CSV download endpoint.
func (r *InstrumentResolver) SetScripURL(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripURL = url
}

func (r *InstrumentResolver) log() *slog.Logger {
	if r.logger != nil {
		return r.logger
	}
	return slog.Default()
}

// MapExchangeSegment converts Dhan's single-letter segment and exchange into Dhan's order ExchangeSegment enum.
func MapExchangeSegment(exchange, segment string) string {
	exch := strings.ToUpper(strings.TrimSpace(exchange))
	seg := strings.ToUpper(strings.TrimSpace(segment))

	switch exch {
	case "NSE":
		switch seg {
		case "D":
			return "NSE_FNO"
		case "C":
			return "NSE_CURRENCY"
		default:
			return "NSE_EQ"
		}
	case "BSE":
		switch seg {
		case "D":
			return "BSE_FNO"
		case "C":
			return "BSE_CURRENCY"
		default:
			return "BSE_EQ"
		}
	case "MCX":
		return "MCX_COMM"
	default:
		if seg == "D" {
			return exch + "_FNO"
		}
		return exch + "_EQ"
	}
}

// LoadFromReader parses a Dhan security master CSV stream and populates the lookup caches.
func (r *InstrumentResolver) LoadFromReader(reader io.Reader) error {
	instruments, err := godhan.ParseInstruments(reader)
	if err != nil {
		return fmt.Errorf("parse dhan instruments: %w", err)
	}

	symToSec := make(map[string]resolvedSecurity, len(instruments)*2)
	secToSym := make(map[string]canonicalSymbol, len(instruments))

	for _, inst := range instruments {
		exSegment := MapExchangeSegment(inst.Exchange, inst.Segment)
		lot := int(inst.LotSize)
		if lot <= 0 {
			lot = 1
		}

		sec := resolvedSecurity{
			ExchangeSegment: exSegment,
			SecurityID:      inst.SecurityID,
			LotSize:         lot,
		}

		rawSym := strings.TrimSpace(inst.TradingSymbol)
		cleanSym := strings.ToUpper(rawSym)
		cleanExch := strings.ToUpper(strings.TrimSpace(inst.Exchange))

		// 1. Direct key: "NSE:TATASTEEL"
		symToSec[cleanExch+":"+cleanSym] = sec

		// 2. Derivative alias: For NSE derivatives, map "NFO:SYMBOL" as well
		if inst.Segment == "D" && cleanExch == "NSE" {
			symToSec["NFO:"+cleanSym] = sec
		}
		if inst.Segment == "D" && cleanExch == "BSE" {
			symToSec["BFO:"+cleanSym] = sec
		}

		// Reverse mapping key: "NSE_EQ:11536"
		canonicalExch := cleanExch
		if inst.Segment == "D" && cleanExch == "NSE" {
			canonicalExch = "NFO"
		}
		secToSym[exSegment+":"+inst.SecurityID] = canonicalSymbol{
			Exchange:      canonicalExch,
			Tradingsymbol: rawSym,
		}
	}

	r.mu.Lock()
	r.symbolToSec = symToSec
	r.secToSymbol = secToSym
	r.mu.Unlock()

	r.log().Info("dhan: instrument resolver loaded", "count", len(instruments))
	return nil
}

// Load downloads the latest Dhan scrip master CSV and builds the index.
func (r *InstrumentResolver) Load(ctx context.Context) error {
	r.mu.RLock()
	url := r.scripURL
	r.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create scrip master request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch dhan scrip master: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http status %d fetching scrip master", resp.StatusCode)
	}

	return r.LoadFromReader(resp.Body)
}

// ResolveToSecurityID maps (exchange, tradingsymbol) to Dhan's (ExchangeSegment, SecurityID, lotSize).
func (r *InstrumentResolver) ResolveToSecurityID(exchange, tradingsymbol string) (string, string, int, error) {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	sym := strings.ToUpper(strings.TrimSpace(tradingsymbol))
	if ex == "" || sym == "" {
		return "", "", 0, fmt.Errorf("%w: empty exchange or symbol", domain.ErrNotFound)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Direct lookup
	key := ex + ":" + sym
	if sec, ok := r.symbolToSec[key]; ok {
		return sec.ExchangeSegment, sec.SecurityID, sec.LotSize, nil
	}

	// Fallback lookup: if exchange is NFO, try NSE; if NSE, try NFO
	if ex == "NFO" {
		if sec, ok := r.symbolToSec["NSE:"+sym]; ok {
			return sec.ExchangeSegment, sec.SecurityID, sec.LotSize, nil
		}
	} else if ex == "NSE" {
		if sec, ok := r.symbolToSec["NFO:"+sym]; ok {
			return sec.ExchangeSegment, sec.SecurityID, sec.LotSize, nil
		}
	}

	return "", "", 0, fmt.Errorf("%w: %s:%s", domain.ErrNotFound, exchange, tradingsymbol)
}

// ResolveToSymbol maps Dhan's (exchangeSegment, securityID) to canonical (exchange, tradingsymbol).
func (r *InstrumentResolver) ResolveToSymbol(exchangeSegment, securityID string) (string, string, error) {
	seg := strings.ToUpper(strings.TrimSpace(exchangeSegment))
	secID := strings.TrimSpace(securityID)
	if seg == "" || secID == "" {
		return "", "", fmt.Errorf("%w: empty segment or securityId", domain.ErrNotFound)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	key := seg + ":" + secID
	if sym, ok := r.secToSymbol[key]; ok {
		return sym.Exchange, sym.Tradingsymbol, nil
	}

	return "", "", fmt.Errorf("%w: %s:%s", domain.ErrNotFound, exchangeSegment, securityID)
}
