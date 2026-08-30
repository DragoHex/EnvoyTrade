package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DispatchState tracks a master fill through the transactional outbox
// (PLAN.md §4.1). The channel between listener and engine is a latency
// optimisation only — this column is the actual source of truth.
type DispatchState string

const (
	DispatchPending    DispatchState = "pending"
	DispatchDispatched DispatchState = "dispatched"
	DispatchDead       DispatchState = "dead"
)

// MasterFill is the signal: a completed (or otherwise terminal) order on
// the master account, captured verbatim from the broker's order-update
// feed. It is broker-agnostic — BrokerOrderID and RawPayload hold whatever
// shape the adapter's native order representation has.
type MasterFill struct {
	ID              int64
	MasterID        uuid.UUID
	BrokerOrderID   string
	Exchange        string
	Tradingsymbol   string
	InstrumentToken int64
	TransactionType string
	Product         string
	OrderType       string
	FilledQuantity  int
	AveragePrice    decimal.Decimal
	Status         string
	OrderTimestamp time.Time
	RawPayload     []byte
	ReceivedAt     time.Time
	DispatchState  DispatchState
	DispatchedAt   *time.Time
}

// FollowLink is the 1-master-per-follower relationship: a follower has at
// most one master, structurally enforced wherever this is persisted
// (follower_id as primary key — PLAN.md §2).
type FollowLink struct {
	FollowerID     uuid.UUID
	MasterID       uuid.UUID
	CapitalRatio   decimal.Decimal
	MaxQtyPerOrder int
	Enabled        bool
	EffectiveFrom  time.Time
}

// TerminalStatus values a follower_order can settle into. Broker-native
// terminal states (COMPLETE, REJECTED, CANCELLED) pass through verbatim
// from the broker; DeadLettered is EnvoyTrade's own outcome for a signal
// that never reached the broker at all (PLAN.md §4.3).
const (
	TerminalComplete     = "COMPLETE"
	TerminalRejected     = "REJECTED"
	TerminalCancelled    = "CANCELLED"
	TerminalDeadLettered = "DEAD_LETTERED"
)

// FollowerOrder is one row per (master_fill, follower) pair, created
// before any broker call is attempted so an idempotency tag exists prior
// to the order ever going live.
type FollowerOrder struct {
	ID             int64
	MasterFillID   int64
	FollowerID     uuid.UUID
	IdempotencyTag string
	IntendedQty    int
	LotSize        int
	SizingReason   SizingReason
	PlacedQty      *int
	BrokerOrderID  string
	TerminalStatus string
	FilledQty      int
	AveragePrice   decimal.Decimal
	AttemptCount   int
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ErrDuplicate is returned by a store when an insert collides with a
// unique constraint — the DB-layer half of the idempotency guarantee in
// PLAN.md §4.1 (a duplicated dispatch collides on a unique index instead
// of double-placing). It lives in domain, not a specific store package,
// so engine can recognize it without depending on a concrete store.
var ErrDuplicate = errors.New("domain: duplicate")

// ErrNotFound is returned by a store when a lookup has no matching row —
// e.g. an instrument the daily sync job hasn't (yet) seen. It lives in
// domain, not a specific store package, so engine can recognize it
// without depending on a concrete store's error types.
var ErrNotFound = errors.New("domain: not found")

// Instrument is one row of the instrument master (PLAN.md §2): the
// canonical lot size, tick size, and contract metadata for a tradable
// symbol, refreshed daily from the broker. Fan-out resolves LotSize from
// here rather than trusting a value carried by the signal itself — F&O
// lot sizes vary per contract and change over a contract's life, so a
// stale or spoofed value on the fill payload must never drive sizing.
type Instrument struct {
	InstrumentToken int64
	Exchange        string
	Tradingsymbol   string
	LotSize         int
	TickSize        decimal.Decimal
	Segment         string
	Expiry          *time.Time
	RefreshedAt     time.Time
}

// Job is a fully-sized, tagged instruction to place one follower order.
// It is the hand-off between engine (which decides what to place) and
// worker (which places it) — a plain data type so neither package needs
// to import the other (PLAN.md §1's dependency rule).
type Job struct {
	FollowerOrderID int64
	FollowerID      uuid.UUID
	MasterFillID    int64
	IdempotencyTag  string
	Exchange        string
	Tradingsymbol   string
	TransactionType string
	Product         string
	OrderType       string
	Quantity        int
}

// OrderEvent is one append-only transition-log row — the SEBI audit
// trail (PLAN.md §2). It lives in domain, not a specific store package,
// so worker can append events without depending on a concrete store.
type OrderEvent struct {
	ID              int64
	FollowerOrderID *int64
	MasterFillID    *int64
	AccountID       uuid.UUID
	EventType       string
	Payload         []byte
	OccurredAt      time.Time
}
