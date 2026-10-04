//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"testbroker/sdk"
)

// Known account IDs from scripts/seed_test_data.sql
const (
	Master01ID  = "f6e70723-b904-4427-83ee-a85771dee2e4"
	Master02ID  = "c80b3596-4449-4f32-b25a-a5bfd6883c4f"
	Follow01AID = "a5183e89-6cb2-4d32-91a2-6dc525570185"
	Follow01BID = "5894c29d-7740-4494-9c98-c9d59d1364bc"
	Follow01CID = "18dfc57d-0f23-49af-8ccd-1c0edcbe4788"
	Follow01DID = "ef0a6211-e753-42b8-a6eb-9c8d15ad90db"
	Follow02AID = "0f40d9f2-34aa-42fa-8d99-90b9255fd168"
	Follow02BID = "d0527ce3-4c4b-40c4-ba0d-af5a79f19a99"
	Follow02CID = "d800e6e7-5d11-4d35-b24a-74a9b4aa4832"
	Follow02DID = "4f9b42e4-e1ea-4fab-b9f7-e6197c28ea9a"
)

type MasterFillRow struct {
	ID             int64
	MasterID       string
	BrokerOrderID  string
	Exchange       string
	TradingSymbol  string
	FilledQuantity int
	AveragePrice   float64
	Status         string
	DispatchState  string
}

type FollowerOrderRow struct {
	ID                int64
	MasterFillID      int64
	FollowerID        string
	BrokerOrderID     string
	Exchange          string
	TradingSymbol     string
	TransactionType   string
	OrderType         string
	RequestedQuantity int
	FilledQuantity    int
	AveragePrice      float64
	Status            string
	SizingReason      int
	IdempotencyTag    string
}

type OrderEventRow struct {
	ID              string
	FollowerOrderID string
	BrokerOrderID   string
	EventType       string
	Payload         string
}

// Harness manages test processes, API clients, and DB connection.
type Harness struct {
	t               *testing.T
	EnvoyURL        string
	TestBrokerURL   string
	TestBrokerWSURL string
	DatabaseURL     string
	DB              *pgxpool.Pool
	HTTPClient      *http.Client
	AdminSDK        *sdk.Client
	cleanupFuncs    []func()
}

// NewHarness initializes the test environment with hybrid process lifecycle.
func NewHarness(t *testing.T) *Harness {
	envoyURL := os.Getenv("ENVOY_URL")
	if envoyURL == "" {
		envoyURL = "http://localhost:8080"
	}
	tbURL := os.Getenv("TESTBROKER_URL")
	if tbURL == "" {
		tbURL = "http://localhost:8089"
	}
	tbWS := os.Getenv("TESTBROKER_WS_URL")
	if tbWS == "" {
		tbWS = "ws://localhost:8089"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://envoytrade:envoytrade@localhost:5434/envoytrade?sslmode=disable"
	}

	h := &Harness{
		t:               t,
		EnvoyURL:        envoyURL,
		TestBrokerURL:   tbURL,
		TestBrokerWSURL: tbWS,
		DatabaseURL:     dbURL,
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	h.HTTPClient = &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	h.ensureServicesRunning()

	// Connect to Postgres
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	h.DB = pool
	h.cleanupFuncs = append(h.cleanupFuncs, pool.Close)

	// Admin SDK client for testbroker
	h.AdminSDK = sdk.New("admin")
	h.AdminSDK.SetBaseURI(tbURL)
	h.AdminSDK.SetAccessToken("admin_token")

	// Login to EnvoyTrade
	h.loginTrader("trader@envoytrade.com", "password123")

	t.Cleanup(func() {
		for i := len(h.cleanupFuncs) - 1; i >= 0; i-- {
			h.cleanupFuncs[i]()
		}
	})

	return h
}

// MasterClient returns an sdk.Client configured for the specified master.
func (h *Harness) MasterClient(user string) *sdk.Client {
	c := sdk.New("key_" + user)
	c.SetAccessToken("token_" + user)
	c.SetBaseURI(h.TestBrokerURL)
	return c
}

// FollowerClient returns an sdk.Client configured for the specified follower.
func (h *Harness) FollowerClient(user string) *sdk.Client {
	c := sdk.New("key_" + user)
	c.SetAccessToken("token_" + user)
	c.SetBaseURI(h.TestBrokerURL)
	return c
}

// ensureServicesRunning probes services and launches them if not reachable.
func (h *Harness) ensureServicesRunning() {
	rootDir, err := findRepoRoot()
	if err != nil {
		h.t.Fatalf("find repo root: %v", err)
	}

	// 1. Check TestBroker
	if !isReachable(h.TestBrokerURL + "/orders") {
		h.t.Logf("TestBroker not running on %s, starting subprocess...", h.TestBrokerURL)
		tbCmd := exec.Command("go", "run", ".", "-config", "configs/testbroker_config.json")
		tbCmd.Dir = filepath.Join(rootDir, "testbroker")
		tbCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		tbLog, _ := os.OpenFile("/tmp/envoy-e2e-tb.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		tbCmd.Stdout = tbLog
		tbCmd.Stderr = tbLog
		if err := tbCmd.Start(); err != nil {
			h.t.Fatalf("failed to start testbroker: %v", err)
		}
		h.cleanupFuncs = append(h.cleanupFuncs, func() {
			if tbCmd.Process != nil {
				_ = syscall.Kill(-tbCmd.Process.Pid, syscall.SIGKILL)
			}
			if tbLog != nil {
				_ = tbLog.Close()
			}
		})
		waitForURL(h.t, h.TestBrokerURL+"/orders", 15*time.Second)
		h.t.Log("TestBroker subprocess is ready")
	}

	// 2. Check EnvoyTrade Server
	if !isReachable(h.EnvoyURL + "/healthz") {
		h.t.Logf("EnvoyTrade not running on %s, starting subprocess...", h.EnvoyURL)
		serverCmd := exec.Command("go", "run", "./cmd/server")
		serverCmd.Dir = rootDir
		serverCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		serverCmd.Env = append(os.Environ(),
			"PORT=8080",
			"DATABASE_URL="+h.DatabaseURL,
			"TESTBROKER_URL="+h.TestBrokerURL,
			"TESTBROKER_WS_URL="+h.TestBrokerWSURL,
			"LOG_TO_STDOUT=true",
			"LOG_LEVEL=info",
		)
		serverLog, _ := os.OpenFile("/tmp/envoy-e2e-server.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		serverCmd.Stdout = serverLog
		serverCmd.Stderr = serverLog
		if err := serverCmd.Start(); err != nil {
			h.t.Fatalf("failed to start envoytrade server: %v", err)
		}
		h.cleanupFuncs = append(h.cleanupFuncs, func() {
			if serverCmd.Process != nil {
				_ = syscall.Kill(-serverCmd.Process.Pid, syscall.SIGKILL)
			}
			if serverLog != nil {
				_ = serverLog.Close()
			}
		})
		waitForURL(h.t, h.EnvoyURL+"/healthz", 20*time.Second)
		h.t.Log("EnvoyTrade server subprocess is ready")
	}
}

func (h *Harness) loginTrader(email, password string) {
	loginBody, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	resp, err := h.HTTPClient.Post(h.EnvoyURL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		h.t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		h.t.Fatalf("login failed status %d: %s", resp.StatusCode, string(b))
	}
}

// EnvoyAPI sends an authenticated HTTP request to EnvoyTrade.
func (h *Harness) EnvoyAPI(method, path string, body any) (*http.Response, []byte, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, h.EnvoyURL+path, bodyReader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	return resp, respBody, err
}

// GetMasterFills returns master_fills for a given master ID.
func (h *Harness) GetMasterFills(ctx context.Context, masterID string) ([]MasterFillRow, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT id, master_id, broker_order_id, exchange, tradingsymbol, filled_quantity, COALESCE(average_price, 0), status, dispatch_state
		FROM master_fills
		WHERE master_id = $1
		ORDER BY id DESC
	`, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fills []MasterFillRow
	for rows.Next() {
		var f MasterFillRow
		if err := rows.Scan(&f.ID, &f.MasterID, &f.BrokerOrderID, &f.Exchange, &f.TradingSymbol, &f.FilledQuantity, &f.AveragePrice, &f.Status, &f.DispatchState); err != nil {
			return nil, err
		}
		fills = append(fills, f)
	}
	return fills, nil
}

// GetFollowerOrders returns follower_orders for a given follower ID.
func (h *Harness) GetFollowerOrders(ctx context.Context, followerID string) ([]FollowerOrderRow, error) {
	rows, err := h.DB.Query(ctx, `
		SELECT fo.id, fo.master_fill_id, fo.follower_id, COALESCE(fo.broker_order_id, ''), mf.exchange, mf.tradingsymbol, mf.transaction_type, mf.order_type, fo.intended_qty, fo.filled_qty, COALESCE(fo.average_price, 0), COALESCE(fo.terminal_status, 'PENDING'), fo.sizing_reason, fo.idempotency_tag
		FROM follower_orders fo
		JOIN master_fills mf ON mf.id = fo.master_fill_id
		WHERE fo.follower_id = $1
		ORDER BY fo.id DESC
	`, followerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []FollowerOrderRow
	for rows.Next() {
		var o FollowerOrderRow
		if err := rows.Scan(&o.ID, &o.MasterFillID, &o.FollowerID, &o.BrokerOrderID, &o.Exchange, &o.TradingSymbol, &o.TransactionType, &o.OrderType, &o.RequestedQuantity, &o.FilledQuantity, &o.AveragePrice, &o.Status, &o.SizingReason, &o.IdempotencyTag); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

// GetFollowerOrderByTag returns the follower_order row matching an idempotency tag.
func (h *Harness) GetFollowerOrderByTag(ctx context.Context, tag string) (*FollowerOrderRow, error) {
	var o FollowerOrderRow
	err := h.DB.QueryRow(ctx, `
		SELECT fo.id, fo.master_fill_id, fo.follower_id, COALESCE(fo.broker_order_id, ''), mf.exchange, mf.tradingsymbol, mf.transaction_type, mf.order_type, fo.intended_qty, fo.filled_qty, COALESCE(fo.average_price, 0), COALESCE(fo.terminal_status, 'PENDING'), fo.sizing_reason, fo.idempotency_tag
		FROM follower_orders fo
		JOIN master_fills mf ON mf.id = fo.master_fill_id
		WHERE fo.idempotency_tag = $1
	`, tag).Scan(&o.ID, &o.MasterFillID, &o.FollowerID, &o.BrokerOrderID, &o.Exchange, &o.TradingSymbol, &o.TransactionType, &o.OrderType, &o.RequestedQuantity, &o.FilledQuantity, &o.AveragePrice, &o.Status, &o.SizingReason, &o.IdempotencyTag)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// WaitForFollowerOrder polls the database until a follower order is created or timeout expires.
func (h *Harness) WaitForFollowerOrder(ctx context.Context, followerID, brokerOrderID string, timeout time.Duration) (*FollowerOrderRow, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		orders, err := h.GetFollowerOrders(ctx, followerID)
		if err == nil {
			for _, o := range orders {
				if brokerOrderID == "" || o.BrokerOrderID == brokerOrderID {
					return &o, nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for follower order for follower %s", followerID)
}

func isReachable(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("tcp", u.Host, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForURL(t *testing.T, targetURL string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isReachable(targetURL) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", targetURL)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repo root containing go.mod")
		}
		dir = parent
	}
}
