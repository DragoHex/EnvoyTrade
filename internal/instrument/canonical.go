package instrument

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"envoytrade/internal/domain"

	"github.com/shopspring/decimal"
)

var (
	// Dhan Hyphenated Option: CRUDEOIL-17Nov2026-5000-PE, NIFTY-Nov2026-24000-CE, M&M-Nov2026-3000-CE
	reDhanOption = regexp.MustCompile(`^([A-Za-z0-9&_-]+)-(\d{1,2})?([A-Za-z]{3})(\d{4})-(\d+(?:\.\d+)?)-(CE|PE)$`)

	// Dhan Hyphenated Future: CRUDEOIL-21Sep2026-FUT, NIFTY-Nov2026-FUT, M&M-Nov2026-FUT
	reDhanFuture = regexp.MustCompile(`^([A-Za-z0-9&_-]+)-(\d{1,2})?([A-Za-z]{3})(\d{4})-FUT$`)

	// Kotak MCX Option with day: CRUDEOIL17NOV265000PE (DD: 01-31, YY: 20-39)
	reKotakMCXOption = regexp.MustCompile(`^([A-Z0-9&_-]+?)(0[1-9]|[12][0-9]|3[01])([A-Z]{3})(2[0-9]|3[0-9])(\d+(?:\.\d+)?)(CE|PE)$`)

	// Kite / Kotak Monthly Option without day: NIFTY26NOV24000CE, CRUDEOIL26NOV5000PE, M&M26NOV3000CE, NIFTYNXT5026NOV12000CE (YY: 20-39)
	reKiteMonthlyOption = regexp.MustCompile(`^([A-Z0-9&_-]+?)(2[0-9]|3[0-9])([A-Z]{3})(\d+(?:\.\d+)?)(CE|PE)$`)

	// Kotak MCX Future with day: CRUDEOIL21SEP26 or GOLDM05MAR27FUT (DD: 01-31, YY: 20-39)
	reKotakMCXFuture = regexp.MustCompile(`^([A-Z0-9&_-]+?)(0[1-9]|[12][0-9]|3[01])([A-Z]{3})(2[0-9]|3[0-9])(?:FUT)?$`)

	// NSE / BSE Weekly Option (Kite & Kotak): NIFTY26N0328350CE, SENSEX26D1772000PE, M&M26N033000CE (YY: 20-39, Month: 1-9/O/N/D, DD: 01-31)
	reNSEWeeklyOption = regexp.MustCompile(`^([A-Z0-9&_-]+?)(2[0-9]|3[0-9])([1-9OND])(0[1-9]|[12][0-9]|3[01])(\d+(?:\.\d+)?)(CE|PE)$`)

	// Kite Future: NIFTY26OCTFUT, M&M26OCTFUT (YY: 20-39)
	reKiteFuture = regexp.MustCompile(`^([A-Z0-9&_-]+?)(2[0-9]|3[0-9])([A-Z]{3})FUT$`)
)

var monthMap = map[string]time.Month{
	"JAN": time.January,
	"FEB": time.February,
	"MAR": time.March,
	"APR": time.April,
	"MAY": time.May,
	"JUN": time.June,
	"JUL": time.July,
	"AUG": time.August,
	"SEP": time.September,
	"OCT": time.October,
	"NOV": time.November,
	"DEC": time.December,
}

// CanonicalContract represents the broker-agnostic identity of a tradeable instrument.
type CanonicalContract struct {
	Exchange   string          // MCX, NFO, BFO, NSE, BSE
	Underlying string          // CRUDEOIL, GOLDM, NIFTY, RELIANCE
	ExpiryDate time.Time       // Date of expiry (exact day if known, or 1st of month)
	HasDay     bool            // True if specific day of month was parsed
	Strike     decimal.Decimal // Strike price for options (zero for futures/equities)
	OptionType string          // CE, PE, or empty
	IsOption   bool            // True if option contract
	IsFuture   bool            // True if futures contract
	IsEquity   bool            // True if cash equity
}

// CanonicalKey returns the primary canonical lookup string.
// Formats:
//   - Equity:        "NSE:RELIANCE:EQ"
//   - Option w/ day: "MCX:CRUDEOIL:2026-11-17:5000:PE"
//   - Option month:  "NFO:NIFTY:2026-11:24000:CE"
//   - Future w/ day: "MCX:CRUDEOIL:2026-09-21:FUT"
//   - Future month:  "NFO:NIFTY:2026-10:FUT"
func (c CanonicalContract) CanonicalKey() string {
	ex := strings.ToUpper(strings.TrimSpace(c.Exchange))
	und := strings.ToUpper(strings.TrimSpace(c.Underlying))

	if c.IsEquity {
		return fmt.Sprintf("%s:%s:EQ", ex, und)
	}

	dateStr := c.ExpiryDate.Format("2006-01")
	if c.HasDay {
		dateStr = c.ExpiryDate.Format("2006-01-02")
	}

	if c.IsOption {
		return fmt.Sprintf("%s:%s:%s:%s:%s", ex, und, dateStr, c.Strike.String(), c.OptionType)
	}

	if c.IsFuture {
		return fmt.Sprintf("%s:%s:%s:FUT", ex, und, dateStr)
	}

	return fmt.Sprintf("%s:%s", ex, und)
}

// LookupKeys returns an ordered slice of keys to probe in the universal index.
// If contract has an exact day, it returns [exactDateKey, monthKey].
func (c CanonicalContract) LookupKeys() []string {
	primary := c.CanonicalKey()
	keys := []string{primary}

	if c.HasDay && !c.IsEquity {
		// Add month-level fallback key
		ex := strings.ToUpper(strings.TrimSpace(c.Exchange))
		und := strings.ToUpper(strings.TrimSpace(c.Underlying))
		dateStr := c.ExpiryDate.Format("2006-01")

		var monthKey string
		if c.IsOption {
			monthKey = fmt.Sprintf("%s:%s:%s:%s:%s", ex, und, dateStr, c.Strike.String(), c.OptionType)
		} else if c.IsFuture {
			monthKey = fmt.Sprintf("%s:%s:%s:FUT", ex, und, dateStr)
		}
		if monthKey != "" && monthKey != primary {
			keys = append(keys, monthKey)
		}
	}

	return keys
}

// ToDhanSymbol generates the broker-specific trading symbol formatted for Dhan.
func (c CanonicalContract) ToDhanSymbol() string {
	if c.IsEquity {
		return c.Underlying
	}

	monthStr := c.ExpiryDate.Format("Jan") // e.g. Nov
	yearStr := c.ExpiryDate.Format("2006")  // e.g. 2026

	dayPrefix := ""
	if c.HasDay {
		dayPrefix = fmt.Sprintf("%02d", c.ExpiryDate.Day())
	}

	if c.IsOption {
		return fmt.Sprintf("%s-%s%s%s-%s-%s", c.Underlying, dayPrefix, monthStr, yearStr, c.Strike.String(), c.OptionType)
	}

	if c.IsFuture {
		return fmt.Sprintf("%s-%s%s%s-FUT", c.Underlying, dayPrefix, monthStr, yearStr)
	}

	return c.Underlying
}

// ToKotakSymbol generates the broker-specific trading symbol formatted for Kotak Neo.
func (c CanonicalContract) ToKotakSymbol() string {
	if c.IsEquity {
		return c.Underlying + "-EQ"
	}

	monthStr := strings.ToUpper(c.ExpiryDate.Format("Jan")) // e.g. NOV
	yyStr := c.ExpiryDate.Format("06")                      // e.g. 26

	if c.IsOption {
		if c.Exchange != "MCX" && c.HasDay {
			// Kotak NSE/BSE Weekly Option: NIFTY26N0328350CE, SENSEX26D1772000PE
			m := c.ExpiryDate.Month()
			var mCode string
			switch m {
			case 10:
				mCode = "O"
			case 11:
				mCode = "N"
			case 12:
				mCode = "D"
			default:
				mCode = strconv.Itoa(int(m))
			}
			return fmt.Sprintf("%s%s%s%02d%s%s", c.Underlying, yyStr, mCode, c.ExpiryDate.Day(), c.Strike.String(), c.OptionType)
		}
		if c.HasDay {
			// Kotak MCX Option: CRUDEOIL17NOV265000PE
			return fmt.Sprintf("%s%02d%s%s%s%s", c.Underlying, c.ExpiryDate.Day(), monthStr, yyStr, c.Strike.String(), c.OptionType)
		}
		return fmt.Sprintf("%s%s%s%s%s", c.Underlying, yyStr, monthStr, c.Strike.String(), c.OptionType)
	}

	if c.IsFuture {
		if c.HasDay {
			return fmt.Sprintf("%s%02d%s%s", c.Underlying, c.ExpiryDate.Day(), monthStr, yyStr)
		}
		return fmt.Sprintf("%s%s%sFUT", c.Underlying, yyStr, monthStr)
	}

	return c.Underlying
}

// ToKiteSymbol generates the broker-specific trading symbol formatted for Zerodha Kite.
func (c CanonicalContract) ToKiteSymbol() string {
	if c.IsEquity {
		return c.Underlying
	}

	monthStr := strings.ToUpper(c.ExpiryDate.Format("Jan")) // e.g. NOV
	yyStr := c.ExpiryDate.Format("06")                      // e.g. 26

	if c.IsOption {
		if c.Exchange != "MCX" && c.HasDay {
			// NFO / BFO weekly option: e.g. NIFTY26N1724000CE
			m := c.ExpiryDate.Month()
			var mCode string
			switch m {
			case 10:
				mCode = "O"
			case 11:
				mCode = "N"
			case 12:
				mCode = "D"
			default:
				mCode = strconv.Itoa(int(m))
			}
			return fmt.Sprintf("%s%s%s%02d%s%s", c.Underlying, yyStr, mCode, c.ExpiryDate.Day(), c.Strike.String(), c.OptionType)
		}
		// MCX options or monthly options: e.g. CRUDEOIL26NOV5000PE or NIFTY26NOV24000CE
		return fmt.Sprintf("%s%s%s%s%s", c.Underlying, yyStr, monthStr, c.Strike.String(), c.OptionType)
	}

	if c.IsFuture {
		return fmt.Sprintf("%s%s%sFUT", c.Underlying, yyStr, monthStr)
	}

	return c.Underlying
}


// ParseCanonicalContract decomposes any broker-specific trading symbol into a CanonicalContract.
func ParseCanonicalContract(exchange, tradingsymbol string) (CanonicalContract, error) {
	ex := strings.ToUpper(strings.TrimSpace(exchange))
	rawSym := strings.TrimSpace(tradingsymbol)
	sym := strings.ToUpper(rawSym)

	if ex == "" || sym == "" {
		return CanonicalContract{}, fmt.Errorf("%w: empty exchange or tradingsymbol", domain.ErrNotFound)
	}

	// 1. Dhan Hyphenated Option
	if m := reDhanOption.FindStringSubmatch(rawSym); len(m) == 7 {
		und := strings.ToUpper(m[1])
		dayStr := m[2]
		monStr := strings.ToUpper(m[3])
		yearStr := m[4]
		strikeStr := m[5]
		optType := strings.ToUpper(m[6])

		month, ok := monthMap[monStr]
		if !ok {
			return CanonicalContract{}, fmt.Errorf("%w: invalid month in dhan option %s", domain.ErrNotFound, rawSym)
		}

		year, err := strconv.Atoi(yearStr)
		if err != nil {
			return CanonicalContract{}, fmt.Errorf("%w: invalid year in dhan option %s", domain.ErrNotFound, rawSym)
		}

		strike, err := decimal.NewFromString(strikeStr)
		if err != nil {
			return CanonicalContract{}, fmt.Errorf("%w: invalid strike in dhan option %s", domain.ErrNotFound, rawSym)
		}

		day := 1
		hasDay := false
		if dayStr != "" {
			if d, err := strconv.Atoi(dayStr); err == nil && d > 0 && d <= 31 {
				day = d
				hasDay = true
			}
		}

		return CanonicalContract{
			Exchange:   canonicalExchange(ex),
			Underlying: und,
			ExpiryDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
			HasDay:     hasDay,
			Strike:     strike,
			OptionType: optType,
			IsOption:   true,
		}, nil
	}

	// 2. Dhan Hyphenated Future
	if m := reDhanFuture.FindStringSubmatch(rawSym); len(m) == 5 {
		und := strings.ToUpper(m[1])
		dayStr := m[2]
		monStr := strings.ToUpper(m[3])
		yearStr := m[4]

		month, ok := monthMap[monStr]
		if !ok {
			return CanonicalContract{}, fmt.Errorf("%w: invalid month in dhan future %s", domain.ErrNotFound, rawSym)
		}

		year, err := strconv.Atoi(yearStr)
		if err != nil {
			return CanonicalContract{}, fmt.Errorf("%w: invalid year in dhan future %s", domain.ErrNotFound, rawSym)
		}

		day := 1
		hasDay := false
		if dayStr != "" {
			if d, err := strconv.Atoi(dayStr); err == nil && d > 0 && d <= 31 {
				day = d
				hasDay = true
			}
		}

		return CanonicalContract{
			Exchange:   canonicalExchange(ex),
			Underlying: und,
			ExpiryDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
			HasDay:     hasDay,
			IsFuture:   true,
		}, nil
	}

	// 3. Compact Options
	if ex != "MCX" {
		// Weekly Index/Stock Option (Kite & Kotak): NIFTY26N0328350CE, SENSEX26D1772000PE
		if m := reNSEWeeklyOption.FindStringSubmatch(sym); len(m) == 7 {
			und := m[1]
			yy, _ := strconv.Atoi(m[2])
			monCode := m[3]
			day, _ := strconv.Atoi(m[4])
			strike, err := decimal.NewFromString(m[5])
			if err != nil {
				return CanonicalContract{}, fmt.Errorf("%w: invalid strike %s", domain.ErrNotFound, m[5])
			}
			optType := m[6]

			var month time.Month
			switch monCode {
			case "1":
				month = time.January
			case "2":
				month = time.February
			case "3":
				month = time.March
			case "4":
				month = time.April
			case "5":
				month = time.May
			case "6":
				month = time.June
			case "7":
				month = time.July
			case "8":
				month = time.August
			case "9":
				month = time.September
			case "O":
				month = time.October
			case "N":
				month = time.November
			case "D":
				month = time.December
			}

			year := 2000 + yy
			return CanonicalContract{
				Exchange:   canonicalExchange(ex),
				Underlying: und,
				ExpiryDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
				HasDay:     true,
				Strike:     strike,
				OptionType: optType,
				IsOption:   true,
			}, nil
		}
	}

	if ex == "MCX" {
		// Kotak MCX Option with day: CRUDEOIL17NOV265000PE
		if m := reKotakMCXOption.FindStringSubmatch(sym); len(m) == 7 {
			und := m[1]
			day, _ := strconv.Atoi(m[2])
			monStr := m[3]
			yy, _ := strconv.Atoi(m[4])
			strike, err := decimal.NewFromString(m[5])
			if err != nil {
				return CanonicalContract{}, fmt.Errorf("%w: invalid strike %s", domain.ErrNotFound, m[5])
			}
			optType := m[6]

			month, ok := monthMap[monStr]
			if ok {
				year := 2000 + yy
				return CanonicalContract{
					Exchange:   canonicalExchange(ex),
					Underlying: und,
					ExpiryDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
					HasDay:     true,
					Strike:     strike,
					OptionType: optType,
					IsOption:   true,
				}, nil
			}
		}
	}

	// 4. Monthly Option without day: NIFTY26NOV24000CE or CRUDEOIL26NOV5000PE
	if m := reKiteMonthlyOption.FindStringSubmatch(sym); len(m) == 6 {
		und := m[1]
		yy, _ := strconv.Atoi(m[2])
		monStr := m[3]
		strike, err := decimal.NewFromString(m[4])
		if err != nil {
			return CanonicalContract{}, fmt.Errorf("%w: invalid strike %s", domain.ErrNotFound, m[4])
		}
		optType := m[5]

		month, ok := monthMap[monStr]
		if ok {
			year := 2000 + yy
			return CanonicalContract{
				Exchange:   canonicalExchange(ex),
				Underlying: und,
				ExpiryDate: time.Date(year, month, 1, 0, 0, 0, 0, time.UTC),
				HasDay:     false,
				Strike:     strike,
				OptionType: optType,
				IsOption:   true,
			}, nil
		}
	}

	if ex != "MCX" {
		// Non-MCX Option with day (e.g. weekly): NIFTY17NOV2624000CE
		if m := reKotakMCXOption.FindStringSubmatch(sym); len(m) == 7 {
			und := m[1]
			day, _ := strconv.Atoi(m[2])
			monStr := m[3]
			yy, _ := strconv.Atoi(m[4])
			strike, err := decimal.NewFromString(m[5])
			if err != nil {
				return CanonicalContract{}, fmt.Errorf("%w: invalid strike %s", domain.ErrNotFound, m[5])
			}
			optType := m[6]

			month, ok := monthMap[monStr]
			if ok {
				year := 2000 + yy
				return CanonicalContract{
					Exchange:   canonicalExchange(ex),
					Underlying: und,
					ExpiryDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
					HasDay:     true,
					Strike:     strike,
					OptionType: optType,
					IsOption:   true,
				}, nil
			}
		}
	}

	// 5. Kotak MCX Future with day: CRUDEOIL21SEP26
	if m := reKotakMCXFuture.FindStringSubmatch(sym); len(m) == 5 {
		und := m[1]
		day, _ := strconv.Atoi(m[2])
		monStr := m[3]
		yy, _ := strconv.Atoi(m[4])

		month, ok := monthMap[monStr]
		if ok {
			year := 2000 + yy
			return CanonicalContract{
				Exchange:   canonicalExchange(ex),
				Underlying: und,
				ExpiryDate: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
				HasDay:     true,
				IsFuture:   true,
			}, nil
		}
	}

	// 6. Kite Future: NIFTY26OCTFUT
	if m := reKiteFuture.FindStringSubmatch(sym); len(m) == 4 {
		und := m[1]
		yy, _ := strconv.Atoi(m[2])
		monStr := m[3]

		month, ok := monthMap[monStr]
		if ok {
			year := 2000 + yy
			return CanonicalContract{
				Exchange:   canonicalExchange(ex),
				Underlying: und,
				ExpiryDate: time.Date(year, month, 1, 0, 0, 0, 0, time.UTC),
				HasDay:     false,
				IsFuture:   true,
			}, nil
		}
	}

	// 7. Cash Equities (NSE/BSE)
	if ex == "NSE" || ex == "BSE" {
		cleanSym := strings.TrimSuffix(sym, "-EQ")
		cleanSym = strings.TrimSuffix(cleanSym, "-BE")
		return CanonicalContract{
			Exchange:   ex,
			Underlying: cleanSym,
			IsEquity:   true,
		}, nil
	}

	return CanonicalContract{}, fmt.Errorf("%w: unrecognized instrument pattern %s:%s", domain.ErrNotFound, exchange, tradingsymbol)
}

func canonicalExchange(ex string) string {
	e := strings.ToUpper(strings.TrimSpace(ex))
	switch e {
	case "NSE_FO", "NSE_FNO":
		return "NFO"
	case "BSE_FO", "BSE_FNO":
		return "BFO"
	case "MCX_FO", "MCX_COMM":
		return "MCX"
	default:
		return e
	}
}
