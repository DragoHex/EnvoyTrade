package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the complete test broker configuration.
type Config struct {
	Port          int                   `json:"port"`
	PostbackURL   string                `json:"postback_url"`
	ExecutionMode string                `json:"execution_mode"` // "instant", "manual", "delayed"
	Users         map[string]UserConfig `json:"users"`          // keyed by broker user ID (e.g. "AB1234")
	Instruments   []Instrument          `json:"instruments"`
	Validation    ValidationConfig      `json:"validation"`
	Orders        []Order               `json:"orders,omitempty"`

	// tokenIndex is built on load for O(1) auth lookups: "api_key:access_token" -> userID
	tokenIndex map[string]string
}

// Holding defines an equity holding in the mock.
type Holding struct {
	Tradingsymbol   string  `json:"tradingsymbol"`
	Exchange        string  `json:"exchange"`
	InstrumentToken int     `json:"instrument_token"`
	ISIN            string  `json:"isin"`
	Product         string  `json:"product"`
	Quantity        int     `json:"quantity"`
	AveragePrice    float64 `json:"average_price"`
	LastPrice       float64 `json:"last_price"`
	PnL             float64 `json:"pnl"`
}

// UserConfig defines a simulated broker account.
type UserConfig struct {
	APIKey      string    `json:"api_key"`
	APISecret   string    `json:"api_secret"`
	AccessToken string    `json:"access_token"`
	Role        string    `json:"role"` // "master" | "follower"
	Holdings    []Holding `json:"holdings,omitempty"`
}

// Instrument defines a tradeable instrument in the mock.
type Instrument struct {
	Exchange        string  `json:"exchange"`
	Tradingsymbol   string  `json:"tradingsymbol"`
	InstrumentToken int     `json:"instrument_token"`
	LotSize         int     `json:"lot_size"`
	TickSize        float64 `json:"tick_size"`
	LTP             float64 `json:"ltp"`
}

// ValidationConfig holds order validation rules.
type ValidationConfig struct {
	LotSizeCheck        bool     `json:"lot_size_check"`
	MaxQuantityPerOrder int      `json:"max_quantity_per_order"`
	CircuitLimitPct     float64  `json:"circuit_limit_pct"`
	AllowedExchanges    []string `json:"allowed_exchanges"`
	AllowedProducts     []string `json:"allowed_products"`
	AllowedOrderTypes   []string `json:"allowed_order_types"`
}

// LoadConfig reads and parses a testbroker config JSON file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Defaults
	if cfg.Port == 0 {
		cfg.Port = 8089
	}
	if cfg.ExecutionMode == "" {
		cfg.ExecutionMode = "instant"
	}

	// Build token index for auth lookups
	cfg.tokenIndex = make(map[string]string, len(cfg.Users))
	for userID, u := range cfg.Users {
		key := u.APIKey + ":" + u.AccessToken
		cfg.tokenIndex[key] = userID
	}

	return &cfg, nil
}

// UserByToken resolves a user ID from an api_key + access_token pair.
func (c *Config) UserByToken(apiKey, accessToken string) (string, bool) {
	userID, ok := c.tokenIndex[apiKey+":"+accessToken]
	return userID, ok
}
