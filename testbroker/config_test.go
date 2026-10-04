package main

import (
	"os"
	"path/filepath"
	"testing"
)

const testConfigJSON = `{
  "port": 9999,
  "postback_url": "http://localhost:8080/broker-callback",
  "execution_mode": "manual",
  "users": {
    "AB1234": {
      "api_key": "k1",
      "api_secret": "s1",
      "access_token": "t1",
      "role": "master"
    },
    "CD5678": {
      "api_key": "k2",
      "api_secret": "s2",
      "access_token": "t2",
      "role": "follower"
    }
  },
  "instruments": [
    {
      "exchange": "NFO",
      "tradingsymbol": "NIFTY26OCTFUT",
      "instrument_token": 408065,
      "lot_size": 75,
      "tick_size": 0.05,
      "ltp": 25000.0
    }
  ],
  "validation": {
    "lot_size_check": true,
    "max_quantity_per_order": 1800,
    "circuit_limit_pct": 10.0,
    "allowed_exchanges": ["NFO", "NSE"],
    "allowed_products": ["NRML", "MIS"],
    "allowed_order_types": ["MARKET", "LIMIT"]
  }
}`

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	path := writeTempConfig(t, testConfigJSON)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want 9999", cfg.Port)
	}
	if cfg.PostbackURL != "http://localhost:8080/broker-callback" {
		t.Errorf("PostbackURL = %q", cfg.PostbackURL)
	}
	if cfg.ExecutionMode != "manual" {
		t.Errorf("ExecutionMode = %q, want manual", cfg.ExecutionMode)
	}
	if len(cfg.Users) != 2 {
		t.Fatalf("len(Users) = %d, want 2", len(cfg.Users))
	}
	u := cfg.Users["AB1234"]
	if u.APIKey != "k1" || u.APISecret != "s1" || u.AccessToken != "t1" || u.Role != "master" {
		t.Errorf("user AB1234 = %+v", u)
	}
	if len(cfg.Instruments) != 1 {
		t.Fatalf("len(Instruments) = %d, want 1", len(cfg.Instruments))
	}
	inst := cfg.Instruments[0]
	if inst.LotSize != 75 || inst.Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("instrument = %+v", inst)
	}
	if !cfg.Validation.LotSizeCheck {
		t.Error("LotSizeCheck should be true")
	}
	if cfg.Validation.MaxQuantityPerOrder != 1800 {
		t.Errorf("MaxQuantityPerOrder = %d", cfg.Validation.MaxQuantityPerOrder)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	path := writeTempConfig(t, `{"users":{}, "instruments":[]}`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 8089 {
		t.Errorf("default Port = %d, want 8089", cfg.Port)
	}
	if cfg.ExecutionMode != "instant" {
		t.Errorf("default ExecutionMode = %q, want instant", cfg.ExecutionMode)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	path := writeTempConfig(t, `{not json}`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConfig_UserByToken(t *testing.T) {
	path := writeTempConfig(t, testConfigJSON)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	userID, ok := cfg.UserByToken("k1", "t1")
	if !ok || userID != "AB1234" {
		t.Errorf("UserByToken(k1, t1) = %q, %v; want AB1234, true", userID, ok)
	}

	_, ok = cfg.UserByToken("bad", "bad")
	if ok {
		t.Error("UserByToken should return false for unknown token")
	}
}
