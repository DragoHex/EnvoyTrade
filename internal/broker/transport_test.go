package broker_test

import (
	"sync"
	"testing"
	"time"

	"envoytrade/internal/broker"
)

func TestTransportPool_ReusesTransport(t *testing.T) {
	pool := broker.NewTransportPool()

	cfg := broker.ProxyConfig{
		Scheme:       "https",
		Host:         "192.168.1.100",
		Port:         8080,
		ClientID:     "user1",
		ClientSecret: "pass1",
	}

	client1, err := pool.GetClient(cfg)
	if err != nil {
		t.Fatalf("first GetClient failed: %v", err)
	}

	client2, err := pool.GetClient(cfg)
	if err != nil {
		t.Fatalf("second GetClient failed: %v", err)
	}

	if client1.Transport == nil || client2.Transport == nil {
		t.Fatalf("expected non-nil Transports")
	}

	// Pointer equality confirms TCP/TLS keep-alive connection reuse
	if client1.Transport != client2.Transport {
		t.Errorf("expected client1.Transport == client2.Transport, got different pointers")
	}
}

func TestTransportPool_InvalidProxy(t *testing.T) {
	pool := broker.NewTransportPool()

	cfg := broker.ProxyConfig{
		Host: "", // missing host
	}

	_, err := pool.GetClient(cfg)
	if err == nil {
		t.Errorf("expected error for empty proxy host, got nil")
	}
}

func TestTransportPool_ConcurrentAccess(t *testing.T) {
	pool := broker.NewTransportPool()
	cfg := broker.ProxyConfig{
		Scheme:       "https",
		Host:         "10.0.0.1",
		Port:         8888,
		ClientID:     "u",
		ClientSecret: "p",
	}

	const concurrency = 50
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			client, err := pool.GetClient(cfg)
			if err != nil {
				t.Errorf("GetClient error: %v", err)
				return
			}
			if client.Timeout != 10*time.Second {
				t.Errorf("Timeout = %v, want 10s", client.Timeout)
			}
		}()
	}

	wg.Wait()
}
