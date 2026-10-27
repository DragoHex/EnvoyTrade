package dhan_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"envoytrade/internal/dhan"

	"github.com/DragoHex/godhan"
)

func TestDefaultHTTPTransport(t *testing.T) {
	tr := dhan.DefaultHTTPTransport()
	if tr == nil {
		t.Fatal("expected non-nil http.Transport")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Errorf("ForceAttemptHTTP2 = false, want true")
	}
	if tr.ResponseHeaderTimeout != 10*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 10s", tr.ResponseHeaderTimeout)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 50 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 50", tr.MaxIdleConnsPerHost)
	}
}

func TestDefaultHTTPClient(t *testing.T) {
	client := dhan.DefaultHTTPClient()
	if client == nil {
		t.Fatal("expected non-nil http.Client")
	}
	if client.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want 15s", client.Timeout)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("expected Transport to be *http.Transport, got %T", client.Transport)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Errorf("ForceAttemptHTTP2 = false, want true")
	}
}

func TestBuildClientOptions_DirectNoProxy(t *testing.T) {
	opts := dhan.BuildClientOptions(nil)
	if len(opts) == 0 {
		t.Fatal("expected at least WithHTTPClient option")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	opts = append(opts, godhan.WithBaseURI(server.URL))
	client := godhan.NewClient("client123", "token123", opts...)
	positions, err := client.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions count = %d, want 0", len(positions))
	}
}

func TestBuildClientOptions_WithProxy(t *testing.T) {
	proxyHit := false
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer proxyServer.Close()

	cfg := &dhan.ProxyConfig{
		Scheme:       "http",
		Host:         proxyServer.Listener.Addr().String(),
		ClientID:     "proxy-user",
		ClientSecret: "proxy-pass",
	}

	opts := dhan.BuildClientOptions(cfg, godhan.WithBaseURI("http://dummy-upstream.dhan.co"))
	client := godhan.NewClient("client123", "token123", opts...)

	positions, err := client.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions via proxy: %v", err)
	}
	if !proxyHit {
		t.Errorf("expected request to route through proxy server")
	}
	if len(positions) != 0 {
		t.Errorf("positions count = %d, want 0", len(positions))
	}
}

func TestBuildClientOptions_EmptyHostInProxyConfig(t *testing.T) {
	cfg := &dhan.ProxyConfig{
		Host: "",
	}
	opts := dhan.BuildClientOptions(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	opts = append(opts, godhan.WithBaseURI(server.URL))
	client := godhan.NewClient("client123", "token123", opts...)
	positions, err := client.GetPositions(context.Background())
	if err != nil {
		t.Fatalf("GetPositions with empty host proxy config: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions count = %d, want 0", len(positions))
	}
}

func TestNewLiveBroker_ProxiedFlag(t *testing.T) {
	bDirect, err := dhan.NewLiveBroker("client1", "token1", nil, nil)
	if err != nil {
		t.Fatalf("NewLiveBroker(nil): %v", err)
	}
	if bDirect.IsProxied() {
		t.Errorf("bDirect.IsProxied() = true, want false")
	}

	cfg := &dhan.ProxyConfig{
		Host: "proxy.local",
		Port: 8080,
	}
	bProxied, err := dhan.NewLiveBroker("client2", "token2", cfg, nil)
	if err != nil {
		t.Fatalf("NewLiveBroker(cfg): %v", err)
	}
	if !bProxied.IsProxied() {
		t.Errorf("bProxied.IsProxied() = false, want true")
	}
}
