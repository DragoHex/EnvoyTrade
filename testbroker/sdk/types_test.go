package sdk

import (
	"encoding/json"
	"testing"
)

func TestTypes_OrderJSON(t *testing.T) {
	rawJSON := `{
		"order_id": "260905000000001",
		"exchange_order_id": "1100000000000001",
		"parent_order_id": "",
		"user_id": "AB1234",
		"status": "COMPLETE",
		"status_message": "",
		"status_message_raw": "",
		"order_timestamp": "2026-09-05 10:30:00",
		"exchange_update_timestamp": "2026-09-05 10:30:00",
		"exchange_timestamp": "2026-09-05 10:30:00",
		"variety": "regular",
		"modified": false,
		"exchange": "NFO",
		"tradingsymbol": "NIFTY26OCTFUT",
		"instrument_token": 408065,
		"order_type": "MARKET",
		"transaction_type": "BUY",
		"validity": "DAY",
		"validity_ttl": 0,
		"product": "NRML",
		"quantity": 75,
		"disclosed_quantity": 0,
		"price": 25000.0,
		"trigger_price": 0.0,
		"average_price": 25000.5,
		"filled_quantity": 75,
		"pending_quantity": 0,
		"cancelled_quantity": 0,
		"market_protection": 0,
		"tag": "et:abc12345",
		"guid": "",
		"checksum": "mockchecksum123"
	}`

	var o Order
	if err := json.Unmarshal([]byte(rawJSON), &o); err != nil {
		t.Fatalf("Unmarshal Order failed: %v", err)
	}

	if o.OrderID != "260905000000001" {
		t.Errorf("OrderID = %s, want 260905000000001", o.OrderID)
	}
	if o.Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("Tradingsymbol = %s, want NIFTY26OCTFUT", o.Tradingsymbol)
	}
	if o.Quantity != 75 {
		t.Errorf("Quantity = %d, want 75", o.Quantity)
	}
	if o.AveragePrice != 25000.5 {
		t.Errorf("AveragePrice = %f, want 25000.5", o.AveragePrice)
	}
	if o.Status != OrderStatusComplete {
		t.Errorf("Status = %s, want %s", o.Status, OrderStatusComplete)
	}
	if o.Checksum != "mockchecksum123" {
		t.Errorf("Checksum = %s, want mockchecksum123", o.Checksum)
	}

	// Test roundtrip marshaling
	bytes, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("Marshal Order failed: %v", err)
	}
	var o2 Order
	if err := json.Unmarshal(bytes, &o2); err != nil {
		t.Fatalf("Unmarshal roundtrip failed: %v", err)
	}
	if o2.OrderID != o.OrderID || o2.Tradingsymbol != o.Tradingsymbol {
		t.Errorf("Roundtrip mismatch: %+v vs %+v", o, o2)
	}
}

func TestTypes_OrderParams(t *testing.T) {
	p := OrderParams{
		Exchange:        "NFO",
		Tradingsymbol:   "NIFTY26OCTFUT",
		TransactionType: TransactionTypeBuy,
		Product:         ProductNRML,
		OrderType:       OrderTypeMarket,
		Quantity:        150,
		Price:           25000.0,
		TriggerPrice:    0,
		Tag:             "tag-1",
		Validity:        "DAY",
	}

	bytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal OrderParams failed: %v", err)
	}

	var p2 OrderParams
	if err := json.Unmarshal(bytes, &p2); err != nil {
		t.Fatalf("Unmarshal OrderParams failed: %v", err)
	}
	if p2.Tradingsymbol != "NIFTY26OCTFUT" || p2.Quantity != 150 {
		t.Errorf("OrderParams mismatch: %+v", p2)
	}
}

func TestTypes_Positions(t *testing.T) {
	rawJSON := `{
		"net": [
			{
				"tradingsymbol": "NIFTY26OCTFUT",
				"exchange": "NFO",
				"instrument_token": 408065,
				"product": "NRML",
				"quantity": 75,
				"average_price": 25000.0,
				"last_price": 25050.0,
				"pnl": 3750.0,
				"buy_quantity": 75,
				"buy_price": 25000.0,
				"sell_quantity": 0,
				"sell_price": 0.0
			}
		],
		"day": []
	}`

	var pos Positions
	if err := json.Unmarshal([]byte(rawJSON), &pos); err != nil {
		t.Fatalf("Unmarshal Positions failed: %v", err)
	}
	if len(pos.Net) != 1 {
		t.Fatalf("Expected 1 net position, got %d", len(pos.Net))
	}
	if pos.Net[0].Tradingsymbol != "NIFTY26OCTFUT" || pos.Net[0].PnL != 3750.0 {
		t.Errorf("Unexpected position: %+v", pos.Net[0])
	}
	if len(pos.Day) != 0 {
		t.Errorf("Expected 0 day positions, got %d", len(pos.Day))
	}
}

func TestTypes_AllMargins(t *testing.T) {
	rawJSON := `{
		"equity": {
			"enabled": true,
			"net": 1000000.0,
			"available": {
				"cash": 1000000.0,
				"live_balance": 1000000.0
			},
			"utilised": {}
		},
		"commodity": {
			"enabled": true,
			"net": 500000.0,
			"available": {
				"cash": 500000.0,
				"live_balance": 500000.0
			},
			"utilised": {}
		}
	}`

	var margins AllMargins
	if err := json.Unmarshal([]byte(rawJSON), &margins); err != nil {
		t.Fatalf("Unmarshal AllMargins failed: %v", err)
	}
	if !margins.Equity.Enabled || margins.Equity.Available.LiveBalance != 1000000.0 {
		t.Errorf("Unexpected equity margin: %+v", margins.Equity)
	}
	if margins.Commodity.Net != 500000.0 {
		t.Errorf("Unexpected commodity margin: %+v", margins.Commodity)
	}
}

func TestTypes_KiteError(t *testing.T) {
	ke := &KiteError{
		StatusCode: 403,
		ErrorType:  "TokenException",
		Message:    "Invalid or missing token",
	}

	expected := "KiteError [403 TokenException]: Invalid or missing token"
	if ke.Error() != expected {
		t.Errorf("Error() = %q, want %q", ke.Error(), expected)
	}
}
