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
	"flag"
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

	"envoytrade/frontend"
	"envoytrade/internal/archive"
	"envoytrade/internal/broker"
	unifiedcallback "envoytrade/internal/callback"
	"envoytrade/internal/dhan"
	dhancallback "envoytrade/internal/dhan/callback"
	dhanws "envoytrade/internal/dhan/ws"
	"envoytrade/internal/domain"
	"envoytrade/internal/engine"
	"envoytrade/internal/httpapi"
	"envoytrade/internal/instrument"
	"envoytrade/internal/kite"
	"envoytrade/internal/kite/callback"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/listener"
	"envoytrade/internal/queue/memchan"
	"envoytrade/internal/rebalance"
	"envoytrade/internal/recon"
	"envoytrade/internal/squareoff"
	"envoytrade/internal/store/postgres"
	"envoytrade/internal/telemetry"
	"envoytrade/internal/testbroker"
	"envoytrade/internal/ticker"
	"envoytrade/internal/worker"

	"github.com/google/uuid"
)

// Version is the application version, set at compile time via -ldflags.
var Version = "dev"

func main() {
	loadEnvFallback()

	logger, cleanup, err := setupLogger()
	if err != nil {
		log.Fatalf("logger setup failed: %v", err)
	}
	defer cleanup()

	slog.SetDefault(logger)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "archive":
			if err := runArchive(logger, os.Args[2:]); err != nil {
				logger.Error("archive command failed", "error", err)
				os.Exit(1)
			}
			return
		case "restore":
			if err := runRestore(logger, os.Args[2:]); err != nil {
				logger.Error("restore command failed", "error", err)
				os.Exit(1)
			}
			return
		case "sync-instruments":
			if err := runSyncInstruments(logger, os.Args[2:]); err != nil {
				logger.Error("sync-instruments command failed", "error", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(logger); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func runArchive(logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	var (
		cutoffStr  = fs.String("cutoff", "", "Cutoff date/time in YYYY-MM-DD or RFC3339 format (defaults to yesterday 23:59:59 IST)")
		archiveDir = fs.String("dir", "/var/backups/envoytrade/archive", "Base directory for Parquet archives")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.UTC
	}

	var cutoff time.Time
	if *cutoffStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", *cutoffStr, loc); err == nil {
			cutoff = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, loc)
		} else if t, err := time.Parse(time.RFC3339, *cutoffStr); err == nil {
			cutoff = t
		} else {
			return fmt.Errorf("invalid cutoff date format %q: use YYYY-MM-DD or RFC3339", *cutoffStr)
		}
	} else {
		now := time.Now().In(loc)
		yesterday := now.AddDate(0, 0, -1)
		cutoff = time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 999999999, loc)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	archiver := archive.NewArchiver(pool, logger)
	summary, err := archiver.Archive(ctx, cutoff, *archiveDir)
	if err != nil {
		return err
	}

	fmt.Printf("Archived: %d follower orders, %d master fills, %d order events into %s\n",
		summary.FollowerOrdersArchived, summary.MasterFillsArchived, summary.OrderEventsArchived, *archiveDir)
	return nil
}

func runRestore(logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	var (
		archiveDir = fs.String("dir", "/var/backups/envoytrade/archive", "Base directory for Parquet archives")
		fromStr    = fs.String("from", "", "Start date (inclusive) in YYYY-MM-DD format")
		uptoStr    = fs.String("upto", "", "End date (inclusive) in YYYY-MM-DD format")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.UTC
	}

	var fromDate, uptoDate *time.Time
	if *fromStr != "" {
		t, err := time.ParseInLocation("2006-01-02", *fromStr, loc)
		if err != nil {
			return fmt.Errorf("invalid --from format %q: use YYYY-MM-DD", *fromStr)
		}
		f := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		fromDate = &f
	}
	if *uptoStr != "" {
		t, err := time.ParseInLocation("2006-01-02", *uptoStr, loc)
		if err != nil {
			return fmt.Errorf("invalid --upto format %q: use YYYY-MM-DD", *uptoStr)
		}
		u := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, loc)
		uptoDate = &u
	}
	if fromDate != nil && uptoDate != nil && fromDate.After(*uptoDate) {
		return fmt.Errorf("invalid date range: --from (%s) cannot be after --upto (%s)", *fromStr, *uptoStr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	restorer := archive.NewRestorer(pool, logger)
	summary, err := restorer.Restore(ctx, *archiveDir, fromDate, uptoDate)
	if err != nil {
		return err
	}

	fmt.Printf("Restored: %d master fills, %d follower orders, %d order events from %d partition(s)\n",
		summary.MasterFillsRestored, summary.FollowerOrdersRestored, summary.OrderEventsRestored, len(summary.PartitionsRestored))
	return nil
}

type instrumentSyncRunner interface {
	SyncDhan(ctx context.Context) error
	SyncKite(ctx context.Context, exchanges []string) error
}

var newInstrumentSyncer = func(store instrument.SyncerStore, logger *slog.Logger) instrumentSyncRunner {
	return instrument.NewSyncer(store, logger)
}

var connectPostgres = func(ctx context.Context, dbURL string) (*postgres.Store, func(), error) {
	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		return nil, nil, err
	}
	return postgres.New(pool), pool.Close, nil
}

func runSyncInstruments(logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("sync-instruments", flag.ContinueOnError)
	var (
		brokerChoice = fs.String("broker", "all", "Broker to sync: 'all', 'dhan', or 'kite'")
		exchangesStr = fs.String("exchanges", "MCX,NFO,BFO", "Comma-separated exchanges for Kite")
		timeoutDur   = fs.Duration("timeout", 10*time.Minute, "Overall sync timeout")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	choice := strings.ToLower(strings.TrimSpace(*brokerChoice))
	syncDhan := choice == "all" || choice == "dhan"
	syncKite := choice == "all" || choice == "kite"

	if !syncDhan && !syncKite {
		return fmt.Errorf("invalid broker choice %q: must be 'all', 'dhan', or 'kite'", *brokerChoice)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutDur)
	defer cancel()

	store, cleanup, err := connectPostgres(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer cleanup()

	if store != nil {
		if err := store.Migrate(ctx); err != nil {
			return fmt.Errorf("migrate db: %w", err)
		}
	}

	var syncerStore instrument.SyncerStore
	if store != nil {
		syncerStore = store
	}
	syncer := newInstrumentSyncer(syncerStore, logger)

	if syncDhan {
		logger.Info("sync-instruments: syncing Dhan scrip master...")
		start := time.Now()
		if err := syncer.SyncDhan(ctx); err != nil {
			return fmt.Errorf("sync dhan instruments: %w", err)
		}
		logger.Info("sync-instruments: Dhan sync completed", "duration", time.Since(start))
	}

	if syncKite {
		var exchanges []string
		for _, e := range strings.Split(*exchangesStr, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				exchanges = append(exchanges, e)
			}
		}
		logger.Info("sync-instruments: syncing Kite instruments...", "exchanges", exchanges)
		start := time.Now()
		if err := syncer.SyncKite(ctx, exchanges); err != nil {
			return fmt.Errorf("sync kite instruments: %w", err)
		}
		logger.Info("sync-instruments: Kite sync completed", "duration", time.Since(start))
	}

	logger.Info("sync-instruments: successfully synced all requested instruments")
	return nil
}

// loadEnvFallback checks standard system and user locations for envoytrade.env
// if environment variables like DATABASE_URL or ENCRYPTION_KEY are not already set.
func loadEnvFallback() {
	if os.Getenv("DATABASE_URL") != "" && os.Getenv("ENCRYPTION_KEY") != "" {
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
		if os.Getenv("DATABASE_URL") != "" && os.Getenv("ENCRYPTION_KEY") != "" {
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

	logProcessLimits(logger)

	logger.Info("starting server",
		"version", Version,
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

	// Pre-allocated 10,000 capacity unified instrument cache
	instrumentCache := instrument.NewCache(10000, store, logger.With("component", "instrument_cache"))
	if err := instrumentCache.AutoLoadActiveFO(ctx); err != nil {
		logger.Warn("auto-load active fo instruments failed on boot", "error", err)
	}

	// Instrument Syncer for daily 05:00 AM IST ingestion
	instrumentSyncer := instrument.NewSyncer(store, logger.With("component", "instrument_syncer"))
	instrumentSyncer.StartDailyRoutine(ctx, 5, 0, func(c context.Context) error {
		return instrumentCache.AutoLoadActiveFO(c)
	})

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
	dhanBrokerFactory := broker.FactoryFunc(func(ctx context.Context, acc broker.BrokerAccount) (broker.Broker, error) {
		var proxyCfg *dhan.ProxyConfig
		if acc.ProxyHost != "" {
			proxyCfg = &dhan.ProxyConfig{
				Scheme:       "https",
				Host:         acc.ProxyHost,
				Port:         acc.ProxyPort,
				ClientID:     acc.ProxyUsername,
				ClientSecret: acc.ProxyPassword,
			}
		}
		clientID := acc.BrokerAccountID
		if clientID == "" {
			clientID = acc.ApiKey
		}
		return dhan.NewLiveBroker(clientID, acc.AccessToken, proxyCfg, instrumentCache)
	})
	brokerRegistry.Register("kite", kiteBrokerFactory)
	brokerRegistry.Register("zerodha", kiteBrokerFactory)
	brokerRegistry.Register("dhan", dhanBrokerFactory)
	brokerRegistry.Register("testbroker", testbroker.NewBrokerFactory())

	rawKiteSyncer := &kite.PortfolioSyncer{
		Store: store,
	}
	rawDhanSyncer := &dhan.PortfolioSyncer{
		Store: store,
	}
	multiSyncer := &multiBrokerSyncer{
		store:      store,
		kiteSyncer: rawKiteSyncer,
		dhanSyncer: rawDhanSyncer,
	}
	syncer := kite.NewDebouncedSyncer(multiSyncer, 500*time.Millisecond)

	brokerResolver := &serverBrokerResolver{
		store:    store,
		registry: brokerRegistry,
		syncer:   multiSyncer,
	}
	accountPool := broker.NewAccountPool(brokerResolver)
	evicter := accountPool

	workerPool := worker.NewPool()
	workerPool.Logger = logger.With("component", "worker_pool")
	workerPool.Syncer = syncer
	defer workerPool.Shutdown()
	followersRegistered, err := registerFollowers(ctx, store, workerPool, accountPool, logger)
	if err != nil {
		return fmt.Errorf("register followers: %w", err)
	}
	logger.Info("worker pool initialized", "followers_registered", followersRegistered)

	followerRegistrar := &workerFollowerRegistrar{
		pool:     workerPool,
		resolver: accountPool,
		store:    store,
		logger:   logger.With("component", "follower_registrar"),
	}

	squareOffSvc := squareoff.NewService(store, accountPool, syncer, logger.With("component", "squareoff"))
	rebalanceSvc := rebalance.NewService(store, accountPool, syncer, logger.With("component", "rebalance"))

	eng := engine.New(store, workerPool)
	eng.SetLotSizeResolver(instrumentCache)
	eng.Logger = logger.With("component", "engine")
	execFeed := telemetry.NewExecutionFeed(25)
	eng.ExecutionRecorder = execFeed

	// Daily lot size cache cleanup at 01:00:00 IST
	go func() {
		istLocation, locErr := time.LoadLocation("Asia/Kolkata")
		if locErr != nil {
			logger.Warn("could not load Asia/Kolkata timezone, fallback to UTC+5:30 fixed zone", "error", locErr)
			istLocation = time.FixedZone("IST", 5*3600+1800)
		}
		for {
			now := time.Now().In(istLocation)
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), 1, 0, 0, 0, istLocation)
			if !now.Before(nextRun) {
				nextRun = nextRun.Add(24 * time.Hour)
			}
			waitDur := time.Until(nextRun)
			timer := time.NewTimer(waitDur)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				eng.ClearLotSizeCache()
				_ = instrumentCache.AutoLoadActiveFO(context.Background())
				logger.Info("daily lot size cache cleared at 01:00:00 IST")
			}
		}
	}()

	// Queues for incoming signals & status updates
	masterFillQueue := memchan.New[domain.MasterFill](1024)
	orderUpdateQueue := memchan.New[domain.OrderUpdate](1024)

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
		defer func() {
			if r := recover(); r != nil {
				logger.Error("master fill consumer panicked", "panic", r)
			}
		}()
		logger.Info("master fill consumer started")
		if err := masterFillConsumer.Run(ctx, masterFillQueue); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("master fill consumer stopped unexpectedly", "error", err)
		}
	}()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("follower status consumer panicked", "panic", r)
			}
		}()
		logger.Info("follower status consumer started")
		if err := followerStatusConsumer.Run(ctx, orderUpdateQueue); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("follower status consumer stopped unexpectedly", "error", err)
		}
	}()

	// Postback HTTP handler (mounted at /broker-callback and /broker-callback/)
	kitePostback := &callback.Handler{
		Accounts:        store,
		MasterFills:     masterFillQueue,
		FollowerUpdates: orderUpdateQueue,
		Logger:          logger.With("component", "kite_postback"),
	}

	postbackRouter := unifiedcallback.NewAdaptiveRouter()
	postbackRouter.Register("kite", kitePostback)
	postbackRouter.Register("zerodha", kitePostback)

	dhanPostback := &dhancallback.Handler{
		Accounts:        store,
		MasterFills:     masterFillQueue,
		FollowerUpdates: orderUpdateQueue,
		Resolver:        instrumentCache,
		Logger:          logger.With("component", "dhan_postback"),
	}
	postbackRouter.Register("dhan", dhanPostback)
	postbackRouter.SetDefault(kitePostback)

	// Reconciliation poller
	poller := recon.NewPoller(
		store,
		&serverMasterReader{pool: accountPool},
		&serverFollowerReader{pool: accountPool},
		masterFillConsumer,
		nil,
		recon.DefaultConfig(),
		logger.With("component", "reconciler"),
	)
	poller.FollowerConsumer = followerStatusConsumer
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("reconciliation poller panicked", "panic", r)
			}
		}()
		logger.Info("reconciliation poller started")
		if err := poller.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("reconciliation poller stopped unexpectedly", "error", err)
		}
	}()

	// Dynamic multi-master WebSocket ticker manager (generic, multi-broker)
	tickerManager := ticker.NewManager(store, poller, logger.With("component", "ticker_manager"))
	kiteTickerFactory := kite.NewTickerFactory(masterFillQueue, logger.With("component", "kite_ticker"))
	dhanTickerFactory := dhanws.NewTickerFactory(masterFillQueue, instrumentCache, logger.With("component", "dhan_ticker"))
	testbrokerTickerFactory := testbroker.NewTickerFactory(masterFillQueue, logger.With("component", "testbroker_ticker"))
	tickerManager.RegisterFactory("kite", kiteTickerFactory)
	tickerManager.RegisterFactory("zerodha", kiteTickerFactory)
	tickerManager.RegisterFactory("dhan", dhanTickerFactory)
	tickerManager.RegisterFactory("testbroker", testbrokerTickerFactory)
	defer tickerManager.Shutdown()

	if err := tickerManager.SyncActiveMasters(ctx); err != nil {
		logger.Error("sync active masters tickers failed", "error", err)
	}

	// Auto-restart master ticker or refresh follower worker on headless token refresh
	rawKiteSyncer.OnTokenRefreshed = func(refreshCtx context.Context, accountID uuid.UUID) error {
		evicter.Evict(accountID)
		return handleTokenRefreshed(refreshCtx, accountID, store, tickerManager, followerRegistrar, logger)
	}

	// Scheduled Dhan token renewal (12:15 AM & 06:00 AM IST)
	dhanRenewer := dhan.NewRenewer(store, logger.With("component", "dhan_renewer"))
	dhanRenewer.OnTokenRefreshed = func(refreshCtx context.Context, accountID uuid.UUID) error {
		evicter.Evict(accountID)
		return handleTokenRefreshed(refreshCtx, accountID, store, tickerManager, followerRegistrar, logger)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("dhan renewer panicked", "panic", r)
			}
		}()
		logger.Info("dhan token renewer started", "schedule", "12:15 AM & 06:00 AM IST")
		if err := dhanRenewer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("dhan token renewer stopped unexpectedly", "error", err)
		}
	}()

	// Periodic session expiration cleaner
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("session pruner panicked", "panic", r)
			}
		}()
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
		defer func() {
			if r := recover(); r != nil {
				logger.Error("periodic portfolio syncer panicked", "panic", r)
			}
		}()
		ticker := time.NewTicker(10 * time.Second)
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
					if a.Active {
						_ = syncer.SyncAccountPortfolio(ctx, a.ID)
					}
				}
			}
		}
	}()

	dbStat := func() domain.DBPoolStatus {
		st := pool.Stat()
		return domain.DBPoolStatus{
			AcquiredConns: st.AcquiredConns(),
			IdleConns:     st.IdleConns(),
			TotalConns:    st.TotalConns(),
			MaxConns:      st.MaxConns(),
		}
	}
	sysSampler := telemetry.NewSystemSampler(3 * time.Second)
	defer sysSampler.Stop()

	telemetryCollector := telemetry.NewCollector(
		eng,
		tickerManager,
		masterFillQueue,
		orderUpdateQueue,
		workerPool,
		dbStat,
		sysSampler,
	)

	var routerOpts []httpapi.Option
	routerOpts = append(routerOpts,
		httpapi.WithVersion(Version),
		httpapi.WithPostbackHandler(postbackRouter),
		httpapi.WithLogger(logger.With("component", "httpapi")),
		httpapi.WithPortfolioSyncer(syncer),
		httpapi.WithTickerManager(tickerManager),
		httpapi.WithSquareOffService(squareOffSvc),
		httpapi.WithRebalanceService(rebalanceSvc),
		httpapi.WithFollowerRegistrar(followerRegistrar),
		httpapi.WithBrokerEvicter(evicter),
		httpapi.WithTelemetry(telemetryCollector),
		httpapi.WithExecutionFeed(execFeed),
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

type brokerResolverStore interface {
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
	ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error)
}

type portfolioSyncer interface {
	SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error
}

type accountBrokerStore interface {
	AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error)
}

type multiBrokerSyncer struct {
	store       accountBrokerStore
	kiteSyncer  portfolioSyncer
	dhanSyncer  portfolioSyncer
	syncAccount func(ctx context.Context, brokerName string, id uuid.UUID) error
}

func (m *multiBrokerSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	authInfo, err := m.store.AccountAuthInfo(ctx, accountID)
	if err != nil {
		return err
	}
	brokerName := strings.ToLower(authInfo.Broker)
	if m.syncAccount != nil {
		return m.syncAccount(ctx, brokerName, accountID)
	}
	var syncErr error
	switch brokerName {
	case "dhan":
		if m.dhanSyncer != nil {
			syncErr = m.dhanSyncer.SyncAccountPortfolio(ctx, accountID)
		}
	default:
		if m.kiteSyncer != nil {
			syncErr = m.kiteSyncer.SyncAccountPortfolio(ctx, accountID)
		}
	}
	if syncErr != nil {
		slog.Default().Error("portfolio sync failed", "account_id", accountID, "broker", brokerName, "error", syncErr)
	}
	return syncErr
}

type serverBrokerResolver struct {
	store    brokerResolverStore
	registry *broker.Registry
	syncer   portfolioSyncer
}

func (r *serverBrokerResolver) ResolveBroker(ctx context.Context, accountID uuid.UUID) (broker.Broker, error) {
	authInfo, err := r.store.AccountAuthInfo(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("load account auth info: %w", err)
	}

	if authInfo.ApiKey == "" && authInfo.AccessToken == "" {
		return &fake.Broker{}, nil
	}

	if (authInfo.AccessToken == "" || authInfo.AuthStatus == "error" || (authInfo.TokenExpiresAt != nil && !time.Now().Before(*authInfo.TokenExpiresAt))) &&
		authInfo.EncryptedPassword != "" && authInfo.EncryptedTotpSecret != "" && r.syncer != nil {
		if err := r.syncer.SyncAccountPortfolio(ctx, accountID); err == nil {
			if refreshed, err := r.store.AccountAuthInfo(ctx, accountID); err == nil {
				authInfo = refreshed
			}
		}
	}

	acc := broker.BrokerAccount{
		ID:              accountID.String(),
		Role:            authInfo.Role,
		Broker:          authInfo.Broker,
		BrokerAccountID: authInfo.BrokerAccountID,
		ApiKey:          authInfo.ApiKey,
		ApiSecret:       authInfo.ApiSecret,
		AccessToken:     authInfo.AccessToken,
		IPAddress:       authInfo.IPAddress,
	}

	if authInfo.Role == "follower" {
		if authInfo.IPAddress == "" {
			return nil, fmt.Errorf("follower account %s: %w", accountID, domain.ErrIPRequired)
		}
		pIP, err := r.store.ProxyIPByAddress(ctx, authInfo.IPAddress)
		if err != nil {
			return nil, fmt.Errorf("resolve proxy IP %s for follower %s: %w", authInfo.IPAddress, accountID, err)
		}
		acc.ProxyHost = pIP.Host
		acc.ProxyPort = pIP.Port
		acc.ProxyUsername = pIP.Username
		acc.ProxyPassword = pIP.Password
	} else if authInfo.Role == "master" {
		if authInfo.IPAddress != "" {
			pIP, err := r.store.ProxyIPByAddress(ctx, authInfo.IPAddress)
			if err != nil {
				return nil, fmt.Errorf("resolve proxy IP %s for master %s: %w", authInfo.IPAddress, accountID, err)
			}
			acc.ProxyHost = pIP.Host
			acc.ProxyPort = pIP.Port
			acc.ProxyUsername = pIP.Username
			acc.ProxyPassword = pIP.Password
		}
	}

	return r.registry.Create(ctx, acc)
}

// registerFollowers registers every existing follower account with the
// worker pool so Rebalance can dispatch to it.
func registerFollowers(ctx context.Context, store *postgres.Store, pool *worker.Pool, resolver followerBrokerResolver, logger *slog.Logger) (int, error) {
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
			b, err := resolver.ResolveBroker(ctx, f.AccountID)
			if err != nil {
				logger.Error("failed to resolve broker for follower, skipping registration", "follower_id", f.AccountID, "error", err)
				continue
			}
			pool.Register(ctx, f.AccountID, b, store, 16)
			count++
		}
	}
	return count, nil
}

type followerBrokerResolver interface {
	ResolveBroker(ctx context.Context, accountID uuid.UUID) (broker.Broker, error)
}

type workerFollowerRegistrar struct {
	pool     *worker.Pool
	resolver followerBrokerResolver
	store    worker.Store
	logger   *slog.Logger
}

func (w *workerFollowerRegistrar) log() *slog.Logger {
	if w.logger != nil {
		return w.logger
	}
	return slog.Default()
}

func (w *workerFollowerRegistrar) RegisterFollower(ctx context.Context, followerID uuid.UUID) error {
	b, err := w.resolver.ResolveBroker(ctx, followerID)
	if err != nil {
		w.log().Error("dynamic worker registration failed: cannot resolve broker", "follower_id", followerID, "error", err)
		return err
	}
	w.pool.Register(context.Background(), followerID, b, w.store, 16)
	w.log().Info("follower registered with worker pool", "follower_id", followerID)
	return nil
}

func (w *workerFollowerRegistrar) UnregisterFollower(followerID uuid.UUID) {
	w.pool.Unregister(followerID)
	w.log().Info("follower unregistered from worker pool", "follower_id", followerID)
}

type accountRoleStore interface {
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
}

type masterTickerRestarter interface {
	RestartMaster(ctx context.Context, masterID uuid.UUID) error
}

type followerRegistrar interface {
	RegisterFollower(ctx context.Context, followerID uuid.UUID) error
}

func handleTokenRefreshed(
	ctx context.Context,
	accountID uuid.UUID,
	store accountRoleStore,
	tickerMgr masterTickerRestarter,
	registrar followerRegistrar,
	logger *slog.Logger,
) error {
	if logger == nil {
		logger = slog.Default()
	}
	role, err := store.AccountRole(ctx, accountID)
	if err != nil {
		logger.Error("failed to get account role on token refresh", "account_id", accountID, "error", err)
		return err
	}
	switch role {
	case "master":
		if tickerMgr != nil {
			return tickerMgr.RestartMaster(ctx, accountID)
		}
	case "follower":
		if registrar != nil {
			if err := registrar.RegisterFollower(ctx, accountID); err != nil {
				logger.Error("failed to register follower worker on token refresh", "account_id", accountID, "error", err)
				return err
			}
		}
	}
	return nil
}

func logProcessLimits(logger *slog.Logger) {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err == nil {
		logger.Info("process file descriptor limits",
			"cur_nofile", rlimit.Cur,
			"max_nofile", rlimit.Max,
		)
	}
}

type serverMasterReader struct {
	pool *broker.AccountPool
}

func (r *serverMasterReader) GetMasterOrders(ctx context.Context, masterID uuid.UUID) ([]broker.Order, error) {
	b, err := r.pool.ResolveBroker(ctx, masterID)
	if err != nil {
		return nil, err
	}
	return b.GetOrders(ctx)
}

type serverFollowerReader struct {
	pool *broker.AccountPool
}

func (r *serverFollowerReader) GetFollowerOrderHistory(ctx context.Context, followerID uuid.UUID, brokerOrderID string) ([]broker.Order, error) {
	b, err := r.pool.ResolveBroker(ctx, followerID)
	if err != nil {
		return nil, err
	}
	return b.GetOrderHistory(ctx, brokerOrderID)
}

func (r *serverFollowerReader) GetFollowerOrders(ctx context.Context, followerID uuid.UUID) ([]broker.Order, error) {
	b, err := r.pool.ResolveBroker(ctx, followerID)
	if err != nil {
		return nil, err
	}
	return b.GetOrders(ctx)
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
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
				return slog.String(slog.TimeKey, a.Value.Time().In(domain.IST).Format("2006-01-02T15:04:05.000+05:30"))
			}
			return a
		},
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
