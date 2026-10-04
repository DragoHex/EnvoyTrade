//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"envoytrade/internal/domain"
)

func TestProxyIPs_UpsertAndQuery(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	p1 := domain.ProxyIP{
		IPAddress:  "192.0.2.1",
		IPType:     "ipv4",
		Host:       "proxy1.example.com",
		Port:       443,
		Username:   "user1",
		Password:   "pass1",
		ValidFrom:  time.Now().Add(-1 * time.Hour),
		ValidUntil: time.Now().Add(24 * time.Hour),
		Plan:       "MONTHLY",
	}

	if err := store.UpsertProxyIP(ctx, p1); err != nil {
		t.Fatalf("UpsertProxyIP failed: %v", err)
	}

	got, err := store.ProxyIPByAddress(ctx, "192.0.2.1")
	if err != nil {
		t.Fatalf("ProxyIPByAddress failed: %v", err)
	}
	if got.Host != "proxy1.example.com" || got.Username != "user1" || got.Password != "pass1" {
		t.Errorf("got %+v, want %+v", got, p1)
	}

	// Update password and plan
	p1.Password = "newpass"
	p1.Plan = "ANNUAL"
	if err := store.UpsertProxyIP(ctx, p1); err != nil {
		t.Fatalf("UpsertProxyIP update failed: %v", err)
	}

	gotUpdated, err := store.ProxyIPByAddress(ctx, "192.0.2.1")
	if err != nil {
		t.Fatalf("ProxyIPByAddress after update failed: %v", err)
	}
	if gotUpdated.Password != "newpass" || gotUpdated.Plan != "ANNUAL" {
		t.Errorf("got %+v after update, want newpass/ANNUAL", gotUpdated)
	}
}

func TestProxyIPs_ListAndAvailable(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	p1 := domain.ProxyIP{
		IPAddress: "192.0.2.10",
		IPType:    "ipv4",
		Host:      "proxy10.example.com",
		Port:      443,
		Username:  "u10",
		Password:  "p10",
	}
	p2 := domain.ProxyIP{
		IPAddress: "2001:db8::10",
		IPType:    "ipv6",
		Host:      "proxy10.example.com",
		Port:      443,
		Username:  "u10v6",
		Password:  "p10v6",
	}

	if err := store.UpsertProxyIP(ctx, p1); err != nil {
		t.Fatalf("UpsertProxyIP p1 failed: %v", err)
	}
	if err := store.UpsertProxyIP(ctx, p2); err != nil {
		t.Fatalf("UpsertProxyIP p2 failed: %v", err)
	}

	list, err := store.ListProxyIPs(ctx)
	if err != nil {
		t.Fatalf("ListProxyIPs failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 proxy IPs, got %d", len(list))
	}

	availIPv4, err := store.AvailableProxyIPs(ctx, "ipv4", nil)
	if err != nil {
		t.Fatalf("AvailableProxyIPs ipv4 failed: %v", err)
	}
	found := false
	for _, ip := range availIPv4 {
		if ip.IPAddress == "192.0.2.10" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 192.0.2.10 in available IPv4 proxy IPs")
	}
}
