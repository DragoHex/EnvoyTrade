package domain_test

import (
	"testing"
	"time"

	"envoytrade/internal/domain"
)

func TestComputeExpiry(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		validUntil   time.Time
		wantDaysMin  int
		wantDaysMax  int
		wantStatus   domain.ExpiryStatus
	}{
		{
			name:        "active with 60 days remaining",
			validUntil:  now.AddDate(0, 2, 0),
			wantDaysMin: 59,
			wantDaysMax: 62,
			wantStatus:  domain.ExpiryActive,
		},
		{
			name:        "boundary expiring soon at exactly 15 days",
			validUntil:  now.Add(15 * 24 * time.Hour),
			wantDaysMin: 15,
			wantDaysMax: 15,
			wantStatus:  domain.ExpiryExpiringSoon,
		},
		{
			name:        "boundary expiring soon at 1 day",
			validUntil:  now.Add(24 * time.Hour),
			wantDaysMin: 1,
			wantDaysMax: 1,
			wantStatus:  domain.ExpiryExpiringSoon,
		},
		{
			name:        "boundary expiring soon today 0 days",
			validUntil:  now.Add(2 * time.Hour),
			wantDaysMin: 0,
			wantDaysMax: 0,
			wantStatus:  domain.ExpiryExpiringSoon,
		},
		{
			name:        "expired 1 day ago",
			validUntil:  now.Add(-25 * time.Hour),
			wantDaysMin: -2,
			wantDaysMax: -1,
			wantStatus:  domain.ExpiryExpired,
		},
		{
			name:        "zero time returns expired",
			validUntil:  time.Time{},
			wantDaysMin: 0,
			wantDaysMax: 0,
			wantStatus:  domain.ExpiryExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, status := domain.ComputeExpiry(tt.validUntil, now)
			if days < tt.wantDaysMin || days > tt.wantDaysMax {
				t.Errorf("ComputeExpiry() days = %d, want between [%d, %d]", days, tt.wantDaysMin, tt.wantDaysMax)
			}
			if status != tt.wantStatus {
				t.Errorf("ComputeExpiry() status = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}

func TestParseProxyTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		wantUTC string
	}{
		{
			name:    "custom IST format with (IST) suffix",
			raw:     "2026-10-06 08:57:22 (IST)",
			wantErr: false,
			// 08:57:22 IST is 03:27:22 UTC
			wantUTC: "2026-10-06 03:27:22",
		},
		{
			name:    "custom IST format without parentheses",
			raw:     "2026-10-06 08:57:22",
			wantErr: false,
			wantUTC: "2026-10-06 03:27:22",
		},
		{
			name:    "RFC3339 format",
			raw:     "2026-10-06T03:27:22Z",
			wantErr: false,
			wantUTC: "2026-10-06 03:27:22",
		},
		{
			name:    "empty string returns zero time",
			raw:     "",
			wantErr: false,
			wantUTC: "",
		},
		{
			name:    "invalid format returns error",
			raw:     "not-a-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseProxyTimestamp(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProxyTimestamp() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.wantUTC != "" {
				formatted := got.Format("2006-01-02 15:04:05")
				if formatted != tt.wantUTC {
					t.Errorf("ParseProxyTimestamp() = %s UTC, want %s UTC", formatted, tt.wantUTC)
				}
			}
		})
	}
}

func TestParseAndValidateProxyIP(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

	t.Run("valid IPv4 from ips.json schema", func(t *testing.T) {
		input := domain.ProxyIPInput{
			IP:         "148.113.41.42",
			Host:       "dc46-mum-01.algoip.in",
			Port:       443,
			UserID:     "aip_live_trnbam1he8ndlerw",
			Password:   "aip_sec_qxgf0tu7317loli1p44s1d12y4dfgiyh",
			ValidFrom:  "2026-09-29 19:35:15 (IST)",
			ValidUntil: "2026-12-29 19:35:15 (IST)",
			Plan:       "QUARTERLY",
			Type:       "IPV4",
		}

		p, err := domain.ParseAndValidateProxyIP(input, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if p.IPAddress != "148.113.41.42" {
			t.Errorf("IPAddress = %s, want 148.113.41.42", p.IPAddress)
		}
		if p.IPType != "ipv4" {
			t.Errorf("IPType = %s, want ipv4", p.IPType)
		}
		if p.Username != "aip_live_trnbam1he8ndlerw" {
			t.Errorf("Username = %s, want aip_live_trnbam1he8ndlerw", p.Username)
		}
		if p.ExpiryStatus != domain.ExpiryActive {
			t.Errorf("ExpiryStatus = %v, want active", p.ExpiryStatus)
		}
	})

	t.Run("valid IPv6 from ips.json schema", func(t *testing.T) {
		input := domain.ProxyIPInput{
			IP:         "2401:c080:2400:16b8:9828:f266:cc87:9162",
			Host:       "dc6-mum-01.algoip.in",
			Port:       443,
			UserID:     "aip_live_2ljaj2bajq7zvw1q",
			Password:   "aip_sec_d5tk8cb7vga9h68bfe0tblqx0kc6ljyr",
			ValidFrom:  "2026-10-06 08:57:22 (IST)",
			ValidUntil: "2027-01-06 08:57:22 (IST)",
			Plan:       "QUARTERLY",
			Type:       "IPV6",
		}

		p, err := domain.ParseAndValidateProxyIP(input, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if p.IPType != "ipv6" {
			t.Errorf("IPType = %s, want ipv6", p.IPType)
		}
	})

	t.Run("negative: empty IP", func(t *testing.T) {
		input := domain.ProxyIPInput{Host: "proxy.com"}
		_, err := domain.ParseAndValidateProxyIP(input, now)
		if err == nil {
			t.Fatal("expected error for empty IP, got nil")
		}
	})

	t.Run("negative: malformed IP", func(t *testing.T) {
		input := domain.ProxyIPInput{IP: "999.999.999.999", Host: "proxy.com"}
		_, err := domain.ParseAndValidateProxyIP(input, now)
		if err == nil {
			t.Fatal("expected error for malformed IP, got nil")
		}
	})

	t.Run("negative: empty host", func(t *testing.T) {
		input := domain.ProxyIPInput{IP: "1.1.1.1"}
		_, err := domain.ParseAndValidateProxyIP(input, now)
		if err == nil {
			t.Fatal("expected error for empty host, got nil")
		}
	})

	t.Run("defaults: port 443 and inferred type", func(t *testing.T) {
		input := domain.ProxyIPInput{
			IP:   "1.2.3.4",
			Host: "proxy.com",
		}
		p, err := domain.ParseAndValidateProxyIP(input, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Port != 443 {
			t.Errorf("Port = %d, want default 443", p.Port)
		}
		if p.IPType != "ipv4" {
			t.Errorf("IPType = %s, want inferred ipv4", p.IPType)
		}
		if p.Plan != "QUARTERLY" {
			t.Errorf("Plan = %s, want default QUARTERLY", p.Plan)
		}
	})

	t.Run("whitelisted_brokers parsing and normalization", func(t *testing.T) {
		input := domain.ProxyIPInput{
			IP:                 "1.2.3.4",
			Host:               "proxy.com",
			WhitelistedBrokers: "kite, testbroker",
		}
		p, err := domain.ParseAndValidateProxyIP(input, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.WhitelistedBrokers != "Zerodha, TestBroker" {
			t.Errorf("WhitelistedBrokers = %q, want 'Zerodha, TestBroker'", p.WhitelistedBrokers)
		}
	})

	t.Run("whitelistedBrokers camelCase parsing", func(t *testing.T) {
		input := domain.ProxyIPInput{
			IP:                      "1.2.3.4",
			Host:                    "proxy.com",
			WhitelistedBrokersCamel: "zerodha",
		}
		p, err := domain.ParseAndValidateProxyIP(input, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.WhitelistedBrokers != "Zerodha" {
			t.Errorf("WhitelistedBrokers = %q, want 'Zerodha'", p.WhitelistedBrokers)
		}
	})
}

func TestNormalizeWhitelistedBrokers(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"zerodha", "Zerodha"},
		{"Zerodha", "Zerodha"},
		{"kite", "Zerodha"},
		{"Kite", "Zerodha"},
		{"testbroker", "TestBroker"},
		{"TestBroker", "TestBroker"},
		{"test_broker", "TestBroker"},
		{"zerodha, testbroker", "Zerodha, TestBroker"},
		{"Kite, TestBroker", "Zerodha, TestBroker"},
		{"unknown_broker", "unknown_broker"},
	}

	for _, tt := range tests {
		got := domain.NormalizeWhitelistedBrokers(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeWhitelistedBrokers(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProxyIP_MatchesBroker(t *testing.T) {
	pZerodha := domain.ProxyIP{WhitelistedBrokers: "Zerodha"}
	pTest := domain.ProxyIP{WhitelistedBrokers: "TestBroker"}
	pBoth := domain.ProxyIP{WhitelistedBrokers: "Zerodha, TestBroker"}
	pEmpty := domain.ProxyIP{WhitelistedBrokers: ""}

	// pZerodha
	if !pZerodha.MatchesBroker("zerodha") {
		t.Errorf("pZerodha should match 'zerodha'")
	}
	if !pZerodha.MatchesBroker("kite") {
		t.Errorf("pZerodha should match 'kite'")
	}
	if pZerodha.MatchesBroker("testbroker") {
		t.Errorf("pZerodha should NOT match 'testbroker'")
	}

	// pTest
	if !pTest.MatchesBroker("testbroker") {
		t.Errorf("pTest should match 'testbroker'")
	}
	if pTest.MatchesBroker("zerodha") {
		t.Errorf("pTest should NOT match 'zerodha'")
	}
	if pTest.MatchesBroker("kite") {
		t.Errorf("pTest should NOT match 'kite'")
	}

	// pBoth
	if !pBoth.MatchesBroker("zerodha") {
		t.Errorf("pBoth should match 'zerodha'")
	}
	if !pBoth.MatchesBroker("kite") {
		t.Errorf("pBoth should match 'kite'")
	}
	if !pBoth.MatchesBroker("testbroker") {
		t.Errorf("pBoth should match 'testbroker'")
	}

	// pEmpty matches all (backwards compatible)
	if !pEmpty.MatchesBroker("zerodha") {
		t.Errorf("pEmpty should match 'zerodha'")
	}
	if !pEmpty.MatchesBroker("testbroker") {
		t.Errorf("pEmpty should match 'testbroker'")
	}
}
