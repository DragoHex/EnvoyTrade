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
		IPAddress:          "192.0.2.1",
		IPType:             "ipv4",
		Host:               "proxy1.example.com",
		Port:               443,
		Username:           "user1",
		Password:           "pass1",
		WhitelistedBrokers:  "Zerodha",
		ValidFrom:          time.Now().Add(-1 * time.Hour),
		ValidUntil:         time.Now().Add(24 * time.Hour),
		Plan:               "MONTHLY",
	}

	if err := store.UpsertProxyIP(ctx, p1); err != nil {
		t.Fatalf("UpsertProxyIP failed: %v", err)
	}

	got, err := store.ProxyIPByAddress(ctx, "192.0.2.1")
	if err != nil {
		t.Fatalf("ProxyIPByAddress failed: %v", err)
	}
	if got.Host != "proxy1.example.com" || got.Username != "user1" || got.Password != "pass1" || got.WhitelistedBrokers != "Zerodha" {
		t.Errorf("got %+v, want %+v with WhitelistedBrokers='Zerodha'", got, p1)
	}

	// Update password, plan, and whitelisted brokers
	p1.Password = "newpass"
	p1.Plan = "ANNUAL"
	p1.WhitelistedBrokers = "TestBroker"
	if err := store.UpsertProxyIP(ctx, p1); err != nil {
		t.Fatalf("UpsertProxyIP update failed: %v", err)
	}

	gotUpdated, err := store.ProxyIPByAddress(ctx, "192.0.2.1")
	if err != nil {
		t.Fatalf("ProxyIPByAddress after update failed: %v", err)
	}
	if gotUpdated.Password != "newpass" || gotUpdated.Plan != "ANNUAL" || gotUpdated.WhitelistedBrokers != "TestBroker" {
		t.Errorf("got %+v after update, want newpass/ANNUAL/TestBroker", gotUpdated)
	}
}

func TestProxyIPs_WhitelistedBrokers_BatchAndFullUpdate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	batch := []domain.ProxyIP{
		{
			IPAddress:          "198.51.100.101",
			IPType:             "ipv4",
			Host:               "batch.proxy.com",
			Port:               443,
			Username:           "buser1",
			Password:           "bpass1",
			WhitelistedBrokers:  "Zerodha",
			ValidFrom:          time.Now().Add(-1 * time.Hour),
			ValidUntil:         time.Now().Add(90 * 24 * time.Hour),
			Plan:               "QUARTERLY",
		},
		{
			IPAddress:          "198.51.100.102",
			IPType:             "ipv4",
			Host:               "batch.proxy.com",
			Port:               443,
			Username:           "buser2",
			Password:           "bpass2",
			WhitelistedBrokers:  "TestBroker",
			ValidFrom:          time.Now().Add(-1 * time.Hour),
			ValidUntil:         time.Now().Add(90 * 24 * time.Hour),
			Plan:               "QUARTERLY",
		},
	}

	res, err := store.UpsertProxyIPsBatch(ctx, batch)
	if err != nil {
		t.Fatalf("UpsertProxyIPsBatch failed: %v", err)
	}
	if res.Inserted != 2 {
		t.Fatalf("expected 2 inserted, got %d", res.Inserted)
	}

	// Verify via ListProxyIPs
	list, err := store.ListProxyIPs(ctx)
	if err != nil {
		t.Fatalf("ListProxyIPs failed: %v", err)
	}
	foundZerodha := false
	foundTestBroker := false
	for _, ip := range list {
		if ip.IPAddress == "198.51.100.101" && ip.WhitelistedBrokers == "Zerodha" {
			foundZerodha = true
		}
		if ip.IPAddress == "198.51.100.102" && ip.WhitelistedBrokers == "TestBroker" {
			foundTestBroker = true
		}
	}
	if !foundZerodha || !foundTestBroker {
		t.Errorf("ListProxyIPs failed to retrieve WhitelistedBrokers: foundZerodha=%v, foundTestBroker=%v, list=%+v", foundZerodha, foundTestBroker, list)
	}

	// UpdateProxyIPFull
	updateTarget := batch[0]
	updateTarget.WhitelistedBrokers = "Zerodha, TestBroker"
	if err := store.UpdateProxyIPFull(ctx, updateTarget); err != nil {
		t.Fatalf("UpdateProxyIPFull failed: %v", err)
	}

	gotFull, err := store.ProxyIPByAddress(ctx, updateTarget.IPAddress)
	if err != nil {
		t.Fatalf("ProxyIPByAddress failed: %v", err)
	}
	if gotFull.WhitelistedBrokers != "Zerodha, TestBroker" {
		t.Errorf("UpdateProxyIPFull did not update WhitelistedBrokers: got %q, want 'Zerodha, TestBroker'", gotFull.WhitelistedBrokers)
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
