// Package httpapi is the stdlib net/http admin API backing the Dashboard
// page (docs/UI-PLAN.md). Interfaces are declared here, the consumer,
// satisfied structurally by *postgres.Store and *engine.Engine — same
// seam rule as the rest of the repo (AGENTS.md "The seam rule").
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Option configures optional router behaviors.
type Option func(*routerConfig)

type routerConfig struct {
	postbackHandler http.Handler
	logger          *slog.Logger
}

// WithPostbackHandler mounts a webhook handler at POST /broker-callback.
func WithPostbackHandler(h http.Handler) Option {
	return func(c *routerConfig) {
		c.postbackHandler = h
	}
}

// WithLogger configures structured request logging middleware.
func WithLogger(logger *slog.Logger) Option {
	return func(c *routerConfig) {
		c.logger = logger
	}
}

// NewRouter wires every Dashboard-page route onto a stdlib ServeMux using
// Go 1.22+ method+path pattern routing. If WithLogger option is supplied,
// requests are wrapped with structured access logging.
func NewRouter(store Store, actionEngine Engine, opts ...Option) http.Handler {
	var cfg routerConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	mux := http.NewServeMux()
	h := &handlers{store: store, engine: actionEngine}

	mux.HandleFunc("GET /api/v1/groups", h.getGroups)
	mux.HandleFunc("POST /api/v1/groups", h.postGroup)
	mux.HandleFunc("GET /api/v1/groups/{id}", h.getGroupDetail)
	mux.HandleFunc("PATCH /api/v1/groups/{id}", h.patchGroup)
	mux.HandleFunc("DELETE /api/v1/groups/{id}", h.deleteGroup)
	mux.HandleFunc("POST /api/v1/groups/{id}/followers", h.postGroupFollower)
	mux.HandleFunc("GET /api/v1/accounts", h.getAccounts)
	mux.HandleFunc("POST /api/v1/accounts", h.postAccount)
	mux.HandleFunc("PATCH /api/v1/accounts/{id}", h.patchAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", h.deleteAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}/group", h.deleteAccountGroup)
	mux.HandleFunc("POST /api/v1/accounts/{id}/actions", h.postAction)
	mux.HandleFunc("GET /api/v1/accounts/{id}/orders", h.getAccountOrders)

	if cfg.postbackHandler != nil {
		mux.Handle("POST /broker-callback", cfg.postbackHandler)
	}

	if cfg.logger != nil {
		return loggingMiddleware(mux, cfg.logger)
	}
	return mux
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration_ms", duration.Milliseconds(),
			"bytes", rec.bytes,
			"remote_ip", r.RemoteAddr,
		}

		if rec.statusCode >= 500 {
			logger.Error("http request error", attrs...)
		} else if rec.statusCode >= 400 {
			logger.Warn("http request warning", attrs...)
		} else {
			logger.Info("http request completed", attrs...)
		}
	})
}

// Store is everything the handlers need from persistence — the union of
// GroupsStore/AccountsStore/ActionsStore, kept as one interface since one
// concrete *postgres.Store always satisfies all three.
type Store interface {
	GroupsStore
	AccountsStore
	ActionsStore
	OrdersStore
}

type handlers struct {
	store  Store
	engine Engine
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
