package callback_test

import (
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
	"github.com/zerodha/gokiteconnect/v4/models"
)

func sampleOrder() kiteconnect.Order {
	ts := time.Date(2026, 9, 5, 10, 30, 0, 0, time.UTC)
	return kiteconnect.Order{
		OrderID:         "231000000123456",
		Exchange:        "NSE",
		TradingSymbol:   "INFY",
		InstrumentToken: 408065,
		TransactionType: "BUY",
		Product:         "MIS",
		OrderType:       "MARKET",
		Status:          "COMPLETE",
		FilledQuantity:  100,
		AveragePrice:    1500.25,
		OrderTimestamp:  models.Time{Time: ts},
	}
}

func TestToMasterFill_MapsAllFields(t *testing.T) {
	masterID := uuid.New()
	got := callback.ToMasterFill(sampleOrder(), masterID)

	if got.MasterID != masterID {
		t.Errorf("MasterID = %v, want %v", got.MasterID, masterID)
	}
	if got.BrokerOrderID != "231000000123456" {
		t.Errorf("BrokerOrderID = %q", got.BrokerOrderID)
	}
	if got.Exchange != "NSE" || got.Tradingsymbol != "INFY" {
		t.Errorf("Exchange/Tradingsymbol = %q/%q", got.Exchange, got.Tradingsymbol)
	}
	if got.InstrumentToken != 408065 {
		t.Errorf("InstrumentToken = %d, want 408065", got.InstrumentToken)
	}
	if got.TransactionType != "BUY" || got.Product != "MIS" || got.OrderType != "MARKET" {
		t.Errorf("TransactionType/Product/OrderType = %q/%q/%q", got.TransactionType, got.Product, got.OrderType)
	}
	if got.FilledQuantity != 100 {
		t.Errorf("FilledQuantity = %d, want 100", got.FilledQuantity)
	}
	if !got.AveragePrice.Equal(decimal.NewFromFloat(1500.25)) {
		t.Errorf("AveragePrice = %v, want 1500.25", got.AveragePrice)
	}
	if got.Status != "COMPLETE" {
		t.Errorf("Status = %q, want COMPLETE", got.Status)
	}
	if !got.OrderTimestamp.Equal(time.Date(2026, 9, 5, 10, 30, 0, 0, time.UTC)) {
		t.Errorf("OrderTimestamp = %v", got.OrderTimestamp)
	}
	if len(got.RawPayload) == 0 {
		t.Error("RawPayload is empty, want the raw order JSON")
	}
}

func TestToMasterFill_TruncatesFractionalQuantity(t *testing.T) {
	o := sampleOrder()
	o.FilledQuantity = 99.9 // Kite never sends this for equities, but guard the conversion anyway.
	got := callback.ToMasterFill(o, uuid.New())
	if got.FilledQuantity != 99 {
		t.Errorf("FilledQuantity = %d, want 99 (truncated)", got.FilledQuantity)
	}
}

func TestToOrderUpdate_MapsStatusFields(t *testing.T) {
	got := callback.ToOrderUpdate(sampleOrder())

	if got.BrokerOrderID != "231000000123456" {
		t.Errorf("BrokerOrderID = %q", got.BrokerOrderID)
	}
	if got.Status != "COMPLETE" {
		t.Errorf("Status = %q, want COMPLETE", got.Status)
	}
	if got.FilledQuantity != 100 {
		t.Errorf("FilledQuantity = %d, want 100", got.FilledQuantity)
	}
	if !got.AveragePrice.Equal(decimal.NewFromFloat(1500.25)) {
		t.Errorf("AveragePrice = %v, want 1500.25", got.AveragePrice)
	}
	if len(got.RawPayload) == 0 {
		t.Error("RawPayload is empty, want the raw order JSON")
	}
}

func TestIsTerminal(t *testing.T) {
	cases := map[string]bool{
		domain.TerminalComplete:     true,
		domain.TerminalRejected:     true,
		domain.TerminalCancelled:    true,
		"OPEN":                      false,
		"TRIGGER PENDING":           false,
		"PUT ORDER REQ RECEIVED":    false,
		"MODIFY_VALIDATION_PENDING": false,
		"":                          false,
	}
	for status, want := range cases {
		if got := callback.IsTerminal(status); got != want {
			t.Errorf("IsTerminal(%q) = %v, want %v", status, got, want)
		}
	}
}
