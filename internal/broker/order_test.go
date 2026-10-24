package broker_test

import (
	"encoding/json"
	"testing"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestOrder_ToMasterFill(t *testing.T) {
	masterID := uuid.New()
	now := time.Now().Truncate(time.Millisecond)

	order := broker.Order{
		OrderID:         "ORD12345",
		Exchange:        "NSE",
		Tradingsymbol:   "RELIANCE",
		TransactionType: "BUY",
		Product:         "CNC",
		OrderType:       "MARKET",
		Status:          domain.TerminalComplete,
		StatusMessage:   "Order executed",
		Quantity:        50,
		FilledQuantity:  50,
		Price:           decimal.NewFromFloat(2500.50),
		TriggerPrice:    decimal.Zero,
		AveragePrice:    decimal.NewFromFloat(2500.50),
		OrderTimestamp:  now,
		Tag:             "tag-123",
		RawPayload:      json.RawMessage(`{"broker":"test"}`),
	}

	fill := order.ToMasterFill(masterID)

	if fill.MasterID != masterID {
		t.Errorf("MasterID = %v, want %v", fill.MasterID, masterID)
	}
	if fill.BrokerOrderID != "ORD12345" {
		t.Errorf("BrokerOrderID = %v, want ORD12345", fill.BrokerOrderID)
	}
	if fill.Exchange != "NSE" || fill.Tradingsymbol != "RELIANCE" {
		t.Errorf("Instrument mismatch: %s:%s", fill.Exchange, fill.Tradingsymbol)
	}
	if fill.TransactionType != "BUY" || fill.Product != "CNC" || fill.OrderType != "MARKET" {
		t.Errorf("Order parameters mismatch: %s %s %s", fill.TransactionType, fill.Product, fill.OrderType)
	}
	if fill.FilledQuantity != 50 {
		t.Errorf("FilledQuantity = %d, want 50", fill.FilledQuantity)
	}
	if !fill.Price.Equal(decimal.NewFromFloat(2500.50)) {
		t.Errorf("Price = %v, want 2500.50", fill.Price)
	}
	if fill.Status != domain.TerminalComplete {
		t.Errorf("Status = %v, want %s", fill.Status, domain.TerminalComplete)
	}
	if !fill.OrderTimestamp.Equal(now) {
		t.Errorf("OrderTimestamp = %v, want %v", fill.OrderTimestamp, now)
	}
	if fill.Tag != "tag-123" {
		t.Errorf("Tag = %v, want tag-123", fill.Tag)
	}
	if string(fill.RawPayload) != `{"broker":"test"}` {
		t.Errorf("RawPayload = %s, want {\"broker\":\"test\"}", string(fill.RawPayload))
	}
}

func TestOrder_ToMasterFill_LimitFallback(t *testing.T) {
	masterID := uuid.New()
	order := broker.Order{
		OrderID:         "ORD_LIMIT",
		OrderType:       "LIMIT",
		Price:           decimal.Zero,
		AveragePrice:    decimal.NewFromFloat(150.25),
		FilledQuantity:  10,
		Status:          domain.TerminalComplete,
	}

	fill := order.ToMasterFill(masterID)
	if !fill.Price.Equal(decimal.NewFromFloat(150.25)) {
		t.Errorf("Price limit fallback = %v, want 150.25", fill.Price)
	}
}

func TestOrder_ToMasterFill_NilRawPayload(t *testing.T) {
	masterID := uuid.New()
	order := broker.Order{
		OrderID:        "ORD_NOPAYLOAD",
		FilledQuantity: 10,
		Status:         domain.TerminalComplete,
	}

	fill := order.ToMasterFill(masterID)
	if len(fill.RawPayload) == 0 {
		t.Fatalf("expected non-empty marshaled RawPayload when input is nil")
	}
	var decoded map[string]any
	if err := json.Unmarshal(fill.RawPayload, &decoded); err != nil {
		t.Fatalf("failed to unmarshal generated RawPayload: %v", err)
	}
	if decoded["order_id"] != "ORD_NOPAYLOAD" {
		t.Errorf("decoded order_id = %v, want ORD_NOPAYLOAD", decoded["order_id"])
	}
}

func TestOrder_ToOrderUpdate(t *testing.T) {
	followerID := uuid.New()
	now := time.Now().Truncate(time.Millisecond)

	order := broker.Order{
		OrderID:         "FOLLOWER_ORD_1",
		Tradingsymbol:   "TCS",
		Exchange:        "BSE",
		Product:         "MIS",
		TransactionType: "SELL",
		OrderType:       "LIMIT",
		Quantity:        100,
		FilledQuantity:  100,
		AveragePrice:    decimal.NewFromFloat(3800.00),
		Status:          domain.TerminalComplete,
		OrderTimestamp:  now,
		Tag:             "tag-fol",
	}

	upd := order.ToOrderUpdate(followerID)

	if upd.FollowerID != followerID {
		t.Errorf("FollowerID = %v, want %v", upd.FollowerID, followerID)
	}
	if upd.BrokerOrderID != "FOLLOWER_ORD_1" {
		t.Errorf("BrokerOrderID = %v, want FOLLOWER_ORD_1", upd.BrokerOrderID)
	}
	if upd.Tradingsymbol != "TCS" || upd.Exchange != "BSE" {
		t.Errorf("Instrument = %s:%s", upd.Exchange, upd.Tradingsymbol)
	}
	if upd.Quantity != 100 || upd.FilledQuantity != 100 {
		t.Errorf("Quantities = %d / %d, want 100 / 100", upd.Quantity, upd.FilledQuantity)
	}
	if !upd.AveragePrice.Equal(decimal.NewFromFloat(3800.00)) {
		t.Errorf("AveragePrice = %v, want 3800.00", upd.AveragePrice)
	}
	if upd.Status != domain.TerminalComplete {
		t.Errorf("Status = %v, want %s", upd.Status, domain.TerminalComplete)
	}
	if !upd.OrderTimestamp.Equal(now) {
		t.Errorf("OrderTimestamp = %v, want %v", upd.OrderTimestamp, now)
	}
	if upd.Tag != "tag-fol" {
		t.Errorf("Tag = %v, want tag-fol", upd.Tag)
	}
}
