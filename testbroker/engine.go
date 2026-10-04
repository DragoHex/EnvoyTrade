package main

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Order mirrors kiteconnect.Order's JSON shape — standalone, no SDK import.
type Order struct {
	OrderID                string  `json:"order_id"`
	ExchangeOrderID        string  `json:"exchange_order_id"`
	ParentOrderID          string  `json:"parent_order_id"`
	UserID                 string  `json:"user_id"`
	Status                 string  `json:"status"`
	StatusMessage          string  `json:"status_message"`
	StatusMessageRaw       string  `json:"status_message_raw"`
	OrderTimestamp         string  `json:"order_timestamp"`
	ExchangeUpdateTimestamp string  `json:"exchange_update_timestamp"`
	ExchangeTimestamp      string  `json:"exchange_timestamp"`
	Variety                string  `json:"variety"`
	Modified               bool    `json:"modified"`
	Exchange               string  `json:"exchange"`
	Tradingsymbol          string  `json:"tradingsymbol"`
	InstrumentToken        int     `json:"instrument_token"`
	OrderType              string  `json:"order_type"`
	TransactionType        string  `json:"transaction_type"`
	Validity               string  `json:"validity"`
	ValidityTTL            int     `json:"validity_ttl"`
	Product                string  `json:"product"`
	Quantity               int     `json:"quantity"`
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

// OrderParams mirrors the subset of fields needed to place an order.
type OrderParams struct {
	Exchange        string  `json:"exchange"`
	Tradingsymbol   string  `json:"tradingsymbol"`
	TransactionType string  `json:"transaction_type"`
	Product         string  `json:"product"`
	OrderType       string  `json:"order_type"`
	Quantity        int     `json:"quantity"`
	Price           float64 `json:"price"`
	TriggerPrice    float64 `json:"trigger_price"`
	Tag             string  `json:"tag"`
}

// OrderResponse is the Kite-style success response payload.
type OrderResponse struct {
	OrderID string `json:"order_id"`
}

const kiteTimeFmt = "2006-01-02 15:04:05"

// OrderEngine manages per-user order books, validation, and state transitions.
type OrderEngine struct {
	mu          sync.RWMutex
	config      *Config
	instruments map[string]*Instrument // "NFO:NIFTY26OCTFUT" -> *Instrument
	orders      map[string]*Order      // orderID -> *Order (global index)
	userOrders  map[string][]string    // userID -> []orderID (preserves insertion order)
	history     map[string][]Order     // orderID -> []Order snapshots
	nextID      atomic.Int64
	onTerminal  []func(*Order)
}

// NewOrderEngine creates an engine from config.
func NewOrderEngine(cfg *Config) *OrderEngine {
	e := &OrderEngine{
		config:      cfg,
		instruments: make(map[string]*Instrument, len(cfg.Instruments)),
		orders:      make(map[string]*Order),
		userOrders:  make(map[string][]string),
		history:     make(map[string][]Order),
	}
	for i := range cfg.Instruments {
		inst := &cfg.Instruments[i]
		e.instruments[inst.Exchange+":"+inst.Tradingsymbol] = inst
	}
	for i := range cfg.Orders {
		o := cfg.Orders[i]
		e.orders[o.OrderID] = &o
		e.userOrders[o.UserID] = append(e.userOrders[o.UserID], o.OrderID)
		e.history[o.OrderID] = []Order{o}
	}
	return e
}

// OnTerminal registers a hook called whenever an order reaches a terminal state.
func (e *OrderEngine) OnTerminal(fn func(*Order)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onTerminal = append(e.onTerminal, fn)
}

func (e *OrderEngine) genOrderID() string {
	id := e.nextID.Add(1)
	return fmt.Sprintf("%s%06d", time.Now().Format("060102150405"), id%1000000)
}

// PlaceOrder validates and places an order for the given user.
func (e *OrderEngine) PlaceOrder(userID, variety string, p OrderParams) (OrderResponse, error) {
	// Validate exchange
	if !contains(e.config.Validation.AllowedExchanges, p.Exchange) {
		return OrderResponse{}, fmt.Errorf("InputError: exchange %q not allowed", p.Exchange)
	}
	// Validate product
	if !contains(e.config.Validation.AllowedProducts, p.Product) {
		return OrderResponse{}, fmt.Errorf("InputError: product %q not allowed", p.Product)
	}
	// Validate order type
	if !contains(e.config.Validation.AllowedOrderTypes, p.OrderType) {
		return OrderResponse{}, fmt.Errorf("InputError: order_type %q not allowed", p.OrderType)
	}

	// Resolve instrument
	key := p.Exchange + ":" + p.Tradingsymbol
	e.mu.RLock()
	inst, ok := e.instruments[key]
	e.mu.RUnlock()
	if !ok {
		return OrderResponse{}, fmt.Errorf("InputError: unknown instrument %s", key)
	}

	// Lot size check
	if e.config.Validation.LotSizeCheck && inst.LotSize > 0 && p.Quantity%inst.LotSize != 0 {
		return OrderResponse{}, fmt.Errorf("InputError: quantity %d is not a multiple of lot size %d", p.Quantity, inst.LotSize)
	}

	// Freeze quantity check
	if e.config.Validation.MaxQuantityPerOrder > 0 && p.Quantity > e.config.Validation.MaxQuantityPerOrder {
		return OrderResponse{}, fmt.Errorf("OrderException: quantity %d exceeds freeze limit %d", p.Quantity, e.config.Validation.MaxQuantityPerOrder)
	}

	// Price band check for LIMIT orders
	if p.OrderType == "LIMIT" && e.config.Validation.CircuitLimitPct > 0 && inst.LTP > 0 {
		band := inst.LTP * e.config.Validation.CircuitLimitPct / 100.0
		lo, hi := inst.LTP-band, inst.LTP+band
		if p.Price < lo || p.Price > hi {
			return OrderResponse{}, fmt.Errorf("OrderException: price %.2f outside circuit band [%.2f, %.2f]", p.Price, lo, hi)
		}
	}

	now := time.Now().Format(kiteTimeFmt)
	orderID := e.genOrderID()

	o := &Order{
		OrderID:                orderID,
		ExchangeOrderID:        fmt.Sprintf("1%s", orderID[1:]),
		UserID:                 userID,
		Status:                 "OPEN",
		OrderTimestamp:         now,
		ExchangeUpdateTimestamp: now,
		ExchangeTimestamp:      now,
		Variety:                variety,
		Exchange:               p.Exchange,
		Tradingsymbol:          p.Tradingsymbol,
		InstrumentToken:        inst.InstrumentToken,
		OrderType:              p.OrderType,
		TransactionType:        p.TransactionType,
		Validity:               "DAY",
		Product:                p.Product,
		Quantity:               p.Quantity,
		Price:                  p.Price,
		TriggerPrice:           p.TriggerPrice,
		PendingQuantity:        p.Quantity,
		Tag:                    p.Tag,
	}

	e.mu.Lock()
	e.orders[orderID] = o
	e.userOrders[userID] = append(e.userOrders[userID], orderID)
	e.history[orderID] = []Order{*o} // snapshot OPEN state
	mode := e.config.ExecutionMode
	hooks := e.onTerminal
	e.mu.Unlock()

	// Instant fill
	if mode == "instant" {
		e.fillOrderLocked(o, inst.LTP, hooks)
	}

	return OrderResponse{OrderID: orderID}, nil
}

// FillOrder transitions an OPEN order to COMPLETE at the given price.
func (e *OrderEngine) FillOrder(orderID string, price float64) error {
	e.mu.Lock()
	o, ok := e.orders[orderID]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("order %s not found", orderID)
	}
	if o.Status != "OPEN" {
		e.mu.Unlock()
		return fmt.Errorf("order %s is %s, not OPEN", orderID, o.Status)
	}
	hooks := e.onTerminal
	e.mu.Unlock()

	e.fillOrderLocked(o, price, hooks)
	return nil
}

func (e *OrderEngine) fillOrderLocked(o *Order, price float64, hooks []func(*Order)) {
	now := time.Now().Format(kiteTimeFmt)

	e.mu.Lock()
	o.Status = "COMPLETE"
	o.AveragePrice = price
	o.FilledQuantity = o.Quantity
	o.PendingQuantity = 0
	o.ExchangeUpdateTimestamp = now
	o.ExchangeTimestamp = now
	e.history[o.OrderID] = append(e.history[o.OrderID], *o)
	e.mu.Unlock()

	for _, fn := range hooks {
		fn(o)
	}
}

// RejectOrder transitions an OPEN order to REJECTED.
func (e *OrderEngine) RejectOrder(orderID, reason string) error {
	e.mu.Lock()
	o, ok := e.orders[orderID]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("order %s not found", orderID)
	}
	if o.Status != "OPEN" {
		e.mu.Unlock()
		return fmt.Errorf("order %s is %s, not OPEN", orderID, o.Status)
	}
	hooks := e.onTerminal

	now := time.Now().Format(kiteTimeFmt)
	o.Status = "REJECTED"
	o.StatusMessage = reason
	o.StatusMessageRaw = reason
	o.ExchangeUpdateTimestamp = now
	e.history[o.OrderID] = append(e.history[o.OrderID], *o)
	e.mu.Unlock()

	for _, fn := range hooks {
		fn(o)
	}
	return nil
}

// CancelOrder transitions an OPEN order to CANCELLED.
func (e *OrderEngine) CancelOrder(orderID string) error {
	e.mu.Lock()
	o, ok := e.orders[orderID]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("order %s not found", orderID)
	}
	if o.Status != "OPEN" {
		e.mu.Unlock()
		return fmt.Errorf("order %s is %s, not OPEN", orderID, o.Status)
	}
	hooks := e.onTerminal

	now := time.Now().Format(kiteTimeFmt)
	o.Status = "CANCELLED"
	o.CancelledQuantity = o.Quantity
	o.PendingQuantity = 0
	o.ExchangeUpdateTimestamp = now
	e.history[o.OrderID] = append(e.history[o.OrderID], *o)
	e.mu.Unlock()

	for _, fn := range hooks {
		fn(o)
	}
	return nil
}

// GetOrders returns all orders for a user, sorted latest first.
func (e *OrderEngine) GetOrders(userID string) []*Order {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ids := e.userOrders[userID]
	out := make([]*Order, 0, len(ids))
	for _, id := range ids {
		if o := e.orders[id]; o != nil {
			out = append(out, o)
		}
	}
	sortOrdersLatestFirst(out)
	return out
}

// GetAllOrders returns all orders across all users, sorted latest first.
func (e *OrderEngine) GetAllOrders() []*Order {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]*Order, 0, len(e.orders))
	for _, o := range e.orders {
		out = append(out, o)
	}
	sortOrdersLatestFirst(out)
	return out
}

func sortOrdersLatestFirst(orders []*Order) {
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].OrderTimestamp != orders[j].OrderTimestamp {
			return orders[i].OrderTimestamp > orders[j].OrderTimestamp
		}
		return orders[i].OrderID > orders[j].OrderID
	})
}

// GetOrder returns a single order by ID.
func (e *OrderEngine) GetOrder(orderID string) (*Order, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	o, ok := e.orders[orderID]
	return o, ok
}

// GetOrderHistory returns the state transition history for an order.
func (e *OrderEngine) GetOrderHistory(orderID string) []Order {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.history[orderID]
}

// GetInstrument returns an instrument by exchange:symbol key.
func (e *OrderEngine) GetInstrument(exchange, symbol string) (*Instrument, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	inst, ok := e.instruments[exchange+":"+symbol]
	return inst, ok
}

// GetAllInstruments returns all configured instruments.
func (e *OrderEngine) GetAllInstruments() []Instrument {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Instrument, 0, len(e.instruments))
	for _, inst := range e.instruments {
		out = append(out, *inst)
	}
	return out
}

// UpdateLTP changes an instrument's last traded price at runtime.
func (e *OrderEngine) UpdateLTP(exchange, symbol string, ltp float64) error {
	key := exchange + ":" + symbol
	e.mu.Lock()
	defer e.mu.Unlock()
	inst, ok := e.instruments[key]
	if !ok {
		return fmt.Errorf("instrument %s not found", key)
	}
	inst.LTP = ltp
	return nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// Position matches the Kite Connect position JSON shape.
type Position struct {
	Tradingsymbol     string  `json:"tradingsymbol"`
	Exchange          string  `json:"exchange"`
	InstrumentToken   int     `json:"instrument_token"`
	Product           string  `json:"product"`
	Quantity          int     `json:"quantity"`
	OvernightQuantity int     `json:"overnight_quantity"`
	Multiplier        float64 `json:"multiplier"`
	AveragePrice      float64 `json:"average_price"`
	ClosePrice        float64 `json:"close_price"`
	LastPrice         float64 `json:"last_price"`
	Value             float64 `json:"value"`
	PnL               float64 `json:"pnl"`
	M2M               float64 `json:"m2m"`
	Unrealised        float64 `json:"unrealised"`
	Realised          float64 `json:"realised"`
	BuyQuantity       int     `json:"buy_quantity"`
	BuyPrice          float64 `json:"buy_price"`
	BuyValue          float64 `json:"buy_value"`
	BuyM2MValue       float64 `json:"buy_m2m"`
	SellQuantity      int     `json:"sell_quantity"`
	SellPrice         float64 `json:"sell_price"`
	SellValue         float64 `json:"sell_value"`
	SellM2MValue      float64 `json:"sell_m2m"`
	DayBuyQuantity    int     `json:"day_buy_quantity"`
	DayBuyPrice       float64 `json:"day_buy_price"`
	DayBuyValue       float64 `json:"day_buy_value"`
	DaySellQuantity   int     `json:"day_sell_quantity"`
	DaySellPrice      float64 `json:"day_sell_price"`
	DaySellValue      float64 `json:"day_sell_value"`
}

// GetPositions aggregates all completed orders for a user into net and day positions.
func (e *OrderEngine) GetPositions(userID string) (net []Position, day []Position) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	type accum struct {
		exchange string
		symbol   string
		product  string
		token    int
		buyQty   int
		buyVal   float64
		sellQty  int
		sellVal  float64
	}

	accums := make(map[string]*accum)
	var keys []string

	orderIDs := e.userOrders[userID]
	for _, oid := range orderIDs {
		o, ok := e.orders[oid]
		if !ok || o.Status != "COMPLETE" || o.FilledQuantity <= 0 {
			continue
		}

		key := o.Exchange + ":" + o.Tradingsymbol + ":" + o.Product
		a, exists := accums[key]
		if !exists {
			a = &accum{
				exchange: o.Exchange,
				symbol:   o.Tradingsymbol,
				product:  o.Product,
				token:    o.InstrumentToken,
			}
			accums[key] = a
			keys = append(keys, key)
		}

		val := float64(o.FilledQuantity) * o.AveragePrice
		if o.TransactionType == "BUY" || o.TransactionType == "B" {
			a.buyQty += o.FilledQuantity
			a.buyVal += val
		} else if o.TransactionType == "SELL" || o.TransactionType == "S" {
			a.sellQty += o.FilledQuantity
			a.sellVal += val
		}
	}

	net = make([]Position, 0, len(keys))
	day = make([]Position, 0, len(keys))

	for _, k := range keys {
		a := accums[k]
		netQty := a.buyQty - a.sellQty

		buyPrice := 0.0
		if a.buyQty > 0 {
			buyPrice = a.buyVal / float64(a.buyQty)
		}
		sellPrice := 0.0
		if a.sellQty > 0 {
			sellPrice = a.sellVal / float64(a.sellQty)
		}

		avgPrice := 0.0
		if netQty > 0 {
			avgPrice = buyPrice
		} else if netQty < 0 {
			avgPrice = sellPrice
		}

		ltp := avgPrice
		if inst, ok := e.instruments[a.exchange+":"+a.symbol]; ok && inst.LTP > 0 {
			ltp = inst.LTP
		}

		realised := 0.0
		unrealised := 0.0
		closedQty := a.buyQty
		if a.sellQty < closedQty {
			closedQty = a.sellQty
		}

		if closedQty > 0 && buyPrice > 0 && sellPrice > 0 {
			realised = float64(closedQty) * (sellPrice - buyPrice)
		}

		if netQty > 0 {
			unrealised = float64(netQty) * (ltp - buyPrice)
		} else if netQty < 0 {
			unrealised = float64(-netQty) * (sellPrice - ltp)
		}

		pnl := realised + unrealised
		m2m := pnl

		pos := Position{
			Tradingsymbol:   a.symbol,
			Exchange:        a.exchange,
			InstrumentToken: a.token,
			Product:         a.product,
			Quantity:        netQty,
			Multiplier:      1.0,
			AveragePrice:    avgPrice,
			LastPrice:       ltp,
			Value:           float64(netQty) * ltp,
			PnL:             pnl,
			M2M:             m2m,
			Unrealised:      unrealised,
			Realised:        realised,
			BuyQuantity:     a.buyQty,
			BuyPrice:        buyPrice,
			BuyValue:        a.buyVal,
			BuyM2MValue:     a.buyVal,
			SellQuantity:    a.sellQty,
			SellPrice:       sellPrice,
			SellValue:       a.sellVal,
			SellM2MValue:    a.sellVal,
			DayBuyQuantity:  a.buyQty,
			DayBuyPrice:     buyPrice,
			DayBuyValue:     a.buyVal,
			DaySellQuantity: a.sellQty,
			DaySellPrice:    sellPrice,
			DaySellValue:    a.sellVal,
		}

		net = append(net, pos)
		day = append(day, pos)
	}

	return net, day
}
