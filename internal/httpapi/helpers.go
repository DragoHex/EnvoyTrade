package httpapi

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// parseFlexibleDecimal converts a string, float64, float32, int, int64, json.Number, or nil into a decimal.Decimal.
// If val is nil or empty/whitespace string, it returns (decimal.Zero, false, nil).
// If val is an invalid number or unsupported type, it returns an error.
func parseFlexibleDecimal(val any) (decimal.Decimal, bool, error) {
	if val == nil {
		return decimal.Zero, false, nil
	}
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return decimal.Zero, false, nil
		}
		d, err := decimal.NewFromString(trimmed)
		if err != nil {
			return decimal.Zero, false, fmt.Errorf("invalid decimal %q: %w", trimmed, err)
		}
		return d, true, nil
	case float64:
		return decimal.NewFromFloat(v), true, nil
	case float32:
		return decimal.NewFromFloat32(v), true, nil
	case int:
		return decimal.NewFromInt(int64(v)), true, nil
	case int64:
		return decimal.NewFromInt(v), true, nil
	case json.Number:
		d, err := decimal.NewFromString(v.String())
		if err != nil {
			return decimal.Zero, false, fmt.Errorf("invalid json.Number %q: %w", v.String(), err)
		}
		return d, true, nil
	case decimal.Decimal:
		return v, true, nil
	default:
		return decimal.Zero, false, fmt.Errorf("unexpected value type %T", val)
	}
}

// parseFlexibleInt converts a string, float64, float32, int, int64, json.Number, or nil into an *int.
// If val is nil or empty/whitespace string, it returns (nil, nil).
// If val is an invalid number or unsupported type, it returns an error.
func parseFlexibleInt(val any) (*int, error) {
	if val == nil {
		return nil, nil
	}
	switch v := val.(type) {
	case float64:
		i := int(v)
		return &i, nil
	case float32:
		i := int(v)
		return &i, nil
	case int:
		return &v, nil
	case int64:
		i := int(v)
		return &i, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, nil
		}
		i, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", trimmed, err)
		}
		return &i, nil
	case json.Number:
		i64, err := v.Int64()
		if err != nil {
			return nil, fmt.Errorf("invalid json.Number integer %q: %w", v.String(), err)
		}
		i := int(i64)
		return &i, nil
	default:
		return nil, fmt.Errorf("unexpected value type %T", val)
	}
}
