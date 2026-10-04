// cmd/server is the process wiring for EnvoyTrade: the Dashboard HTTP
// API (internal/httpapi), Kite callback handler (internal/kite/callback),
// order-update listeners (internal/listener), fan-out engine (internal/engine),
// and reconciliation poller (internal/recon) backed by Postgres
// (internal/store/postgres).
//
// ponytail: follower accounts are registered with a kite/fake.Broker
// placeholder when no live Kite credentials are configured (see AGENTS.md).
// Swap this for a real kite.Broker once live sessions are established.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"envoytrade/internal/engine"
	"envoytrade/internal/httpapi"
	"envoytrade/internal/kite"
	"envoytrade/internal/kite/callback"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/listener"
	"envoytrade/internal/queue/memchan"
	"envoytrade/internal/recon"
	"envoytrade/internal/store/postgres"
	"envoytrade/internal/testbroker"
	"envoytrade/internal/ticker"
	"envoytrade/frontend"
	"envoytrade/internal/worker"

	"github.com/google/uuid"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

func main() {
	loadEnvFallback()

	logger, cleanup, err := setupLogger()
	if err != nil {
		log.Fatalf("logger setup failed: %v", err)
	}
	defer cleanup()

	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

// loadEnvFallback checks standard system and user locations for envoytrade.env
// if environment variables like DATABASE_URL are not already set.
func loadEnvFallback() {
	if os.Getenv("DATABASE_URL") != "" {
		return
	}

	var candidates []string
	if custom := os.Getenv("ENVOYTRADE_CONFIG"); custom != "" {
		candidates = append(candidates, custom)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".envoytrade", "envoytrade.env"))
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		candidates = append(candidates, filepath.Join(u.HomeDir, ".envoytrade", "envoytrade.env"))
	}
	candidates = append(candidates,
		"/etc/envoytrade/envoytrade.env",
		".env",
	)

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, rawLine := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(rawLine)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)
				if os.Getenv(k) == "" {
					os.Setenv(k, v)
				}
			}
		}
		if os.Getenv("DATABASE_URL") != "" {
			break
		}
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		return errors.New("ENCRYPTION_KEY is required")
	}
	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}

	logger.Info("starting server",
		"port", addr,
		"database", maskDatabaseURL(dbURL),
	)

	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	logger.Info("connected to postgres pool")

	store := postgres.New(pool)
	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	logger.Info("database migrations applied")

	brokerRegistry := broker.NewRegistry()
	kiteBrokerFactory := broker.FactoryFunc(func(ctx context.Context, acc broker.BrokerAccount) (broker.Broker, error) {
		var proxyCfg *kite.ProxyConfig
		if acc.ProxyHost != "" {
			proxyCfg = &kite.ProxyConfig{
				Scheme:       "https",
				Host:         acc.ProxyHost,
				Port:         acc.ProxyPort,
				ClientID:     acc.ProxyUsername,
				ClientSecret: acc.ProxyPassword,
			}
		}
		return kite.NewLiveBroker(acc.ApiKey, acc.AccessToken, proxyCfg)
	})
	brokerRegistry.Register("kite", kiteBrokerFactory)
	brokerRegistry.Register("zerodha", kiteBrokerFactory)
	brokerRegistry.Register("testbroker", testbroker.NewBrokerFactory())

	workerPool := worker.NewPool()
	workerPool.Logger = logger.With("component", "worker_pool")
	defer workerPool.Shutdown()
	followersRegistered, err := registerFollowers(ctx, store, workerPool, brokerRegistry)
	if err != nil {
		return fmt.Errorf("register followers: %w", err)
	}
	logger.Info("worker pool initialized", "followers_registered", followersRegistered)

	eng := engine.New(store, workerPool)
	eng.Logger = logger.With("component", "engine")

	// Queues for incoming signals & status updates
	masterFillQueue := memchan.New[domain.MasterFill](1024)
	orderUpdateQueue := memchan.New[domain.OrderUpdate](1024)

	syncer := &kite.PortfolioSyncer{
		Store: store,
	}
	workerPool.Syncer = syncer

	// Consumers draining queues into store and engine
	masterFillConsumer := &listener.MasterFillConsumer{
		Store:  store,
		Engine: eng,
		Syncer: syncer,
		Logger: logger.With("component", "master_fill_consumer"),
	}
	followerStatusConsumer := &listener.FollowerStatusConsumer{
		Store:  store,
		Syncer: syncer,
		Logger: logger.With("component", "follower_status_consumer"),
	}

	go func() {
		logger.Info("master fill consumer started")
		if err := masterFillConsumer.Run(ctx, masterFillQueue); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("master fill consumer stopped unexpectedly", "error", err)
		}
	}()
	go func() {
		logger.Info("follower status consumer started")
		if err := followerStatusConsumer.Run(ctx, orderUpdateQueue); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("follower status consumer stopped unexpectedly", "error", err)
		}
	}()

	// Postback HTTP handler (mounted at /broker-callback)
	postbackHandler := &callback.Handler{
		Accounts:        store,
		MasterFills:     masterFillQueue,
		FollowerUpdates: orderUpdateQueue,
		Logger:          logger.With("component", "postback"),
	}

	// Reconciliation poller
	poller := recon.NewPoller(
		store,
		&serverMasterReader{store: store},
		&serverFollowerReader{store: store},
		masterFillConsumer,
		nil,
		recon.DefaultConfig(),
		logger.With("component", "reconciler"),
	)
	go func() {
		logger.Info("reconciliation poller started")
		if err := poller.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("reconciliation poller stopped unexpectedly", "error", err)
		}
	}()

	// Dynamic multi-master WebSocket ticker manager (generic, multi-broker)
	tickerManager := ticker.NewManager(store, poller, logger.With("component", "ticker_manager"))
	kiteTickerFactory := kite.NewTickerFactory(masterFillQueue, logger.With("component", "kite_ticker"))
	testbrokerTickerFactory := testbroker.NewTickerFactory(masterFillQueue, logger.With("component", "testbroker_ticker"))
	tickerManager.RegisterFactory("kite", kiteTickerFactory)
	tickerManager.RegisterFactory("zerodha", kiteTickerFactory)
	tickerManager.RegisterFactory("testbroker", testbrokerTickerFactory)
	defer tickerManager.Shutdown()

	if err := tickerManager.SyncActiveMasters(ctx); err != nil {
		logger.Error("sync active masters tickers failed", "error", err)
	}

	// Auto-restart master ticker on headless token refresh
	syncer.OnTokenRefreshed = func(refreshCtx context.Context, accountID uuid.UUID) error {
		role, err := store.AccountRole(refreshCtx, accountID)
		if err == nil && role == "master" {
			return tickerManager.RestartMaster(refreshCtx, accountID)
		}
		return nil
	}

	// Periodic session expiration cleaner
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if deleted, err := store.DeleteExpiredSessions(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("failed to prune expired sessions", "error", err)
				} else if deleted > 0 {
					logger.Info("pruned expired sessions", "count", deleted)
				}
			}
		}
	}()

	// Periodic portfolio sync for active authenticated accounts to keep MTM and positions fresh
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				accs, err := store.Accounts(ctx, nil)
				if err != nil {
					continue
				}
				for _, a := range accs {
					if a.Active && a.AuthStatus == "authenticated" {
						_ = syncer.SyncAccountPortfolio(ctx, a.ID)
					}
				}
			}
		}
	}()

	var routerOpts []httpapi.Option
	routerOpts = append(routerOpts,
		httpapi.WithPostbackHandler(postbackHandler),
		httpapi.WithLogger(logger.With("component", "httpapi")),
		httpapi.WithPortfolioSyncer(syncer),
		httpapi.WithTickerManager(tickerManager),
	)

	if frontend.HasEmbeddedUI() {
		staticFS, err := frontend.Dist()
		if err != nil {
			logger.Error("failed to load embedded UI", "error", err)
		} else {
			logger.Info("mounting embedded UI")
			routerOpts = append(routerOpts, httpapi.WithStaticFS(staticFS))
		}
	} else {
		logger.Info("running in headless API mode (no embedded UI)")
	}

	router := httpapi.NewRouter(store, eng, routerOpts...)

	server := &http.Server{
		Addr:    ":" + addr,
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, draining connections")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", "error", err)
		} else {
			logger.Info("http server shutdown complete")
		}
	}()

	logger.Info("server listening", "addr", ":"+addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// registerFollowers registers every existing follower account with the
// worker pool so Rebalance can dispatch to it.
func registerFollowers(ctx context.Context, store *postgres.Store, pool *worker.Pool, registry *broker.Registry) (int, error) {
	groups, err := store.Groups(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, g := range groups {
		detail, err := store.GroupDetail(ctx, g.MasterID)
		if err != nil {
			return 0, err
		}
		for _, f := range detail.Followers {
			var b broker.Broker = &fake.Broker{}
			authInfo, err := store.AccountAuthInfo(ctx, f.AccountID)
			if err == nil && authInfo.ApiKey != "" && authInfo.AccessToken != "" {
				acc := broker.BrokerAccount{
					ID:              f.AccountID.String(),
					Broker:          authInfo.Broker,
					BrokerAccountID: authInfo.BrokerAccountID,
					ApiKey:          authInfo.ApiKey,
					ApiSecret:       authInfo.ApiSecret,
					AccessToken:     authInfo.AccessToken,
				}
				if authInfo.IPAddress != "" {
					if pIP, err := store.ProxyIPByAddress(ctx, authInfo.IPAddress); err == nil {
						acc.ProxyHost = pIP.Host
						acc.ProxyPort = pIP.Port
						acc.ProxyUsername = pIP.Username
						acc.ProxyPassword = pIP.Password
					}
				}
				if liveBroker, err := registry.Create(ctx, acc); err == nil {
					b = liveBroker
				}
			}
			pool.Register(ctx, f.AccountID, b, store, 16)
			count++
		}
	}
	return count, nil
}

type serverMasterReader struct {
	store *postgres.Store
}

func (r *serverMasterReader) GetMasterOrders(ctx context.Context, masterID uuid.UUID) ([]kiteconnect.Order, error) {
	authInfo, err := r.store.AccountAuthInfo(ctx, masterID)
	if err != nil {
		return nil, err
	}
	if authInfo.ApiKey == "" || authInfo.AccessToken == "" {
		return nil, nil
	}
	kc := kiteconnect.New(authInfo.ApiKey)
	kc.SetAccessToken(authInfo.AccessToken)
	if strings.EqualFold(authInfo.Broker, "testbroker") {
		tbURL := os.Getenv("TESTBROKER_URL")
		if tbURL == "" {
			tbURL = "http://localhost:8089"
		}
		kc.SetBaseURI(tbURL)
	}
	return kc.GetOrders()
}

type serverFollowerReader struct {
	store *postgres.Store
}

func (r *serverFollowerReader) GetFollowerOrderHistory(ctx context.Context, followerID uuid.UUID, brokerOrderID string) ([]kiteconnect.Order, error) {
	authInfo, err := r.store.AccountAuthInfo(ctx, followerID)
	if err != nil {
		return nil, err
	}
	if authInfo.ApiKey == "" || authInfo.AccessToken == "" {
		return nil, nil
	}
	kc := kiteconnect.New(authInfo.ApiKey)
	kc.SetAccessToken(authInfo.AccessToken)
	if strings.EqualFold(authInfo.Broker, "testbroker") {
		tbURL := os.Getenv("TESTBROKER_URL")
		if tbURL == "" {
			tbURL = "http://localhost:8089"
		}
		kc.SetBaseURI(tbURL)
	} else if authInfo.IPAddress != "" {
		pIP, err := r.store.ProxyIPByAddress(ctx, authInfo.IPAddress)
		if err == nil {
			proxyCfg := kite.ProxyConfig{
				Scheme:       "https",
				Host:         pIP.Host,
				Port:         pIP.Port,
				ClientID:     pIP.Username,
				ClientSecret: pIP.Password,
			}
			if client, err := kite.RESTClientFor(proxyCfg); err == nil {
				kc.SetHTTPClient(client)
			}
		}
	}
	return kc.GetOrderHistory(brokerOrderID)
}



// setupLogger initializes structured slog output to a file (default: /var/log/envoytrade/app.log)
// or falls back to stdout if unwritable.
func setupLogger() (*slog.Logger, func(), error) {
	logFilePath := os.Getenv("LOG_FILE")
	if logFilePath == "" {
		logFilePath = "/var/log/envoytrade/app.log"
	}

	var writer io.Writer
	cleanup := func() {}

	dir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot create log directory %q (%v), falling back to stdout\n", dir, err)
		writer = os.Stdout
	} else {
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot open log file %q (%v), falling back to stdout\n", logFilePath, err)
			writer = os.Stdout
		} else {
			cleanup = func() { _ = f.Close() }
			if os.Getenv("LOG_TO_STDOUT") == "true" {
				writer = io.MultiWriter(os.Stdout, f)
			} else {
				writer = f
			}
		}
	}

	// Parse level
	var level slog.Level
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "text" {
		handler = slog.NewTextHandler(writer, opts)
	} else {
		handler = slog.NewJSONHandler(writer, opts)
	}

	logger := slog.New(handler)
	return logger, cleanup, nil
}

// maskDatabaseURL masks credentials in postgres://user:pass@host:port/dbname.
func maskDatabaseURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	return u.Redacted()
}
