// Package domain holds pure types and rules for the copy-trade fan-out
// mechanism. Zero external dependencies beyond decimal — no store, no
// broker, no queue.
package domain

import "github.com/shopspring/decimal"

// SizingReason explains why SizeOrder returned the quantity it did. A
// follower that is skipped or capped must be visible, never silent, so
// every non-error outcome carries a reason worth recording alongside the
// follower_order row.
type SizingReason int

const (
	// ReasonOK: the computed quantity is exactly what was intended,
	// possibly after flooring to a lot multiple.
	ReasonOK SizingReason = iota
	// ReasonBelowOneLot: the ratio produced less than one full lot.
	// The follower is skipped for this signal, not upsized.
	ReasonBelowOneLot
	// ReasonCapped: the computed quantity exceeded max_qty_per_order
	// and was reduced to the largest lot multiple within the cap.
	ReasonCapped
	// ReasonBadInstrument: lot size is not a usable positive value.
	ReasonBadInstrument
	// ReasonInvalidRatio: capital_ratio is not a usable positive value.
	ReasonInvalidRatio
)

// SizeOrder computes the quantity a follower should place for a given
// master fill quantity, follower capital ratio, instrument lot size, and
// optional per-link cap. It never exceeds the master's intent: the result
// is always floored to a lot multiple, never rounded up.
//
// This is the highest-risk function in the system — a bug here places
// real money in the market — so it is pure, deterministic, and uses
// decimal arithmetic throughout to avoid float64 drift.
func SizeOrder(masterQty int, ratio decimal.Decimal, lotSize int, maxQty int) (int, SizingReason) {
	if lotSize <= 0 {
		return 0, ReasonBadInstrument
	}
	if ratio.Sign() <= 0 {
		return 0, ReasonInvalidRatio
	}

	lot := decimal.NewFromInt(int64(lotSize))
	raw := decimal.NewFromInt(int64(masterQty)).Mul(ratio)
	lots := raw.Div(lot).Floor().IntPart()
	qty := int(lots) * lotSize

	if maxQty > 0 && qty > maxQty {
		capped := maxQty - maxQty%lotSize
		if capped == 0 {
			return 0, ReasonBelowOneLot
		}
		return capped, ReasonCapped
	}

	if qty == 0 {
		return 0, ReasonBelowOneLot
	}
	return qty, ReasonOK
}
