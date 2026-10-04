package sdk

import "fmt"

// Variety constants matching Kite Connect.
const (
	VarietyRegular = "regular"
	VarietyAMO     = "amo"
)

// TransactionType constants.
const (
	TransactionTypeBuy  = "BUY"
	TransactionTypeSell = "SELL"
)

// OrderType constants.
const (
	OrderTypeMarket = "MARKET"
	OrderTypeLimit  = "LIMIT"
	OrderTypeSL     = "SL"
	OrderTypeSLM    = "SL-M"
)

// Product constants.
const (
	ProductNRML = "NRML"
	ProductMIS  = "MIS"
	ProductCNC  = "CNC"
)

// Order status constants.
const (
	OrderStatusOpen      = "OPEN"
	OrderStatusComplete  = "COMPLETE"
	OrderStatusRejected  = "REJECTED"
	OrderStatusCancelled = "CANCELLED"
)

// OrderParams represents parameters for placing or modifying an order.
type OrderParams struct {
	Exchange        string  `json:"exchange" url:"exchange"`
	Tradingsymbol   string  `json:"tradingsymbol" url:"tradingsymbol"`
	TransactionType string  `json:"transaction_type" url:"transaction_type"`
	Product         string  `json:"product" url:"product"`
	OrderType       string  `json:"order_type" url:"order_type"`
	Quantity        int     `json:"quantity" url:"quantity"`
	Price           float64 `json:"price,omitempty" url:"price,omitempty"`
	TriggerPrice    float64 `json:"trigger_price,omitempty" url:"trigger_price,omitempty"`
	Tag             string  `json:"tag,omitempty" url:"tag,omitempty"`
	Validity        string  `json:"validity,omitempty" url:"validity,omitempty"`
}

// OrderResponse represents the order placement response.
type OrderResponse struct {
	OrderID string `json:"order_id"`
}

// Order represents an individual order in Kite Connect format.
type Order struct {
	OrderID                 string  `json:"order_id"`
	ExchangeOrderID         string  `json:"exchange_order_id"`
	ParentOrderID           string  `json:"parent_order_id"`
	UserID                  string  `json:"user_id"`
	Status                  string  `json:"status"`
	StatusMessage           string  `json:"status_message"`
	StatusMessageRaw        string  `json:"status_message_raw"`
	OrderTimestamp          string  `json:"order_timestamp"`
	ExchangeUpdateTimestamp string  `json:"exchange_update_timestamp"`
	ExchangeTimestamp       string  `json:"exchange_timestamp"`
	Variety                 string  `json:"variety"`
	Modified                bool    `json:"modified"`
	Exchange                string  `json:"exchange"`
	Tradingsymbol           string  `json:"tradingsymbol"`
	InstrumentToken         int     `json:"instrument_token"`
	OrderType               string  `json:"order_type"`
	TransactionType         string  `json:"transaction_type"`
	Validity                string  `json:"validity"`
	ValidityTTL            int     `json:"validity_ttl"`
	Product                 string  `json:"product"`
	Quantity                int     `json:"quantity"`
	DisclosedQuantity      int     `json:"disclosed_quantity"`
	Price                  float64 `json:"price"`
	TriggerPrice           float64 `json:"trigger_price"`
	AveragePrice           float64 `json:"average_price"`
	FilledQuantity         int     `json:"filled_quantity"`
	PendingQuantity        int     `json:"pending_quantity"`
	CancelledQuantity      int     `json:"cancelled_quantity"`
	MarketProtection       int     `json:"market_protection"`
	Tag                    string  `json:"tag"`
	GUID                   string  `json:"guid"`
	Checksum               string  `json:"checksum,omitempty"`
}

// Position represents an individual net or day position.
type Position struct {
	Tradingsymbol   string  `json:"tradingsymbol"`
	Exchange        string  `json:"exchange"`
	InstrumentToken int     `json:"instrument_token"`
	Product         string  `json:"product"`
	Quantity        int     `json:"quantity"`
	AveragePrice    float64 `json:"average_price"`
	LastPrice       float64 `json:"last_price"`
	Value           float64 `json:"value"`
	PnL             float64 `json:"pnl"`
	BuyQuantity     int     `json:"buy_quantity"`
	BuyPrice        float64 `json:"buy_price"`
	SellQuantity    int     `json:"sell_quantity"`
	SellPrice       float64 `json:"sell_price"`
}

// Positions represents a collection of net and day positions.
type Positions struct {
	Net []Position `json:"net"`
	Day []Position `json:"day"`
}

// Holding represents an equity holding.
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

// MarginAvailable represents available margin cash/balance.
type MarginAvailable struct {
	Cash        float64 `json:"cash"`
	LiveBalance float64 `json:"live_balance"`
}

// Margins represents margin details for an account segment.
type Margins struct {
	Enabled   bool            `json:"enabled"`
	Net       float64         `json:"net"`
	Available MarginAvailable `json:"available"`
	Utilised  map[string]any  `json:"utilised"`
}

// AllMargins contains equity and commodity margins.
type AllMargins struct {
	Equity    Margins `json:"equity"`
	Commodity Margins `json:"commodity"`
}

// QuoteLTP represents the LTP quote for a single instrument.
type QuoteLTP struct {
	InstrumentToken int     `json:"instrument_token"`
	LastPrice       float64 `json:"last_price"`
}

// UserSession represents session response from token generation.
type UserSession struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
}

// KiteError represents an API error envelope from Kite Connect / Mock Broker.
type KiteError struct {
	StatusCode int    `json:"-"`
	Status     string `json:"status"`
	ErrorType  string `json:"error_type"`
	Message    string `json:"message"`
}

func (e *KiteError) Error() string {
	return fmt.Sprintf("KiteError [%d %s]: %s", e.StatusCode, e.ErrorType, e.Message)
}
