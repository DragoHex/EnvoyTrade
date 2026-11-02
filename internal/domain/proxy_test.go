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
		{"dhan", "Dhan"},
		{"Dhan", "Dhan"},
		{"dhan, kite", "Dhan, Zerodha"},
		{"kotak", "Kotak"},
		{"Kotak", "Kotak"},
		{"kotakneo", "Kotak"},
		{"KotakNeo", "Kotak"},
		{"kotak_neo", "Kotak"},
		{"kotak-neo", "Kotak"},
		{"Kotak, Zerodha", "Kotak, Zerodha"},
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
	pDhan := domain.ProxyIP{WhitelistedBrokers: "Dhan"}
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

	// pDhan
	if !pDhan.MatchesBroker("dhan") {
		t.Errorf("pDhan should match 'dhan'")
	}
	if !pDhan.MatchesBroker("Dhan") {
		t.Errorf("pDhan should match 'Dhan'")
	}
	if pDhan.MatchesBroker("zerodha") {
		t.Errorf("pDhan should NOT match 'zerodha'")
	}

	// pKotak
	pKotak := domain.ProxyIP{WhitelistedBrokers: "Kotak"}
	if !pKotak.MatchesBroker("kotak") {
		t.Errorf("pKotak should match 'kotak'")
	}
	if !pKotak.MatchesBroker("Kotak") {
		t.Errorf("pKotak should match 'Kotak'")
	}
	if !pKotak.MatchesBroker("kotakneo") {
		t.Errorf("pKotak should match 'kotakneo'")
	}
	if !pKotak.MatchesBroker("KotakNeo") {
		t.Errorf("pKotak should match 'KotakNeo'")
	}
	if !pKotak.MatchesBroker("kotak_neo") {
		t.Errorf("pKotak should match 'kotak_neo'")
	}
	if !pKotak.MatchesBroker("kotak-neo") {
		t.Errorf("pKotak should match 'kotak-neo'")
	}
	if pKotak.MatchesBroker("zerodha") {
		t.Errorf("pKotak should NOT match 'zerodha'")
	}
	if pKotak.MatchesBroker("dhan") {
		t.Errorf("pKotak should NOT match 'dhan'")
	}

	// pEmpty matches all (backwards compatible)
	if !pEmpty.MatchesBroker("zerodha") {
		t.Errorf("pEmpty should match 'zerodha'")
	}
	if !pEmpty.MatchesBroker("dhan") {
		t.Errorf("pEmpty should match 'dhan'")
	}
	if !pEmpty.MatchesBroker("testbroker") {
		t.Errorf("pEmpty should match 'testbroker'")
	}
	if !pEmpty.MatchesBroker("kotakneo") {
		t.Errorf("pEmpty should match 'kotakneo'")
	}
}

func TestProxyIP_BrokerPriority(t *testing.T) {
	pZerodha := domain.ProxyIP{WhitelistedBrokers: "Zerodha"}
	pDhan := domain.ProxyIP{WhitelistedBrokers: "Dhan"}
	pKotak := domain.ProxyIP{WhitelistedBrokers: "Kotak"}
	pBoth := domain.ProxyIP{WhitelistedBrokers: "Zerodha, TestBroker"}
	pEmpty := domain.ProxyIP{WhitelistedBrokers: ""}

	// Priority constants: 0 = incompatible, 1 = specialized, 2 = all brokers (unrestricted)

	// For Kite / Zerodha
	if p := pZerodha.BrokerPriority("kite"); p != 1 {
		t.Errorf("pZerodha for 'kite' got priority %d, want 1", p)
	}
	if p := pZerodha.BrokerPriority("zerodha"); p != 1 {
		t.Errorf("pZerodha for 'zerodha' got priority %d, want 1", p)
	}
	if p := pBoth.BrokerPriority("kite"); p != 1 {
		t.Errorf("pBoth for 'kite' got priority %d, want 1", p)
	}
	if p := pEmpty.BrokerPriority("kite"); p != 2 {
		t.Errorf("pEmpty for 'kite' got priority %d, want 2", p)
	}
	if p := pDhan.BrokerPriority("kite"); p != 0 {
		t.Errorf("pDhan for 'kite' got priority %d, want 0", p)
	}

	// For Dhan
	if p := pDhan.BrokerPriority("dhan"); p != 1 {
		t.Errorf("pDhan for 'dhan' got priority %d, want 1", p)
	}
	if p := pEmpty.BrokerPriority("dhan"); p != 2 {
		t.Errorf("pEmpty for 'dhan' got priority %d, want 2", p)
	}
	if p := pZerodha.BrokerPriority("dhan"); p != 0 {
		t.Errorf("pZerodha for 'dhan' got priority %d, want 0", p)
	}
	if p := pBoth.BrokerPriority("dhan"); p != 0 {
		t.Errorf("pBoth for 'dhan' got priority %d, want 0", p)
	}

	// For Kotak Neo
	if p := pKotak.BrokerPriority("kotakneo"); p != 1 {
		t.Errorf("pKotak for 'kotakneo' got priority %d, want 1", p)
	}
	if p := pKotak.BrokerPriority("kotak"); p != 1 {
		t.Errorf("pKotak for 'kotak' got priority %d, want 1", p)
	}
	if p := pEmpty.BrokerPriority("kotakneo"); p != 2 {
		t.Errorf("pEmpty for 'kotakneo' got priority %d, want 2", p)
	}
	if p := pZerodha.BrokerPriority("kotakneo"); p != 0 {
		t.Errorf("pZerodha for 'kotakneo' got priority %d, want 0", p)
	}
	if p := pDhan.BrokerPriority("kotakneo"); p != 0 {
		t.Errorf("pDhan for 'kotakneo' got priority %d, want 0", p)
	}
}

func TestParseProxyConfigText(t *testing.T) {
	rawConfig := `IP (to be whitelisted in broker): 2401:c080:2400:16b8:7c15:2ea3:64b6:5a00
Host: dc6-mum-01.algoip.in
Port: 443
user_id: aip_live_lq0z72n0s2uagmm6
password: aip_sec_pctja3asoghh021117cl1gtw5i1h4566
Whitelisted Brokers: Kotak
Valid From: 2026-11-02 11:17:55 (IST)
Valid Until: 2027-02-02 11:17:55 (IST)
Plan: QUARTERLY`

	input, err := domain.ParseProxyConfigText(rawConfig)
	if err != nil {
		t.Fatalf("unexpected error parsing proxy config text: %v", err)
	}

	if input.IP != "2401:c080:2400:16b8:7c15:2ea3:64b6:5a00" {
		t.Errorf("expected IP 2401:c080:2400:16b8:7c15:2ea3:64b6:5a00, got %q", input.IP)
	}
	if input.Host != "dc6-mum-01.algoip.in" {
		t.Errorf("expected Host dc6-mum-01.algoip.in, got %q", input.Host)
	}
	if input.Port != 443 {
		t.Errorf("expected Port 443, got %d", input.Port)
	}
	if input.UserID != "aip_live_lq0z72n0s2uagmm6" {
		t.Errorf("expected UserID aip_live_lq0z72n0s2uagmm6, got %q", input.UserID)
	}
	if input.Password != "aip_sec_pctja3asoghh021117cl1gtw5i1h4566" {
		t.Errorf("expected Password aip_sec_pctja3asoghh021117cl1gtw5i1h4566, got %q", input.Password)
	}
	if input.WhitelistedBrokers != "Kotak" {
		t.Errorf("expected WhitelistedBrokers Kotak, got %q", input.WhitelistedBrokers)
	}
	if input.ValidFrom != "2026-11-02 11:17:55 (IST)" {
		t.Errorf("expected ValidFrom 2026-11-02 11:17:55 (IST), got %q", input.ValidFrom)
	}
	if input.ValidUntil != "2027-02-02 11:17:55 (IST)" {
		t.Errorf("expected ValidUntil 2027-02-02 11:17:55 (IST), got %q", input.ValidUntil)
	}
	if input.Plan != "QUARTERLY" {
		t.Errorf("expected Plan QUARTERLY, got %q", input.Plan)
	}

	// Now validate through ParseAndValidateProxyIP
	p, err := domain.ParseAndValidateProxyIP(input, time.Now())
	if err != nil {
		t.Fatalf("unexpected error validating parsed proxy IP: %v", err)
	}
	if p.IPType != "ipv6" {
		t.Errorf("expected IPType ipv6, got %q", p.IPType)
	}
	if p.WhitelistedBrokers != "Kotak" {
		t.Errorf("expected WhitelistedBrokers Kotak, got %q", p.WhitelistedBrokers)
	}
	if p.Port != 443 {
		t.Errorf("expected Port 443, got %d", p.Port)
	}

	// Negative tests
	t.Run("missing IP", func(t *testing.T) {
		_, err := domain.ParseProxyConfigText("Host: test.com\nPort: 443\n")
		if err == nil {
			t.Errorf("expected error for missing IP, got nil")
		}
	})

	t.Run("missing Host", func(t *testing.T) {
		_, err := domain.ParseProxyConfigText("IP: 10.0.0.1\nPort: 443\n")
		if err == nil {
			t.Errorf("expected error for missing Host, got nil")
		}
	})

	t.Run("json text input", func(t *testing.T) {
		jsonText := `{
  "ip": "2401:c080:2400:16b8:c19:6ec5:4ebd:4e5f",
  "host": "dc6-mum-01.algoip.in",
  "port": 443,
  "user_id": "aip_live_inx8bbazx5to5w6e",
  "password": "",
  "valid_until": "2027-01-27 23:59:59 (IST)",
  "plan": "QUARTERLY",
  "type": "IPV6",
  "whitelisted_brokers": "Dhan"
}`
		parsed, err := domain.ParseProxyConfigText(jsonText)
		if err != nil {
			t.Fatalf("unexpected error parsing json text in ParseProxyConfigText: %v", err)
		}
		if parsed.IP != "2401:c080:2400:16b8:c19:6ec5:4ebd:4e5f" {
			t.Errorf("expected IP 2401:c080:2400:16b8:c19:6ec5:4ebd:4e5f, got %q", parsed.IP)
		}
		if parsed.Host != "dc6-mum-01.algoip.in" {
			t.Errorf("expected Host dc6-mum-01.algoip.in, got %q", parsed.Host)
		}
		if parsed.Port != 443 {
			t.Errorf("expected Port 443, got %d", parsed.Port)
		}
		if parsed.UserID != "aip_live_inx8bbazx5to5w6e" {
			t.Errorf("expected UserID aip_live_inx8bbazx5to5w6e, got %q", parsed.UserID)
		}
		if parsed.WhitelistedBrokers != "Dhan" {
			t.Errorf("expected WhitelistedBrokers Dhan, got %q", parsed.WhitelistedBrokers)
		}
	})
}


