package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseFlexibleDecimal(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantVal   decimal.Decimal
		wantHas   bool
		wantErr   bool
	}{
		{name: "nil", input: nil, wantVal: decimal.Zero, wantHas: false, wantErr: false},
		{name: "empty string", input: "", wantVal: decimal.Zero, wantHas: false, wantErr: false},
		{name: "whitespace string", input: "   ", wantVal: decimal.Zero, wantHas: false, wantErr: false},
		{name: "valid string int", input: "1", wantVal: decimal.RequireFromString("1"), wantHas: true, wantErr: false},
		{name: "valid string float", input: "1.5", wantVal: decimal.RequireFromString("1.5"), wantHas: true, wantErr: false},
		{name: "invalid string", input: "not-a-number", wantVal: decimal.Zero, wantHas: false, wantErr: true},
		{name: "float64", input: float64(1.5), wantVal: decimal.NewFromFloat(1.5), wantHas: true, wantErr: false},
		{name: "int", input: int(2), wantVal: decimal.NewFromInt(2), wantHas: true, wantErr: false},
		{name: "int64", input: int64(3), wantVal: decimal.NewFromInt(3), wantHas: true, wantErr: false},
		{name: "json.Number", input: json.Number("0.75"), wantVal: decimal.RequireFromString("0.75"), wantHas: true, wantErr: false},
		{name: "decimal.Decimal", input: decimal.RequireFromString("2.5"), wantVal: decimal.RequireFromString("2.5"), wantHas: true, wantErr: false},
		{name: "unsupported boolean", input: true, wantVal: decimal.Zero, wantHas: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotHas, err := parseFlexibleDecimal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFlexibleDecimal(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if gotHas != tt.wantHas {
				t.Errorf("parseFlexibleDecimal(%v) hasVal = %v, want %v", tt.input, gotHas, tt.wantHas)
			}
			if !gotVal.Equal(tt.wantVal) {
				t.Errorf("parseFlexibleDecimal(%v) val = %v, want %v", tt.input, gotVal, tt.wantVal)
			}
		})
	}
}

func TestParseFlexibleInt(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name    string
		input   any
		want    *int
		wantErr bool
	}{
		{name: "nil", input: nil, want: nil, wantErr: false},
		{name: "empty string", input: "", want: nil, wantErr: false},
		{name: "whitespace string", input: "   ", want: nil, wantErr: false},
		{name: "valid string", input: "50", want: intPtr(50), wantErr: false},
		{name: "invalid string", input: "abc", want: nil, wantErr: true},
		{name: "float64", input: float64(100), want: intPtr(100), wantErr: false},
		{name: "int", input: int(25), want: intPtr(25), wantErr: false},
		{name: "int64", input: int64(75), want: intPtr(75), wantErr: false},
		{name: "json.Number", input: json.Number("200"), want: intPtr(200), wantErr: false},
		{name: "unsupported slice", input: []int{1}, want: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlexibleInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFlexibleInt(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseFlexibleInt(%v) = %v, want nil", tt.input, *got)
				}
			} else {
				if got == nil || *got != *tt.want {
					t.Errorf("parseFlexibleInt(%v) = %v, want %v", tt.input, got, *tt.want)
				}
			}
		})
	}
}
