package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"envoytrade/internal/broker"
	"envoytrade/internal/dhan"
	"envoytrade/internal/domain"
	"envoytrade/internal/instrument"
	"envoytrade/internal/kite"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/kotak"
	"envoytrade/internal/store/postgres"
	"envoytrade/internal/testbroker"
	"envoytrade/internal/worker"
)

func TestMaskDatabaseURL(t *testing.T) {
	raw := "postgres://user:secretpassword@localhost:5432/mydb?sslmode=disable"
	masked := maskDatabaseURL(raw)
	if strings.Contains(masked, "secretpassword") {
		t.Fatalf("expected password to be masked, got: %s", masked)
	}
	if !strings.Contains(masked, "user") || !strings.Contains(masked, "localhost:5432") || !strings.Contains(masked, "mydb") {
		t.Fatalf("unexpected masked url: %s", masked)
	}
}

func TestSetupLogger_FileCreationAndFallback(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "logs", "test.log")

	t.Setenv("LOG_FILE", logFile)
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")

	logger, cleanup, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger failed: %v", err)
	}

	logger.Info("test message", "key", "val")
	cleanup()

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(data), "test message") {
		t.Errorf("log file missing message: %s", string(data))
	}
	if !strings.Contains(string(data), `"key":"val"`) {
		t.Errorf("log file missing attributes: %s", string(data))
	}
}

func TestSetupLogger_FallbackWhenDirUnwritable(t *testing.T) {
	// A path in an unwritable directory falls back to stdout
	t.Setenv("LOG_FILE", "/unwritable_root_dir_test_1234/test.log")
	t.Setenv("LOG_FORMAT", "text")

	logger, cleanup, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger unexpected error: %v", err)
	}
	defer cleanup()

	if logger == nil {
		t.Fatal("expected fallback logger, got nil")
	}
}

func TestRun_RequiresEnvVars(t *testing.T) {
	logger := slog.Default()

	// Missing DATABASE_URL
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ENCRYPTION_KEY", "any-key")
	err := run(logger)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("expected DATABASE_URL is required error, got: %v", err)
	}

	// Missing ENCRYPTION_KEY
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/db")
	t.Setenv("ENCRYPTION_KEY", "")
	err = run(logger)
	if err == nil || !strings.Contains(err.Error(), "ENCRYPTION_KEY is required") {
		t.Fatalf("expected ENCRYPTION_KEY is required error, got: %v", err)
	}
}

type mockResolverStore struct {
	accounts map[uuid.UUID]domain.AccountAuthInfo
	proxies  map[string]domain.ProxyIP
}

func (m *mockResolverStore) AccountAuthInfo(_ context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	acc, ok := m.accounts[id]
	if !ok {
		return domain.AccountAuthInfo{}, domain.ErrNotFound
	}
	return acc, nil
}

func (m *mockResolverStore) ProxyIPByAddress(_ context.Context, ip string) (domain.ProxyIP, error) {
	p, ok := m.proxies[ip]
	if !ok {
		return domain.ProxyIP{}, fmt.Errorf("proxy ip %s not found", ip)
	}
	return p, nil
}

func setupTestResolver(store brokerResolverStore) *serverBrokerResolver {
	reg := broker.NewRegistry()
	kiteFactory := broker.FactoryFunc(func(_ context.Context, acc broker.BrokerAccount) (broker.Broker, error) {
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
	reg.Register("kite", kiteFactory)
	reg.Register("zerodha", kiteFactory)
	reg.Register("testbroker", testbroker.NewBrokerFactory())

	dhanResolver := instrument.NewCache(1000, nil, nil)
	dhanFactory := broker.FactoryFunc(func(_ context.Context, acc broker.BrokerAccount) (broker.Broker, error) {
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
		return dhan.NewLiveBroker(clientID, acc.AccessToken, proxyCfg, dhanResolver)
	})
	reg.Register("dhan", dhanFactory)

	kotakResolver := instrument.NewCache(1000, nil, nil)
	kotakFactory := broker.FactoryFunc(func(_ context.Context, acc broker.BrokerAccount) (broker.Broker, error) {
		var proxyURL string
		if acc.ProxyHost != "" {
			u := url.URL{
				Scheme: "https",
				Host:   fmt.Sprintf("%s:%d", acc.ProxyHost, acc.ProxyPort),
			}
			if acc.ProxyUsername != "" {
				u.User = url.UserPassword(acc.ProxyUsername, acc.ProxyPassword)
			}
			proxyURL = u.String()
		}
		consumerKey := strings.TrimSpace(acc.ApiSecret)
		var session *kotak.TradeSessionJSON
		if acc.AccessToken != "" {
			session, _ = kotak.DeserializeTradeSession(acc.AccessToken)
		}
		return kotak.NewLiveBroker(consumerKey, session, proxyURL, kotakResolver)
	})
	reg.Register("kotak", kotakFactory)
	reg.Register("kotakneo", kotakFactory)

	return &serverBrokerResolver{
		store:    store,
		registry: reg,
	}
}

func TestResolveBroker_FollowerWithoutIP_FailsWithErrIPRequired(t *testing.T) {
	ctx := context.Background()
	fid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			fid: {
				ID:          fid,
				Role:        "follower",
				Broker:      "testbroker",
				ApiKey:      "key",
				AccessToken: "token",
				IPAddress:   "", // No IP set
			},
		},
	}
	resolver := setupTestResolver(store)

	_, err := resolver.ResolveBroker(ctx, fid)
	if err == nil {
		t.Fatal("expected error for follower without IP, got nil")
	}
	if !errors.Is(err, domain.ErrIPRequired) {
		t.Fatalf("expected ErrIPRequired, got: %v", err)
	}
}

func TestResolveBroker_FollowerMissingProxyConfig_Fails(t *testing.T) {
	ctx := context.Background()
	fid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			fid: {
				ID:          fid,
				Role:        "follower",
				Broker:      "testbroker",
				ApiKey:      "key",
				AccessToken: "token",
				IPAddress:   "192.168.1.50",
			},
		},
		proxies: map[string]domain.ProxyIP{}, // Missing from proxy registry
	}
	resolver := setupTestResolver(store)

	_, err := resolver.ResolveBroker(ctx, fid)
	if err == nil {
		t.Fatal("expected error resolving proxy IP, got nil")
	}
	if !strings.Contains(err.Error(), "resolve proxy IP") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestResolveBroker_FollowerWithValidProxy_ReturnsProxiedBroker(t *testing.T) {
	ctx := context.Background()
	fid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			fid: {
				ID:          fid,
				Role:        "follower",
				Broker:      "testbroker",
				ApiKey:      "key",
				AccessToken: "token",
				IPAddress:   "192.168.1.50",
			},
		},
		proxies: map[string]domain.ProxyIP{
			"192.168.1.50": {
				IPAddress: "192.168.1.50",
				Host:      "proxy.internal",
				Port:      8080,
				Username:  "u",
				Password:  "p",
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, fid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tb, ok := b.(*testbroker.Broker)
	if !ok {
		t.Fatalf("expected *testbroker.Broker, got %T", b)
	}
	if !tb.IsProxied() {
		t.Fatal("expected follower broker to be marked proxied")
	}
}

func TestResolveBroker_MasterWithoutIP_ReturnsUnproxiedBroker(t *testing.T) {
	ctx := context.Background()
	mid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			mid: {
				ID:          mid,
				Role:        "master",
				Broker:      "testbroker",
				ApiKey:      "key",
				AccessToken: "token",
				IPAddress:   "", // Master without IP is allowed
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, mid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tb, ok := b.(*testbroker.Broker)
	if !ok {
		t.Fatalf("expected *testbroker.Broker, got %T", b)
	}
	if tb.IsProxied() {
		t.Fatal("expected master broker to be marked unproxied")
	}

	// Execution calls must fail fast with ErrUnproxiedNotAllowed
	_, err = b.PlaceOrder(ctx, "regular", broker.OrderParams{})
	if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
		t.Fatalf("expected ErrUnproxiedNotAllowed on PlaceOrder, got %v", err)
	}
	_, err = b.CancelOrder(ctx, "regular", "ord-1")
	if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
		t.Fatalf("expected ErrUnproxiedNotAllowed on CancelOrder, got %v", err)
	}
}

func TestResolveBroker_MasterWithIP_ReturnsProxiedBroker(t *testing.T) {
	ctx := context.Background()
	mid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			mid: {
				ID:          mid,
				Role:        "master",
				Broker:      "testbroker",
				ApiKey:      "key",
				AccessToken: "token",
				IPAddress:   "192.168.1.10",
			},
		},
		proxies: map[string]domain.ProxyIP{
			"192.168.1.10": {
				IPAddress: "192.168.1.10",
				Host:      "proxy.internal",
				Port:      8080,
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, mid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tb, ok := b.(*testbroker.Broker)
	if !ok {
		t.Fatalf("expected *testbroker.Broker, got %T", b)
	}
	if !tb.IsProxied() {
		t.Fatal("expected master with IP to be marked proxied")
	}
}

func TestResolveBroker_Kite_UnproxiedMaster_BlocksPlaceOrder(t *testing.T) {
	ctx := context.Background()
	mid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			mid: {
				ID:          mid,
				Role:        "master",
				Broker:      "kite",
				ApiKey:      "test-api-key",
				AccessToken: "test-token",
				IPAddress:   "",
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, mid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	kb, ok := b.(*kite.Broker)
	if !ok {
		t.Fatalf("expected *kite.Broker, got %T", b)
	}
	if kb.IsProxied() {
		t.Fatal("expected master without IP to be marked unproxied")
	}

	_, err = b.PlaceOrder(ctx, "regular", broker.OrderParams{})
	if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
		t.Fatalf("expected ErrUnproxiedNotAllowed on PlaceOrder, got %v", err)
	}
	_, err = b.CancelOrder(ctx, "regular", "ord-1")
	if !errors.Is(err, domain.ErrUnproxiedNotAllowed) {
		t.Fatalf("expected ErrUnproxiedNotAllowed on CancelOrder, got %v", err)
	}
}

func TestResolveBroker_Kite_ProxiedFollower_IsProxied(t *testing.T) {
	ctx := context.Background()
	fid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			fid: {
				ID:          fid,
				Role:        "follower",
				Broker:      "kite",
				ApiKey:      "test-api-key",
				AccessToken: "test-token",
				IPAddress:   "192.168.1.100",
			},
		},
		proxies: map[string]domain.ProxyIP{
			"192.168.1.100": {
				IPAddress: "192.168.1.100",
				Host:      "proxy.internal",
				Port:      8080,
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, fid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	kb, ok := b.(*kite.Broker)
	if !ok {
		t.Fatalf("expected *kite.Broker, got %T", b)
	}
	if !kb.IsProxied() {
		t.Fatal("expected follower with valid proxy to be marked proxied")
	}
}

type mockRoleStore struct {
	roles map[uuid.UUID]string
	err   error
}

func (m *mockRoleStore) AccountRole(_ context.Context, id uuid.UUID) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	r, ok := m.roles[id]
	if !ok {
		return "", domain.ErrNotFound
	}
	return r, nil
}

type mockTickerRestarter struct {
	restarted []uuid.UUID
	err       error
}

func (m *mockTickerRestarter) RestartMaster(_ context.Context, masterID uuid.UUID) error {
	m.restarted = append(m.restarted, masterID)
	return m.err
}

type mockFollowerRegistrar struct {
	registered   []uuid.UUID
	unregistered []uuid.UUID
	registerErr  error
}

func (m *mockFollowerRegistrar) RegisterFollower(_ context.Context, followerID uuid.UUID) error {
	m.registered = append(m.registered, followerID)
	return m.registerErr
}

func (m *mockFollowerRegistrar) UnregisterFollower(followerID uuid.UUID) {
	m.unregistered = append(m.unregistered, followerID)
}

func TestHandleTokenRefreshed_Follower_RegistersWorker(t *testing.T) {
	fid := uuid.New()
	store := &mockRoleStore{roles: map[uuid.UUID]string{fid: "follower"}}
	tickerMgr := &mockTickerRestarter{}
	registrar := &mockFollowerRegistrar{}

	err := handleTokenRefreshed(context.Background(), fid, store, tickerMgr, registrar, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(registrar.registered) != 1 || registrar.registered[0] != fid {
		t.Errorf("registered = %v, want [%v]", registrar.registered, fid)
	}
	if len(tickerMgr.restarted) != 0 {
		t.Errorf("ticker restarted unexpectedly: %v", tickerMgr.restarted)
	}
}

func TestHandleTokenRefreshed_Master_RestartsTicker(t *testing.T) {
	mid := uuid.New()
	store := &mockRoleStore{roles: map[uuid.UUID]string{mid: "master"}}
	tickerMgr := &mockTickerRestarter{}
	registrar := &mockFollowerRegistrar{}

	err := handleTokenRefreshed(context.Background(), mid, store, tickerMgr, registrar, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tickerMgr.restarted) != 1 || tickerMgr.restarted[0] != mid {
		t.Errorf("restarted = %v, want [%v]", tickerMgr.restarted, mid)
	}
	if len(registrar.registered) != 0 {
		t.Errorf("registrar called unexpectedly for master: %v", registrar.registered)
	}
}

func TestHandleTokenRefreshed_StoreError_ReturnsError(t *testing.T) {
	id := uuid.New()
	store := &mockRoleStore{err: errors.New("db disconnect")}
	tickerMgr := &mockTickerRestarter{}
	registrar := &mockFollowerRegistrar{}

	err := handleTokenRefreshed(context.Background(), id, store, tickerMgr, registrar, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(registrar.registered) != 0 || len(tickerMgr.restarted) != 0 {
		t.Fatal("neither registrar nor tickerMgr should be called on store error")
	}
}

func TestHandleTokenRefreshed_Follower_RegistrarError_ReturnsError(t *testing.T) {
	fid := uuid.New()
	store := &mockRoleStore{roles: map[uuid.UUID]string{fid: "follower"}}
	tickerMgr := &mockTickerRestarter{}
	regErr := errors.New("proxy failure")
	registrar := &mockFollowerRegistrar{registerErr: regErr}

	err := handleTokenRefreshed(context.Background(), fid, store, tickerMgr, registrar, nil)
	if !errors.Is(err, regErr) {
		t.Fatalf("expected %v, got %v", regErr, err)
	}
}

func TestHandleTokenRefreshed_Master_TickerError_ReturnsError(t *testing.T) {
	mid := uuid.New()
	store := &mockRoleStore{roles: map[uuid.UUID]string{mid: "master"}}
	tickErr := errors.New("ws dial timeout")
	tickerMgr := &mockTickerRestarter{err: tickErr}
	registrar := &mockFollowerRegistrar{}

	err := handleTokenRefreshed(context.Background(), mid, store, tickerMgr, registrar, nil)
	if !errors.Is(err, tickErr) {
		t.Fatalf("expected %v, got %v", tickErr, err)
	}
}

type mockBrokerResolver struct {
	broker broker.Broker
	err    error
}

func (m *mockBrokerResolver) ResolveBroker(_ context.Context, _ uuid.UUID) (broker.Broker, error) {
	return m.broker, m.err
}

type mockWorkerStore struct{}

func (m *mockWorkerStore) UpdateFollowerOrderPlaced(_ context.Context, _ int64, _ string, _ int) error {
	return nil
}
func (m *mockWorkerStore) UpdateFollowerOrderFailed(_ context.Context, _ int64, _, _ string) error {
	return nil
}
func (m *mockWorkerStore) AppendOrderEvent(_ context.Context, _ domain.OrderEvent) error {
	return nil
}

func TestWorkerFollowerRegistrar_RegisterAndUnregister(t *testing.T) {
	pool := worker.NewPool()
	defer pool.Shutdown()

	fid := uuid.New()
	mockB := &fake.Broker{}
	resolver := &mockBrokerResolver{broker: mockB}
	store := &mockWorkerStore{}

	reg := &workerFollowerRegistrar{
		pool:     pool,
		resolver: resolver,
		store:    store,
	}

	job := domain.Job{FollowerOrderID: 1, FollowerID: fid, Quantity: 10}

	// Before registration: Dispatch returns false
	if pool.Dispatch(fid, job) {
		t.Fatal("Dispatch should return false before registration")
	}

	// Register follower
	if err := reg.RegisterFollower(context.Background(), fid); err != nil {
		t.Fatalf("RegisterFollower failed: %v", err)
	}

	// After registration: Dispatch returns true
	if !pool.Dispatch(fid, job) {
		t.Fatal("Dispatch should return true after registration")
	}

	// Unregister follower
	reg.UnregisterFollower(fid)

	// After unregister: Dispatch returns false
	if pool.Dispatch(fid, job) {
		t.Fatal("Dispatch should return false after unregister")
	}
}

func TestWorkerFollowerRegistrar_ResolveBrokerError_ReturnsError(t *testing.T) {
	pool := worker.NewPool()
	defer pool.Shutdown()

	fid := uuid.New()
	resErr := errors.New("cannot decrypt totp")
	resolver := &mockBrokerResolver{err: resErr}
	reg := &workerFollowerRegistrar{
		pool:     pool,
		resolver: resolver,
		store:    &mockWorkerStore{},
	}

	err := reg.RegisterFollower(context.Background(), fid)
	if !errors.Is(err, resErr) {
		t.Fatalf("expected error %v, got %v", resErr, err)
	}
}

func TestResolveBroker_DhanMaster_ResolvesDhanBroker(t *testing.T) {
	ctx := context.Background()
	mid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			mid: {
				ID:              mid,
				Role:            "master",
				Broker:          "dhan",
				BrokerAccountID: "1000000001",
				ApiKey:          "1000000001",
				AccessToken:     "valid-dhan-token",
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, mid)
	if err != nil {
		t.Fatalf("unexpected error resolving Dhan master broker: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker for Dhan master")
	}
}

func TestResolveBroker_DhanFollower_RequiresProxyAndResolvesProxied(t *testing.T) {
	ctx := context.Background()
	fid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			fid: {
				ID:              fid,
				Role:            "follower",
				Broker:          "dhan",
				BrokerAccountID: "1000000002",
				ApiKey:          "1000000002",
				AccessToken:     "valid-dhan-token",
				IPAddress:       "", // Missing IP
			},
		},
		proxies: map[string]domain.ProxyIP{
			"198.51.100.25": {
				IPAddress:          "198.51.100.25",
				Host:               "proxy.dhan.corp",
				Port:               8080,
				Username:           "dhan_user",
				Password:           "dhan_pass",
				WhitelistedBrokers: "Dhan",
			},
		},
	}
	resolver := setupTestResolver(store)

	// Negative: missing IP should fail with ErrIPRequired
	_, err := resolver.ResolveBroker(ctx, fid)
	if err == nil {
		t.Fatal("expected ErrIPRequired for Dhan follower without IP, got nil")
	}
	if !errors.Is(err, domain.ErrIPRequired) {
		t.Fatalf("expected ErrIPRequired, got: %v", err)
	}

	// Positive: with assigned proxy IP
	acc := store.accounts[fid]
	acc.IPAddress = "198.51.100.25"
	store.accounts[fid] = acc

	b, err := resolver.ResolveBroker(ctx, fid)
	if err != nil {
		t.Fatalf("unexpected error resolving Dhan follower with proxy: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker for Dhan follower with proxy")
	}
}

func TestResolveBroker_KotakMaster_ResolvesKotakBroker(t *testing.T) {
	ctx := context.Background()
	mid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			mid: {
				ID:          mid,
				Role:        "master",
				Broker:      "kotakneo",
				ApiKey:      "+919876543210",
				ApiSecret:   "consumer-key-123",
				AccessToken: `{"token":"trade-token","sid":"trade-sid","baseUrl":"https://api.kotak","feedUrl":"wss://feed.kotak","rtUrl":"wss://rt.kotak"}`,
			},
		},
	}
	resolver := setupTestResolver(store)

	b, err := resolver.ResolveBroker(ctx, mid)
	if err != nil {
		t.Fatalf("unexpected error resolving Kotak master broker: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker for Kotak master")
	}
}

func TestResolveBroker_KotakFollower_RequiresProxyAndResolvesProxied(t *testing.T) {
	ctx := context.Background()
	fid := uuid.New()
	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			fid: {
				ID:          fid,
				Role:        "follower",
				Broker:      "kotakneo",
				ApiKey:      "+919876543210",
				ApiSecret:   "consumer-key-123",
				AccessToken: `{"token":"trade-token","sid":"trade-sid","baseUrl":"https://api.kotak","feedUrl":"wss://feed.kotak","rtUrl":"wss://rt.kotak"}`,
				IPAddress:   "", // Missing IP
			},
		},
		proxies: map[string]domain.ProxyIP{
			"198.51.100.30": {
				IPAddress:          "198.51.100.30",
				Host:               "proxy.kotak.corp",
				Port:               8080,
				Username:           "kotak_user",
				Password:           "kotak_pass",
				WhitelistedBrokers: "KotakNeo",
			},
		},
	}
	resolver := setupTestResolver(store)

	// Negative: missing IP should fail with ErrIPRequired
	_, err := resolver.ResolveBroker(ctx, fid)
	if err == nil {
		t.Fatal("expected ErrIPRequired for Kotak follower without IP, got nil")
	}
	if !errors.Is(err, domain.ErrIPRequired) {
		t.Fatalf("expected ErrIPRequired, got: %v", err)
	}

	// Positive: with assigned proxy IP
	acc := store.accounts[fid]
	acc.IPAddress = "198.51.100.30"
	store.accounts[fid] = acc

	b, err := resolver.ResolveBroker(ctx, fid)
	if err != nil {
		t.Fatalf("unexpected error resolving Kotak follower with proxy: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil broker for Kotak follower with proxy")
	}
}

func TestMultiBrokerSyncer_RoutesByBroker(t *testing.T) {
	ctx := context.Background()
	kiteAccID := uuid.New()
	dhanAccID := uuid.New()
	kotakAccID := uuid.New()

	store := &mockResolverStore{
		accounts: map[uuid.UUID]domain.AccountAuthInfo{
			kiteAccID: {
				ID:     kiteAccID,
				Broker: "zerodha",
			},
			dhanAccID: {
				ID:     dhanAccID,
				Broker: "dhan",
			},
			kotakAccID: {
				ID:     kotakAccID,
				Broker: "kotakneo",
			},
		},
	}

	kiteSyncCalled := false
	dhanSyncCalled := false
	kotakSyncCalled := false

	multi := &multiBrokerSyncer{
		store: store,
		syncAccount: func(ctx context.Context, brokerName string, id uuid.UUID) error {
			switch brokerName {
			case "dhan":
				dhanSyncCalled = true
			case "kotak", "kotakneo":
				kotakSyncCalled = true
			default:
				kiteSyncCalled = true
			}
			return nil
		},
	}

	if err := multi.SyncAccountPortfolio(ctx, kiteAccID); err != nil {
		t.Fatalf("SyncAccountPortfolio kite failed: %v", err)
	}
	if !kiteSyncCalled {
		t.Error("expected kite sync to be called")
	}

	if err := multi.SyncAccountPortfolio(ctx, dhanAccID); err != nil {
		t.Fatalf("SyncAccountPortfolio dhan failed: %v", err)
	}
	if !dhanSyncCalled {
		t.Error("expected dhan sync to be called")
	}

	if err := multi.SyncAccountPortfolio(ctx, kotakAccID); err != nil {
		t.Fatalf("SyncAccountPortfolio kotak failed: %v", err)
	}
	if !kotakSyncCalled {
		t.Error("expected kotak sync to be called")
	}
}

type fakeInstrumentSyncer struct {
	dhanCalled    bool
	dhanErr       error
	kiteCalled    bool
	kiteExchanges []string
	kiteErr       error
	kotakCalled   bool
	kotakSegments []string
	kotakErr      error
}

func (f *fakeInstrumentSyncer) SyncDhan(_ context.Context) error {
	f.dhanCalled = true
	return f.dhanErr
}

func (f *fakeInstrumentSyncer) SyncKite(_ context.Context, exchanges []string) error {
	f.kiteCalled = true
	f.kiteExchanges = exchanges
	return f.kiteErr
}

func (f *fakeInstrumentSyncer) SyncKotak(_ context.Context, segments []string) error {
	f.kotakCalled = true
	f.kotakSegments = segments
	return f.kotakErr
}

func TestRunSyncInstruments_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := runSyncInstruments(slog.Default(), []string{})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is not set") {
		t.Fatalf("expected DATABASE_URL is not set error, got: %v", err)
	}
}

func TestRunSyncInstruments_InvalidFlag(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	err := runSyncInstruments(slog.Default(), []string{"-invalid-flag=true"})
	if err == nil {
		t.Fatal("expected error for invalid flag, got nil")
	}
}

func TestRunSyncInstruments_InvalidBrokerChoice(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	origConnect := connectPostgres
	defer func() { connectPostgres = origConnect }()
	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}

	err := runSyncInstruments(slog.Default(), []string{"-broker=unsupported_broker"})
	if err == nil || !strings.Contains(err.Error(), "invalid broker choice") {
		t.Fatalf("expected invalid broker choice error, got: %v", err)
	}
}

func TestRunSyncInstruments_DBConnectionFailure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	origConnect := connectPostgres
	defer func() { connectPostgres = origConnect }()
	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, nil, errors.New("network dial failed")
	}

	err := runSyncInstruments(slog.Default(), []string{})
	if err == nil || !strings.Contains(err.Error(), "connect db: network dial failed") {
		t.Fatalf("expected connect db error, got: %v", err)
	}
}

func TestRunSyncInstruments_Positive_AllBrokersDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	fakeSyncer := &fakeInstrumentSyncer{}

	origConnect := connectPostgres
	origFactory := newInstrumentSyncer
	defer func() {
		connectPostgres = origConnect
		newInstrumentSyncer = origFactory
	}()

	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}
	newInstrumentSyncer = func(_ instrument.SyncerStore, _ *slog.Logger) instrumentSyncRunner {
		return fakeSyncer
	}

	err := runSyncInstruments(slog.Default(), []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeSyncer.dhanCalled {
		t.Error("expected Dhan sync to be called")
	}
	if !fakeSyncer.kiteCalled {
		t.Error("expected Kite sync to be called")
	}
	if !fakeSyncer.kotakCalled {
		t.Error("expected Kotak sync to be called")
	}
	expectedExchanges := []string{"MCX", "NFO", "BFO"}
	if len(fakeSyncer.kiteExchanges) != len(expectedExchanges) {
		t.Errorf("kite exchanges = %v, want %v", fakeSyncer.kiteExchanges, expectedExchanges)
	}
}

func TestRunSyncInstruments_Positive_KotakOnly(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	fakeSyncer := &fakeInstrumentSyncer{}

	origConnect := connectPostgres
	origFactory := newInstrumentSyncer
	defer func() {
		connectPostgres = origConnect
		newInstrumentSyncer = origFactory
	}()

	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}
	newInstrumentSyncer = func(_ instrument.SyncerStore, _ *slog.Logger) instrumentSyncRunner {
		return fakeSyncer
	}

	err := runSyncInstruments(slog.Default(), []string{"-broker=kotak"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeSyncer.dhanCalled {
		t.Error("expected Dhan sync NOT to be called")
	}
	if fakeSyncer.kiteCalled {
		t.Error("expected Kite sync NOT to be called")
	}
	if !fakeSyncer.kotakCalled {
		t.Error("expected Kotak sync to be called")
	}
}

func TestRunSyncInstruments_Positive_DhanOnly(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	fakeSyncer := &fakeInstrumentSyncer{}

	origConnect := connectPostgres
	origFactory := newInstrumentSyncer
	defer func() {
		connectPostgres = origConnect
		newInstrumentSyncer = origFactory
	}()

	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}
	newInstrumentSyncer = func(_ instrument.SyncerStore, _ *slog.Logger) instrumentSyncRunner {
		return fakeSyncer
	}

	// Case-insensitive test "DHAN"
	err := runSyncInstruments(slog.Default(), []string{"-broker=DHAN"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fakeSyncer.dhanCalled {
		t.Error("expected Dhan sync to be called")
	}
	if fakeSyncer.kiteCalled {
		t.Error("expected Kite sync NOT to be called")
	}
}

func TestRunSyncInstruments_Positive_KiteOnly_CustomExchanges(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	fakeSyncer := &fakeInstrumentSyncer{}

	origConnect := connectPostgres
	origFactory := newInstrumentSyncer
	defer func() {
		connectPostgres = origConnect
		newInstrumentSyncer = origFactory
	}()

	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}
	newInstrumentSyncer = func(_ instrument.SyncerStore, _ *slog.Logger) instrumentSyncRunner {
		return fakeSyncer
	}

	// Comma/whitespace padding test
	err := runSyncInstruments(slog.Default(), []string{"-broker=kite", "-exchanges= MCX , , NFO "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fakeSyncer.dhanCalled {
		t.Error("expected Dhan sync NOT to be called")
	}
	if !fakeSyncer.kiteCalled {
		t.Error("expected Kite sync to be called")
	}
	expected := []string{"MCX", "NFO"}
	if len(fakeSyncer.kiteExchanges) != 2 || fakeSyncer.kiteExchanges[0] != "MCX" || fakeSyncer.kiteExchanges[1] != "NFO" {
		t.Errorf("kite exchanges = %v, want %v", fakeSyncer.kiteExchanges, expected)
	}
}

func TestRunSyncInstruments_UpstreamDhanFailure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	fakeSyncer := &fakeInstrumentSyncer{
		dhanErr: errors.New("503 Service Unavailable"),
	}

	origConnect := connectPostgres
	origFactory := newInstrumentSyncer
	defer func() {
		connectPostgres = origConnect
		newInstrumentSyncer = origFactory
	}()

	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}
	newInstrumentSyncer = func(_ instrument.SyncerStore, _ *slog.Logger) instrumentSyncRunner {
		return fakeSyncer
	}

	err := runSyncInstruments(slog.Default(), []string{"-broker=dhan"})
	if err == nil || !strings.Contains(err.Error(), "sync dhan instruments: 503 Service Unavailable") {
		t.Fatalf("expected dhan error, got: %v", err)
	}
}

func TestRunSyncInstruments_UpstreamKiteFailure(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
	fakeSyncer := &fakeInstrumentSyncer{
		kiteErr: errors.New("timeout reading csv"),
	}

	origConnect := connectPostgres
	origFactory := newInstrumentSyncer
	defer func() {
		connectPostgres = origConnect
		newInstrumentSyncer = origFactory
	}()

	connectPostgres = func(_ context.Context, _ string) (*postgres.Store, func(), error) {
		return nil, func() {}, nil
	}
	newInstrumentSyncer = func(_ instrument.SyncerStore, _ *slog.Logger) instrumentSyncRunner {
		return fakeSyncer
	}

	err := runSyncInstruments(slog.Default(), []string{"-broker=kite"})
	if err == nil || !strings.Contains(err.Error(), "sync kite instruments: timeout reading csv") {
		t.Fatalf("expected kite error, got: %v", err)
	}
}



