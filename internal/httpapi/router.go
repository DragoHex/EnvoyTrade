// Package httpapi is the stdlib net/http admin API backing the Dashboard
// page (docs/plans/UI-PLAN.md). Interfaces are declared here, the consumer,
// satisfied structurally by *postgres.Store and *engine.Engine — same
// seam rule as the rest of the repo (AGENTS.md "The seam rule").
package httpapi

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// Option configures optional router behaviors.
type Option func(*routerConfig)

type routerConfig struct {
	postbackHandler   http.Handler
	logger            *slog.Logger
	syncer            PortfolioSyncer
	tickerMgr         TickerManager
	squareOffSvc      SquareOffService
	rebalanceSvc      RebalanceService
	followerRegistrar FollowerRegistrar
	brokerEvicter     BrokerEvicter
	telemetry         TelemetryCollector
	proxyTester       ProxyTester
	executionFeed     ExecutionFeed
	staticFS          fs.FS
}

// ExecutionFeed supplies instantaneous master fill execution events.
type ExecutionFeed interface {
	Recent() []domain.ExecutionEvent
}

// FollowerRegistrar dynamically manages follower registration in the worker pool.
type FollowerRegistrar interface {
	RegisterFollower(ctx context.Context, followerID uuid.UUID) error
	UnregisterFollower(followerID uuid.UUID)
}

// PortfolioSyncer provides live portfolio synchronization for an account.
type PortfolioSyncer interface {
	SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error
}

// TickerManager manages dynamic start and stop of master WebSocket tickers.
type TickerManager interface {
	StartMaster(ctx context.Context, masterID uuid.UUID) error
	StopMaster(masterID uuid.UUID) error
}

// WithPostbackHandler mounts a webhook handler at POST /broker-callback.
func WithPostbackHandler(h http.Handler) Option {
	return func(c *routerConfig) {
		c.postbackHandler = h
	}
}

// WithPortfolioSyncer configures a portfolio syncer for automated login and sync actions.
func WithPortfolioSyncer(syncer PortfolioSyncer) Option {
	return func(c *routerConfig) {
		c.syncer = syncer
	}
}

// WithTickerManager configures a TickerManager for dynamic WebSocket ticker control.
func WithTickerManager(tm TickerManager) Option {
	return func(c *routerConfig) {
		c.tickerMgr = tm
	}
}

// WithSquareOffService configures a SquareOffService for portfolio square-off operations.
func WithSquareOffService(svc SquareOffService) Option {
	return func(c *routerConfig) {
		c.squareOffSvc = svc
	}
}

// WithRebalanceService configures a RebalanceService for portfolio rebalance operations.
func WithRebalanceService(svc RebalanceService) Option {
	return func(c *routerConfig) {
		c.rebalanceSvc = svc
	}
}

// BrokerEvicter evicts cached brokers on credential mutations or deletions.
type BrokerEvicter interface {
	Evict(accountID uuid.UUID)
}

// WithBrokerEvicter configures cache invalidation for broker instances.
func WithBrokerEvicter(evicter BrokerEvicter) Option {
	return func(c *routerConfig) {
		c.brokerEvicter = evicter
	}
}

// WithFollowerRegistrar configures dynamic follower worker pool registration.
func WithFollowerRegistrar(fr FollowerRegistrar) Option {
	return func(c *routerConfig) {
		c.followerRegistrar = fr
	}
}

// WithTelemetry configures in-memory pipeline telemetry.
func WithTelemetry(t TelemetryCollector) Option {
	return func(c *routerConfig) {
		c.telemetry = t
	}
}

// WithProxyTester configures a custom proxy connectivity tester.
func WithProxyTester(pt ProxyTester) Option {
	return func(c *routerConfig) {
		c.proxyTester = pt
	}
}

// WithExecutionFeed configures the in-memory execution event feed.
func WithExecutionFeed(feed ExecutionFeed) Option {
	return func(c *routerConfig) {
		c.executionFeed = feed
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
	h := &handlers{
		store:             store,
		engine:            actionEngine,
		syncer:            cfg.syncer,
		tickerMgr:         cfg.tickerMgr,
		squareOffSvc:      cfg.squareOffSvc,
		rebalanceSvc:      cfg.rebalanceSvc,
		followerRegistrar: cfg.followerRegistrar,
		brokerEvicter:     cfg.brokerEvicter,
		telemetry:         cfg.telemetry,
		proxyTester:       cfg.proxyTester,
		executionFeed:     cfg.executionFeed,
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/auth/register", h.postRegister)
	mux.HandleFunc("POST /api/v1/auth/login", h.postLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", h.postLogout)
	mux.HandleFunc("GET /api/v1/auth/me", h.getMe)
	mux.HandleFunc("PUT /api/v1/user/profile", h.putUserProfile)
	mux.HandleFunc("POST /api/v1/user/password", h.postUserPassword)

	mux.HandleFunc("GET /api/v1/groups", h.getGroups)
	mux.HandleFunc("POST /api/v1/groups", h.postGroup)
	mux.HandleFunc("GET /api/v1/groups/{id}", h.getGroupDetail)
	mux.HandleFunc("PATCH /api/v1/groups/{id}", h.patchGroup)
	mux.HandleFunc("DELETE /api/v1/groups/{id}", h.deleteGroup)
	mux.HandleFunc("POST /api/v1/groups/{id}/followers", h.postGroupFollower)
	mux.HandleFunc("POST /api/v1/groups/{id}/positions/square-off", h.postGroupSquareOff)
	mux.HandleFunc("GET /api/v1/groups/{id}/positions/rebalance/diff", h.getGroupRebalanceDiff)
	mux.HandleFunc("POST /api/v1/groups/{id}/positions/rebalance", h.postGroupRebalance)
	mux.HandleFunc("GET /api/v1/accounts", h.getAccounts)
	mux.HandleFunc("POST /api/v1/accounts", h.postAccount)
	mux.HandleFunc("PATCH /api/v1/accounts/{id}", h.patchAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", h.deleteAccount)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}/group", h.deleteAccountGroup)
	mux.HandleFunc("POST /api/v1/accounts/{id}/positions/square-off", h.postAccountSquareOff)
	mux.HandleFunc("GET /api/v1/accounts/{id}/positions/rebalance/diff", h.getAccountRebalanceDiff)
	mux.HandleFunc("POST /api/v1/accounts/{id}/positions/rebalance", h.postAccountRebalance)
	mux.HandleFunc("POST /api/v1/accounts/{id}/actions", h.postAction)
	mux.HandleFunc("GET /api/v1/accounts/{id}/orders", h.getAccountOrders)
	mux.HandleFunc("GET /api/v1/proxy-ips", h.getProxyIPs)
	mux.HandleFunc("GET /api/v1/proxy-ips/available", h.getAvailableProxyIPs)
	mux.HandleFunc("GET /api/v1/killswitch", h.getKillSwitchStatus)

	// Admin portal endpoints protected by requireAdminMiddleware
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /api/v1/admin/status", h.getAdminServiceStatus)
	adminMux.HandleFunc("POST /api/v1/admin/killswitch", h.postAdminKillSwitch)
	adminMux.HandleFunc("GET /api/v1/admin/broker-readiness", h.getAdminBrokerReadiness)
	adminMux.HandleFunc("GET /api/v1/admin/order-issues", h.getAdminOrderIssues)
	adminMux.HandleFunc("GET /api/v1/admin/execution-feed", h.getAdminExecutionFeed)

	adminMux.HandleFunc("GET /api/v1/admin/proxy-ips", h.getAdminProxyIPs)
	adminMux.HandleFunc("POST /api/v1/admin/proxy-ips/bulk", h.postAdminProxyIPsBulk)
	adminMux.HandleFunc("POST /api/v1/admin/proxy-ips", h.postAdminProxyIP)
	adminMux.HandleFunc("PUT /api/v1/admin/proxy-ips/{ip}", h.putAdminProxyIP)
	adminMux.HandleFunc("PATCH /api/v1/admin/proxy-ips/{ip}", h.patchAdminProxyIP)
	adminMux.HandleFunc("DELETE /api/v1/admin/proxy-ips/{ip}", h.deleteAdminProxyIP)
	adminMux.HandleFunc("POST /api/v1/admin/proxy-ips/{ip}/test", h.postAdminProxyIPTest)
	adminMux.HandleFunc("POST /api/v1/admin/proxy-ips/test-all", h.postAdminProxyIPsTestAll)

	adminMux.HandleFunc("GET /api/v1/admin/users", h.getAdminUsers)
	adminMux.HandleFunc("POST /api/v1/admin/users", h.postAdminUser)
	adminMux.HandleFunc("PATCH /api/v1/admin/users/{id}", h.patchAdminUser)
	adminMux.HandleFunc("GET /api/v1/admin/users/{id}/accounts", h.getAdminUserAccounts)
	adminMux.HandleFunc("GET /api/v1/admin/users/{id}/sessions", h.getAdminUserSessions)
	adminMux.HandleFunc("DELETE /api/v1/admin/users/{id}/sessions", h.deleteAdminUserSessions)

	mux.Handle("/api/v1/admin/", requireAdminMiddleware(adminMux, store))


	if cfg.postbackHandler != nil {
		mux.Handle("POST /broker-callback", cfg.postbackHandler)
		mux.Handle("POST /broker-callback/", cfg.postbackHandler)
	}

	if cfg.staticFS != nil {
		mux.Handle("/", spaHandler(cfg.staticFS))
	}

	var handler http.Handler = mux
	handler = authMiddleware(handler, store, cfg.logger)

	if cfg.logger != nil {
		handler = loggingMiddleware(handler, cfg.logger)
	}
	return recoveryMiddleware(handler, cfg.logger)
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

// AdminOperationalStore defines queries for live broker readiness and order exception auditing.
type AdminOperationalStore interface {
	BrokerReadiness(ctx context.Context) (domain.BrokerReadinessSummary, error)
	ListRecentOrderIssues(ctx context.Context, limit, offset int) ([]domain.OrderIssue, int, error)
}

// Store is everything the handlers need from persistence — the union of
// GroupsStore/AccountsStore/ActionsStore, kept as one interface since one
// concrete *postgres.Store always satisfies all three.
type Store interface {
	GroupsStore
	AccountsStore
	ActionsStore
	OrdersStore
	AuthStore
	ProxyStore
	AdminProxyStore
	AdminUserStore
	AdminOperationalStore
}

type handlers struct {
	store             Store
	engine            Engine
	syncer            PortfolioSyncer
	tickerMgr         TickerManager
	squareOffSvc      SquareOffService
	rebalanceSvc      RebalanceService
	followerRegistrar FollowerRegistrar
	brokerEvicter     BrokerEvicter
	telemetry         TelemetryCollector
	proxyTester       ProxyTester
	executionFeed     ExecutionFeed
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
