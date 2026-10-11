package domain_test

import (
	"encoding/json"
	"testing"

	"envoytrade/internal/domain"

	"github.com/shopspring/decimal"
)

func TestSegmentBreakdown_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name          string
		inputJSON     string
		wantEquity    decimal.Decimal
		wantCommodity decimal.Decimal
	}{
		{
			name:          "standard lowercase keys",
			inputJSON:     `{"equity":"1500000.50","commodity":"663520.25"}`,
			wantEquity:    decimal.NewFromFloat(1500000.50),
			wantCommodity: decimal.NewFromFloat(663520.25),
		},
		{
			name:          "legacy uppercase keys",
			inputJSON:     `{"EQUITY":"1500000.50","COMMODITY":"663520.25"}`,
			wantEquity:    decimal.NewFromFloat(1500000.50),
			wantCommodity: decimal.NewFromFloat(663520.25),
		},
		{
			name:          "empty json object",
			inputJSON:     `{}`,
			wantEquity:    decimal.Zero,
			wantCommodity: decimal.Zero,
		},
		{
			name:          "null values",
			inputJSON:     `{"equity":null,"commodity":null}`,
			wantEquity:    decimal.Zero,
			wantCommodity: decimal.Zero,
		},
		{
			name:          "numeric json values",
			inputJSON:     `{"equity":1500000.5,"commodity":663520.25}`,
			wantEquity:    decimal.NewFromFloat(1500000.5),
			wantCommodity: decimal.NewFromFloat(663520.25),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb domain.SegmentBreakdown
			if err := json.Unmarshal([]byte(tt.inputJSON), &sb); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if !sb.Equity.Equal(tt.wantEquity) {
				t.Errorf("Equity = %v, want %v", sb.Equity, tt.wantEquity)
			}
			if !sb.Commodity.Equal(tt.wantCommodity) {
				t.Errorf("Commodity = %v, want %v", sb.Commodity, tt.wantCommodity)
			}
		})
	}
}

func TestSegmentBreakdown_MarshalJSON(t *testing.T) {
	sb := domain.SegmentBreakdown{
		Equity:    decimal.NewFromFloat(1500000),
		Commodity: decimal.NewFromFloat(663520),
	}
	data, err := json.Marshal(sb)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal map error: %v", err)
	}
	if parsed["equity"] != "1500000" {
		t.Errorf("equity = %q, want 1500000", parsed["equity"])
	}
	if parsed["commodity"] != "663520" {
		t.Errorf("commodity = %q, want 663520", parsed["commodity"])
	}
}

func TestSegmentBreakdown_IsZero(t *testing.T) {
	var zero domain.SegmentBreakdown
	if !zero.IsZero() {
		t.Errorf("expected zero.IsZero() to be true")
	}

	nonZero := domain.SegmentBreakdown{Equity: decimal.NewFromInt(100)}
	if nonZero.IsZero() {
		t.Errorf("expected nonZero.IsZero() to be false")
	}
}
