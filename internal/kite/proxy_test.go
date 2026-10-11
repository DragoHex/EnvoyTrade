package kite_test

import (
	"net/http"
	"sync"
	"testing"

	"envoytrade/internal/kite"
)

func TestTransportPool_ReusesTransport(t *testing.T) {
	pool := kite.NewTransportPool()
	cfg := kite.ProxyConfig{
		Scheme:       "https",
		Host:         "dc46-mum-01.algoip.in",
		Port:         443,
		ClientID:     "user1",
		ClientSecret: "pass1",
	}

	c1, err := pool.GetClient(cfg)
	if err != nil {
		t.Fatalf("first GetClient failed: %v", err)
	}
	c2, err := pool.GetClient(cfg)
	if err != nil {
		t.Fatalf("second GetClient failed: %v", err)
	}

	if c1.Transport == nil || c2.Transport == nil {
		t.Fatal("expected non-nil transports")
	}

	// Must share the exact same underlying *http.Transport pointer to reuse TCP/TLS sockets
	if c1.Transport != c2.Transport {
		t.Errorf("expected shared *http.Transport pointer, got %p and %p", c1.Transport, c2.Transport)
	}
}

func TestTransportPool_DifferentProxies(t *testing.T) {
	pool := kite.NewTransportPool()
	cfg1 := kite.ProxyConfig{
		Scheme:       "https",
		Host:         "dc46-mum-01.algoip.in",
		Port:         443,
		ClientID:     "user1",
		ClientSecret: "pass1",
	}
	cfg2 := kite.ProxyConfig{
		Scheme:       "https",
		Host:         "dc46-mum-02.algoip.in",
		Port:         443,
		ClientID:     "user2",
		ClientSecret: "pass2",
	}

	c1, err := pool.GetClient(cfg1)
	if err != nil {
		t.Fatalf("cfg1 failed: %v", err)
	}
	c2, err := pool.GetClient(cfg2)
	if err != nil {
		t.Fatalf("cfg2 failed: %v", err)
	}

	if c1.Transport == c2.Transport {
		t.Errorf("expected different transports for different proxies, got identical pointer %p", c1.Transport)
	}
}

func TestTransportPool_ConcurrentAccess(t *testing.T) {
	pool := kite.NewTransportPool()
	cfg := kite.ProxyConfig{
		Scheme:       "https",
		Host:         "dc46-mum-01.algoip.in",
		Port:         443,
		ClientID:     "user1",
		ClientSecret: "pass1",
	}

	var wg sync.WaitGroup
	clients := make([]*http.Client, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, err := pool.GetClient(cfg)
			if err != nil {
				t.Errorf("concurrent GetClient failed: %v", err)
				return
			}
			clients[idx] = c
		}(i)
	}
	wg.Wait()

	firstTransport := clients[0].Transport
	for i, c := range clients {
		if c.Transport != firstTransport {
			t.Errorf("client %d transport mismatch: got %p, expected %p", i, c.Transport, firstTransport)
		}
	}
}

func TestRESTClientFor_UsesDefaultPool(t *testing.T) {
	cfg := kite.ProxyConfig{
		Scheme:       "https",
		Host:         "dc46-mum-03.algoip.in",
		Port:         443,
		ClientID:     "user3",
		ClientSecret: "pass3",
	}

	c1, err := kite.RESTClientFor(cfg)
	if err != nil {
		t.Fatalf("c1 failed: %v", err)
	}
	c2, err := kite.RESTClientFor(cfg)
	if err != nil {
		t.Fatalf("c2 failed: %v", err)
	}

	if c1.Transport != c2.Transport {
		t.Errorf("RESTClientFor should reuse transport from DefaultTransportPool: %p != %p", c1.Transport, c2.Transport)
	}
}
