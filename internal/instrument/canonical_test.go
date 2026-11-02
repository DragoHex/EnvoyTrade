package instrument_test

import (
	"testing"
	"time"

	"envoytrade/internal/instrument"

	"github.com/shopspring/decimal"
)

func TestParseCanonicalContract(t *testing.T) {
	tests := []struct {
		name          string
		exchange      string
		tradingsymbol string
		wantKey       string
		wantErr       bool
	}{
		// Kotak Neo MCX Options: <Root><DD><Mon><YY><Strike><Type>
		{
			name:          "Kotak MCX PE Option",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL17NOV265000PE",
			wantKey:       "MCX:CRUDEOIL:2026-11-17:5000:PE",
		},
		{
			name:          "Kotak MCX CE Option with high strike",
			exchange:      "MCX",
			tradingsymbol: "GOLDM27NOV26160000CE",
			wantKey:       "MCX:GOLDM:2026-11-27:160000:CE",
		},
		// Dhan MCX Options: <Root>-<DD><Mon><YYYY>-<Strike>-<Type>
		{
			name:          "Dhan MCX PE Option",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL-17Nov2026-5000-PE",
			wantKey:       "MCX:CRUDEOIL:2026-11-17:5000:PE",
		},
		{
			name:          "Dhan MCX CE Option",
			exchange:      "MCX",
			tradingsymbol: "GOLDM-27Nov2026-160000-CE",
			wantKey:       "MCX:GOLDM:2026-11-27:160000:CE",
		},
		// Kite MCX Options: <Root><YY><Mon><Strike><Type>
		{
			name:          "Kite MCX PE Option (monthly)",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL26NOV5000PE",
			wantKey:       "MCX:CRUDEOIL:2026-11:5000:PE", // Month-level canonical key
		},
		// NFO Monthly Options: <Root><YY><Mon><Strike><Type>
		{
			name:          "NFO Monthly Option Kite/Kotak",
			exchange:      "NFO",
			tradingsymbol: "NIFTY26NOV24000CE",
			wantKey:       "NFO:NIFTY:2026-11:24000:CE",
		},
		{
			name:          "NFO Monthly Option Dhan",
			exchange:      "NFO",
			tradingsymbol: "NIFTY-Nov2026-24000-CE",
			wantKey:       "NFO:NIFTY:2026-11:24000:CE",
		},
		// Kotak / Kite Futures: <Root><DD><Mon><YY> or <Root><YY><Mon>FUT
		{
			name:          "Kotak MCX Future with day",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL21SEP26",
			wantKey:       "MCX:CRUDEOIL:2026-09-21:FUT",
		},
		{
			name:          "Dhan MCX Future",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL-21Sep2026-FUT",
			wantKey:       "MCX:CRUDEOIL:2026-09-21:FUT",
		},
		{
			name:          "Kite NFO Future",
			exchange:      "NFO",
			tradingsymbol: "NIFTY26OCTFUT",
			wantKey:       "NFO:NIFTY:2026-10:FUT",
		},
		// NSE / BSE Weekly Options (Kite & Kotak): <Root><YY><1-9OND><DD><Strike><Type>
		{
			name:          "Kite/Kotak NFO Weekly Option (Nov)",
			exchange:      "NFO",
			tradingsymbol: "NIFTY26N0328350CE",
			wantKey:       "NFO:NIFTY:2026-11-03:28350:CE",
		},
		{
			name:          "Kite/Kotak NFO Weekly Option (Dec)",
			exchange:      "NFO",
			tradingsymbol: "NIFTY26D0121100CE",
			wantKey:       "NFO:NIFTY:2026-12-01:21100:CE",
		},
		{
			name:          "Kite/Kotak BFO Weekly Option",
			exchange:      "BFO",
			tradingsymbol: "SENSEX26D1772000PE",
			wantKey:       "BFO:SENSEX:2026-12-17:72000:PE",
		},
		// MCX Futures & Options with live broker patterns
		{
			name:          "Kotak MCX Future with FUT suffix",
			exchange:      "MCX",
			tradingsymbol: "GOLDM05MAR27FUT",
			wantKey:       "MCX:GOLDM:2027-03-05:FUT",
		},
		{
			name:          "Kotak MCX Natgas Option",
			exchange:      "MCX",
			tradingsymbol: "NATGASMINI20NOV26115CE",
			wantKey:       "MCX:NATGASMINI:2026-11-20:115:CE",
		},
		{
			name:          "Kotak MCX Silver Option",
			exchange:      "MCX",
			tradingsymbol: "SILVERM19FEB27155000CE",
			wantKey:       "MCX:SILVERM:2027-02-19:155000:CE",
		},
		{
			name:          "NFO Stock Option (Kotak/Kite)",
			exchange:      "NFO",
			tradingsymbol: "INFY26NOV1000CE",
			wantKey:       "NFO:INFY:2026-11:1000:CE",
		},
		// Cash Equities
		{
			name:          "NSE Plain Equity",
			exchange:      "NSE",
			tradingsymbol: "RELIANCE",
			wantKey:       "NSE:RELIANCE:EQ",
		},
		{
			name:          "NSE Equity with -EQ suffix (Kotak)",
			exchange:      "NSE",
			tradingsymbol: "RELIANCE-EQ",
			wantKey:       "NSE:RELIANCE:EQ",
		},
		{
			name:          "BSE Plain Equity",
			exchange:      "BSE",
			tradingsymbol: "INFY",
			wantKey:       "BSE:INFY:EQ",
		},
		// Invalid / Blank
		{
			name:          "Blank Symbol",
			exchange:      "NSE",
			tradingsymbol: "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract, err := instrument.ParseCanonicalContract(tt.exchange, tt.tradingsymbol)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (contract=%+v)", contract)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotKey := contract.CanonicalKey()
			if gotKey != tt.wantKey {
				t.Errorf("got canonical key %q, want %q", gotKey, tt.wantKey)
			}
		})
	}
}

func TestCanonicalContract_MatchKeys(t *testing.T) {
	// A contract with exact day: 2026-11-17
	c := instrument.CanonicalContract{
		Exchange:   "MCX",
		Underlying: "CRUDEOIL",
		ExpiryDate: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
		HasDay:     true,
		Strike:     decimal.NewFromInt(5000),
		OptionType: "PE",
		IsOption:   true,
	}

	keys := c.LookupKeys()
	// Should contain exact date key and month-level key for fallback
	// "MCX:CRUDEOIL:2026-11-17:5000:PE" and "MCX:CRUDEOIL:2026-11:5000:PE"
	if len(keys) < 2 {
		t.Fatalf("expected at least 2 lookup keys, got %v", keys)
	}
	if keys[0] != "MCX:CRUDEOIL:2026-11-17:5000:PE" {
		t.Errorf("keys[0] = %q, want exact date key", keys[0])
	}
	if keys[1] != "MCX:CRUDEOIL:2026-11:5000:PE" {
		t.Errorf("keys[1] = %q, want month key", keys[1])
	}
}

func TestCanonicalContract_ToDhanSymbol(t *testing.T) {
	tests := []struct {
		name          string
		exchange      string
		tradingsymbol string // input symbol from Kotak or Kite
		wantDhan      string // expected Dhan symbol
	}{
		{
			name:          "Kotak MCX Option to Dhan",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL17NOV265000PE",
			wantDhan:      "CRUDEOIL-17Nov2026-5000-PE",
		},
		{
			name:          "Kotak MCX Future to Dhan",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL21SEP26",
			wantDhan:      "CRUDEOIL-21Sep2026-FUT",
		},
		{
			name:          "Kotak/Kite NFO Option to Dhan",
			exchange:      "NFO",
			tradingsymbol: "NIFTY26NOV24000CE",
			wantDhan:      "NIFTY-Nov2026-24000-CE",
		},
		{
			name:          "Equity to Dhan",
			exchange:      "NSE",
			tradingsymbol: "RELIANCE-EQ",
			wantDhan:      "RELIANCE",
		},
		{
			name:          "Kotak/Kite Weekly NFO Option to Dhan",
			exchange:      "NFO",
			tradingsymbol: "NIFTY26N0328350CE",
			wantDhan:      "NIFTY-03Nov2026-28350-CE",
		},
		{
			name:          "Kotak/Kite Weekly BFO Option to Dhan",
			exchange:      "BFO",
			tradingsymbol: "SENSEX26D1772000PE",
			wantDhan:      "SENSEX-17Dec2026-72000-PE",
		},
		{
			name:          "Kotak MCX Future to Dhan",
			exchange:      "MCX",
			tradingsymbol: "GOLDM05MAR27FUT",
			wantDhan:      "GOLDM-05Mar2027-FUT",
		},
		{
			name:          "Kotak MCX Natgas Option to Dhan",
			exchange:      "MCX",
			tradingsymbol: "NATGASMINI20NOV26115CE",
			wantDhan:      "NATGASMINI-20Nov2026-115-CE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := instrument.ParseCanonicalContract(tt.exchange, tt.tradingsymbol)
			if err != nil {
				t.Fatalf("ParseCanonicalContract failed: %v", err)
			}
			got := c.ToDhanSymbol()
			if got != tt.wantDhan {
				t.Errorf("ToDhanSymbol = %q, want %q", got, tt.wantDhan)
			}
		})
	}
}

func TestCanonicalContract_ToKotakSymbol(t *testing.T) {
	tests := []struct {
		name          string
		exchange      string
		tradingsymbol string // input symbol from Dhan
		wantKotak     string // expected Kotak symbol
	}{
		{
			name:          "Dhan MCX Option to Kotak",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL-17Nov2026-5000-PE",
			wantKotak:     "CRUDEOIL17NOV265000PE",
		},
		{
			name:          "Dhan MCX Future to Kotak",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL-21Sep2026-FUT",
			wantKotak:     "CRUDEOIL21SEP26",
		},
		{
			name:          "Dhan NFO Option to Kotak",
			exchange:      "NFO",
			tradingsymbol: "NIFTY-Nov2026-24000-CE",
			wantKotak:     "NIFTY26NOV24000CE",
		},
		{
			name:          "Equity to Kotak",
			exchange:      "NSE",
			tradingsymbol: "RELIANCE",
			wantKotak:     "RELIANCE-EQ",
		},
		{
			name:          "Dhan Weekly NFO Option to Kotak",
			exchange:      "NFO",
			tradingsymbol: "NIFTY-03Nov2026-28350-CE",
			wantKotak:     "NIFTY26N0328350CE",
		},
		{
			name:          "Dhan Weekly BFO Option to Kotak",
			exchange:      "BFO",
			tradingsymbol: "SENSEX-17Dec2026-72000-PE",
			wantKotak:     "SENSEX26D1772000PE",
		},
		{
			name:          "Dhan MCX Future to Kotak",
			exchange:      "MCX",
			tradingsymbol: "GOLDM-05Mar2027-FUT",
			wantKotak:     "GOLDM05MAR27",
		},
		{
			name:          "Dhan MCX Natgas Option to Kotak",
			exchange:      "MCX",
			tradingsymbol: "NATGASMINI-20Nov2026-115-CE",
			wantKotak:     "NATGASMINI20NOV26115CE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := instrument.ParseCanonicalContract(tt.exchange, tt.tradingsymbol)
			if err != nil {
				t.Fatalf("ParseCanonicalContract failed: %v", err)
			}
			got := c.ToKotakSymbol()
			if got != tt.wantKotak {
				t.Errorf("ToKotakSymbol = %q, want %q", got, tt.wantKotak)
			}
		})
	}
}

func TestCanonicalContract_ToKiteSymbol(t *testing.T) {
	tests := []struct {
		name          string
		exchange      string
		tradingsymbol string // input symbol from Kotak or Dhan
		wantKite      string // expected Kite symbol
	}{
		{
			name:          "Kotak MCX Option to Kite",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL17NOV265000PE",
			wantKite:      "CRUDEOIL26NOV5000PE",
		},
		{
			name:          "Dhan MCX Option to Kite",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL-17Nov2026-5000-PE",
			wantKite:      "CRUDEOIL26NOV5000PE",
		},
		{
			name:          "Kotak MCX Future to Kite",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL21SEP26",
			wantKite:      "CRUDEOIL26SEPFUT",
		},
		{
			name:          "Dhan MCX Future to Kite",
			exchange:      "MCX",
			tradingsymbol: "CRUDEOIL-21Sep2026-FUT",
			wantKite:      "CRUDEOIL26SEPFUT",
		},
		{
			name:          "Dhan NFO Monthly Option to Kite",
			exchange:      "NFO",
			tradingsymbol: "NIFTY-Nov2026-24000-CE",
			wantKite:      "NIFTY26NOV24000CE",
		},
		{
			name:          "Equity with -EQ to Kite",
			exchange:      "NSE",
			tradingsymbol: "RELIANCE-EQ",
			wantKite:      "RELIANCE",
		},
		{
			name:          "Dhan Weekly NFO Option to Kite",
			exchange:      "NFO",
			tradingsymbol: "NIFTY-03Nov2026-28350-CE",
			wantKite:      "NIFTY26N0328350CE",
		},
		{
			name:          "Dhan Weekly BFO Option to Kite",
			exchange:      "BFO",
			tradingsymbol: "SENSEX-17Dec2026-72000-PE",
			wantKite:      "SENSEX26D1772000PE",
		},
		{
			name:          "Kotak MCX Natgas Option to Kite",
			exchange:      "MCX",
			tradingsymbol: "NATGASMINI20NOV26115CE",
			wantKite:      "NATGASMINI26NOV115CE",
		},
		{
			name:          "Kotak MCX Future to Kite",
			exchange:      "MCX",
			tradingsymbol: "GOLDM05MAR27FUT",
			wantKite:      "GOLDM27MARFUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := instrument.ParseCanonicalContract(tt.exchange, tt.tradingsymbol)
			if err != nil {
				t.Fatalf("ParseCanonicalContract failed: %v", err)
			}
			got := c.ToKiteSymbol()
			if got != tt.wantKite {
				t.Errorf("ToKiteSymbol = %q, want %q", got, tt.wantKite)
			}
		})
	}
}

