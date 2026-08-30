package domain_test

import (
	"testing"

	"envoytrade/internal/domain"

	"github.com/shopspring/decimal"
)

func ratio(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestSizeOrder(t *testing.T) {
	tests := []struct {
		name       string
		masterQty  int
		ratio      decimal.Decimal
		lotSize    int
		maxQty     int
		wantQty    int
		wantReason domain.SizingReason
	}{
		{
			name:       "exact multiple of lot size",
			masterQty:  100,
			ratio:      ratio("0.5"),
			lotSize:    10,
			maxQty:     0,
			wantQty:    50,
			wantReason: domain.ReasonOK,
		},
		{
			name:       "sub-lot ratio rounds down to zero",
			masterQty:  10,
			ratio:      ratio("0.05"),
			lotSize:    10,
			maxQty:     0,
			wantQty:    0,
			wantReason: domain.ReasonBelowOneLot,
		},
		{
			name:       "huge ratio capped, cap rounds down to lot multiple",
			masterQty:  1000,
			ratio:      ratio("2"),
			lotSize:    10,
			maxQty:     105,
			wantQty:    100,
			wantReason: domain.ReasonCapped,
		},
		{
			name:       "maxQty zero means no cap",
			masterQty:  1000,
			ratio:      ratio("2"),
			lotSize:    10,
			maxQty:     0,
			wantQty:    2000,
			wantReason: domain.ReasonOK,
		},
		{
			name:       "lot size zero is a bad instrument",
			masterQty:  100,
			ratio:      ratio("1"),
			lotSize:    0,
			maxQty:     0,
			wantQty:    0,
			wantReason: domain.ReasonBadInstrument,
		},
		{
			name:       "lot size negative is a bad instrument",
			masterQty:  100,
			ratio:      ratio("1"),
			lotSize:    -10,
			maxQty:     0,
			wantQty:    0,
			wantReason: domain.ReasonBadInstrument,
		},
		{
			name:       "master quantity zero yields below one lot",
			masterQty:  0,
			ratio:      ratio("1"),
			lotSize:    10,
			maxQty:     0,
			wantQty:    0,
			wantReason: domain.ReasonBelowOneLot,
		},
		{
			name:       "ratio zero is invalid, distinct from below-one-lot",
			masterQty:  100,
			ratio:      ratio("0"),
			lotSize:    10,
			maxQty:     0,
			wantQty:    0,
			wantReason: domain.ReasonInvalidRatio,
		},
		{
			name:       "negative ratio is invalid",
			masterQty:  100,
			ratio:      ratio("-0.5"),
			lotSize:    10,
			maxQty:     0,
			wantQty:    0,
			wantReason: domain.ReasonInvalidRatio,
		},
		{
			name:       "floored qty exactly equal to maxQty is OK, not capped",
			masterQty:  100,
			ratio:      ratio("1"),
			lotSize:    10,
			maxQty:     100,
			wantQty:    100,
			wantReason: domain.ReasonOK,
		},
		{
			name:       "maxQty not a multiple of lot size rounds cap down",
			masterQty:  1000,
			ratio:      ratio("1"),
			lotSize:    30,
			maxQty:     100,
			wantQty:    90,
			wantReason: domain.ReasonCapped,
		},
		{
			name:       "large master qty with fractional ratio keeps decimal precision",
			masterQty:  1_000_000,
			ratio:      ratio("0.0000013"),
			lotSize:    1,
			maxQty:     0,
			wantQty:    1,
			wantReason: domain.ReasonOK,
		},
		{
			name:       "cap below one lot after rounding is still below one lot, not capped",
			masterQty:  1000,
			ratio:      ratio("1"),
			lotSize:    10,
			maxQty:     5,
			wantQty:    0,
			wantReason: domain.ReasonBelowOneLot,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotQty, gotReason := domain.SizeOrder(tc.masterQty, tc.ratio, tc.lotSize, tc.maxQty)
			if gotQty != tc.wantQty {
				t.Errorf("qty = %d, want %d", gotQty, tc.wantQty)
			}
			if gotReason != tc.wantReason {
				t.Errorf("reason = %v, want %v", gotReason, tc.wantReason)
			}
		})
	}
}
