package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"envoytrade/internal/kite"
	"envoytrade/internal/testbroker"
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


