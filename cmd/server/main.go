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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/engine"
	"envoytrade/internal/httpapi"
	"envoytrade/internal/kite/callback"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/listener"
	"envoytrade/internal/queue/memchan"
	"envoytrade/internal/recon"
	"envoytrade/internal/store/postgres"
	"envoytrade/internal/worker"

	"github.com/google/uuid"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

func main() {
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

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is required")
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

	workerPool := worker.NewPool()
	workerPool.Logger = logger.With("component", "worker_pool")
	defer workerPool.Shutdown()
	followersRegistered, err := registerFollowers(ctx, store, workerPool)
	if err != nil {
		return fmt.Errorf("register followers: %w", err)
	}
	logger.Info("worker pool initialized", "followers_registered", followersRegistered)

	eng := engine.New(store, workerPool)
	eng.Logger = logger.With("component", "engine")

	// Queues for incoming signals & status updates
	masterFillQueue := memchan.New[domain.MasterFill](1024)
	orderUpdateQueue := memchan.New[domain.OrderUpdate](1024)

	// Consumers draining queues into store and engine
	masterFillConsumer := &listener.MasterFillConsumer{
		Store:  store,
		Engine: eng,
		Logger: logger.With("component", "master_fill_consumer"),
	}
	followerStatusConsumer := &listener.FollowerStatusConsumer{
		Store:  store,
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

	router := httpapi.NewRouter(
		store,
		eng,
		httpapi.WithPostbackHandler(postbackHandler),
		httpapi.WithLogger(logger.With("component", "httpapi")),
	)

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
func registerFollowers(ctx context.Context, store *postgres.Store, pool *worker.Pool) (int, error) {
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
			pool.Register(ctx, f.AccountID, &fake.Broker{}, store, 16)
			count++
		}
	}
	return count, nil
}

type serverMasterReader struct {
	store *postgres.Store
}

func (r *serverMasterReader) GetMasterOrders(ctx context.Context, masterID uuid.UUID) ([]kiteconnect.Order, error) {
	// Placeholder until live master session is authenticated
	return nil, nil
}

type serverFollowerReader struct {
	store *postgres.Store
}

func (r *serverFollowerReader) GetFollowerOrderHistory(ctx context.Context, followerID uuid.UUID, brokerOrderID string) ([]kiteconnect.Order, error) {
	// Placeholder until live follower session is authenticated
	return nil, nil
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
