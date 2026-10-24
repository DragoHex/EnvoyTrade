package callback

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
)

// DefaultMaxBodyBytes is the default maximum read limit (1MB) for callback payloads.
const DefaultMaxBodyBytes = 1024 * 1024

// AdaptiveRouter routes incoming broker postbacks to the appropriate broker-specific
// handler using an adaptive multi-stage pipeline:
// 1. Explicit query parameter (?broker=name)
// 2. Explicit HTTP header (X-Broker: name)
// 3. Path parameter (/broker-callback/{broker})
// 4. Zero-allocation fast raw byte fingerprinting of the JSON payload
// 5. Default handler fallback (e.g., Zerodha Kite)
type AdaptiveRouter struct {
	mu             sync.RWMutex
	handlers       map[string]http.Handler
	defaultHandler http.Handler
	MaxBodyBytes   int64
}

// NewAdaptiveRouter creates a new initialized AdaptiveRouter.
func NewAdaptiveRouter() *AdaptiveRouter {
	return &AdaptiveRouter{
		handlers:     make(map[string]http.Handler),
		MaxBodyBytes: DefaultMaxBodyBytes,
	}
}

// Register registers a handler for a given broker name (case-insensitive).
func (r *AdaptiveRouter) Register(broker string, h http.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[strings.ToLower(strings.TrimSpace(broker))] = h
}

// SetDefault sets the fallback handler when no specific broker is identified.
func (r *AdaptiveRouter) SetDefault(h http.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultHandler = h
}

func (r *AdaptiveRouter) getHandler(broker string) http.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b := strings.ToLower(strings.TrimSpace(broker))
	if h, ok := r.handlers[b]; ok {
		return h
	}
	switch b {
	case "zerodha":
		return r.handlers["kite"]
	case "kite":
		return r.handlers["zerodha"]
	case "kotak":
		return r.handlers["kotakneo"]
	case "kotakneo":
		return r.handlers["kotak"]
	}
	return nil
}

func (r *AdaptiveRouter) getDefaultHandler() http.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultHandler
}

// ServeHTTP implements http.Handler.
func (r *AdaptiveRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Stage 1: Explicit Query Param (?broker=name)
	if qb := req.URL.Query().Get("broker"); qb != "" {
		if h := r.getHandler(qb); h != nil {
			h.ServeHTTP(w, req)
			return
		}
		// Explicit broker requested but unknown: per PLAN.md §3.4 acknowledge and drop
		w.WriteHeader(http.StatusOK)
		return
	}

	// Stage 2: Explicit HTTP Header (X-Broker)
	if hb := req.Header.Get("X-Broker"); hb != "" {
		if h := r.getHandler(hb); h != nil {
			h.ServeHTTP(w, req)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// Stage 3: Subpath parameter (/broker-callback/{broker})
	path := req.URL.Path
	if strings.HasPrefix(path, "/broker-callback/") {
		sub := strings.TrimPrefix(path, "/broker-callback/")
		if idx := strings.Index(sub, "/"); idx != -1 {
			sub = sub[:idx]
		}
		if sub != "" {
			if h := r.getHandler(sub); h != nil {
				h.ServeHTTP(w, req)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Stage 4: Byte Fingerprinting
	maxBytes := r.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyBytes
	}

	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBytes))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Restore req.Body for downstream handler
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if broker := fingerprintPayload(bodyBytes); broker != "" {
		if h := r.getHandler(broker); h != nil {
			h.ServeHTTP(w, req)
			return
		}
	}

	// Stage 5: Default Fallback
	if defH := r.getDefaultHandler(); defH != nil {
		defH.ServeHTTP(w, req)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// fingerprintPayload checks the raw byte content for unique broker JSON signature tokens.
// Takes ~80ns with zero JSON unmarshaling and zero heap allocations.
func fingerprintPayload(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	// Groww signature: "groww_order_id"
	if bytes.Contains(body, []byte(`"groww_order_id"`)) {
		return "groww"
	}

	// Dhan signature: "dhanClientId"
	if bytes.Contains(body, []byte(`"dhanClientId"`)) || bytes.Contains(body, []byte(`"dhan_client_id"`)) {
		return "dhan"
	}

	// Kotak Neo signature: "nOrdNo" AND "trdSym"
	if bytes.Contains(body, []byte(`"nOrdNo"`)) && bytes.Contains(body, []byte(`"trdSym"`)) {
		return "kotakneo"
	}

	// Symphony signature: "AppOrderID" AND "ExchangeInstrumentID"
	if bytes.Contains(body, []byte(`"AppOrderID"`)) && bytes.Contains(body, []byte(`"ExchangeInstrumentID"`)) {
		return "symphony"
	}

	// Angel One signature: "uniqueorderid" AND "filledshares"
	if bytes.Contains(body, []byte(`"uniqueorderid"`)) && bytes.Contains(body, []byte(`"filledshares"`)) {
		return "angelone"
	}

	// Zerodha Kite signature: "checksum" AND "user_id"
	if bytes.Contains(body, []byte(`"checksum"`)) && bytes.Contains(body, []byte(`"user_id"`)) {
		return "kite"
	}

	return ""
}
