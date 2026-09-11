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
	Status          string
	OrderTimestamp  time.Time
	RawPayload      []byte
	ReceivedAt      time.Time
	DispatchState   DispatchState
	DispatchedAt    *time.Time
}

// FollowLink is the 1-group-per-follower relationship: a follower belongs to
// at most one group, structurally enforced wherever this is persisted
// (follower_id as primary key — PLAN.md §2).
type FollowLink struct {
	FollowerID     uuid.UUID
	GroupID        uuid.UUID
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

// ErrConflict is returned by a store when an operation is blocked by an
// existing reference — e.g. deleting an account still attached to a
// group or referenced by order history (PLAN.md has no cascading-delete
// story; Postgres's own FK constraints are the source of truth here).
var ErrConflict = errors.New("domain: conflict")

// Account is the flat, ungrouped view of a single account (master or
// follower) the Accounts page manages — unlike GroupSummary/GroupDetail,
// which model the master+followers rollup, this is one row per account
// regardless of group membership (docs/APIs/accounts.md).
type Account struct {
	ID              uuid.UUID
	Name            string
	Role            string
	Broker          string
	BrokerAccountID string
	Active          bool
	Status          string
	GroupID         *uuid.UUID
	GroupName       *string
	MasterID        *uuid.UUID
	CapitalRatio    *decimal.Decimal
	MaxQtyPerOrder  *int
	Enabled         bool
}

// Group models a trading group with a designated master account.
type Group struct {
	ID        uuid.UUID
	Name      string
	MasterID  uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

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

// OrderUpdate is a status change on an order already placed (almost always
// a follower's), delivered by postback or WS after the initial place call.
// It carries only what's needed to locate and update the matching
// follower_orders row by BrokerOrderID — unlike MasterFill, it never
// creates a new row.
type OrderUpdate struct {
	BrokerOrderID  string
	Status         string
	FilledQuantity int
	AveragePrice   decimal.Decimal
	OrderTimestamp time.Time
	RawPayload     []byte
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

// GroupSummary is one row of the Dashboard's group list: a master account
// plus a rollup of its followers.
type GroupSummary struct {
	ID              uuid.UUID
	Name            string
	MasterID        uuid.UUID
	MasterAccountID string
	MasterName      string
	Broker          string
	FollowerCount   int
	Status          string
	Active          bool
}

// GroupFollower is one follower row inside a GroupDetail. MTM/cash/margin/
// net-qty/positions are intentionally absent — they require broker data
// (gokiteconnect's GetMargins/GetPositions) not wired yet (docs/APIs/groups.md).
type GroupFollower struct {
	AccountID       uuid.UUID
	Name            string
	BrokerAccountID string
	Enabled         bool
	Status          string
}

// GroupDetail is the full Dashboard GroupCard payload for one group.
type GroupDetail struct {
	GroupID         uuid.UUID
	GroupName       string
	MasterID        uuid.UUID
	MasterAccountID string
	MasterName      string
	MasterActive    bool
	Followers       []GroupFollower
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

// AccountSummaryMetrics represents the persistent top summary header for an account.
type AccountSummaryMetrics struct {
	NetQty               int             `json:"netQty"`
	OpenPositionsCount   int             `json:"openPositionsCount"`
	ClosedPositionsCount int             `json:"closedPositionsCount"`
	PendingOrdersCount   int             `json:"pendingOrdersCount"`
	TotalMtm             decimal.Decimal `json:"totalMtm"`
	RealizedPnl          decimal.Decimal `json:"realizedPnl"`
	AccountValue         decimal.Decimal `json:"accountValue"`
	Status               string          `json:"status"`
}

// PositionItem represents an open or closed position row.
type PositionItem struct {
	Product    string          `json:"product"`
	Instrument string          `json:"instrument"`
	Qty        int             `json:"qty"`
	AvgPrice   string          `json:"avgPrice"`
	Ltp        decimal.Decimal `json:"ltp"`
	Mtm        decimal.Decimal `json:"mtm"`
	Action     string          `json:"action,omitempty"`
}

// HoldingItem represents a portfolio holding row.
type HoldingItem struct {
	Instrument       string          `json:"instrument"`
	SellableQuantity int             `json:"sellableQuantity"`
	BuyAveragePrice  decimal.Decimal `json:"buyAveragePrice"`
	Ltp              decimal.Decimal `json:"ltp"`
	Pnl              decimal.Decimal `json:"pnl"`
	Action           string          `json:"action"`
}

// OrderDetailItem represents an order row in Open, Closed, or Rejected order tabs.
type OrderDetailItem struct {
	ID           string           `json:"id,omitempty"`
	Product      string           `json:"product,omitempty"`
	Time         string           `json:"time"`
	Instrument   string           `json:"instrument"`
	Quantity     int              `json:"quantity"`
	Price        *decimal.Decimal `json:"price,omitempty"`
	TriggerPrice *decimal.Decimal `json:"triggerPrice,omitempty"`
	LimitPrice   *decimal.Decimal `json:"limitPrice,omitempty"`
	Type         string           `json:"type"` // "B" or "S"
	Status       string           `json:"status,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	Action       string           `json:"action,omitempty"`
}

// TabCounts stores the total counts for each of the 6 drawer tabs.
type TabCounts struct {
	OpenPositions   int `json:"openPositions"`
	ClosedPositions int `json:"closedPositions"`
	Holdings        int `json:"holdings"`
	OpenOrders      int `json:"openOrders"`
	ClosedOrders    int `json:"closedOrders"`
	RejectedOrders  int `json:"rejectedOrders"`
}

// PaginationInfo describes pagination state for the active tab.
type PaginationInfo struct {
	Tab        string `json:"tab"`
	Page       int    `json:"page"`
	Limit      int    `json:"limit"`
	TotalCount int    `json:"totalCount"`
	TotalPages int    `json:"totalPages"`
}

// AccountOrdersDetail is the complete response for GET /api/v1/accounts/{id}/orders.
type AccountOrdersDetail struct {
	AccountID       uuid.UUID             `json:"accountId"`
	Role            string                `json:"role"`
	BrokerAccountID string                `json:"brokerAccountId"`
	Summary         AccountSummaryMetrics `json:"summary"`
	Counts          TabCounts             `json:"counts"`
	Pagination      PaginationInfo        `json:"pagination"`
	OpenPositions   []PositionItem        `json:"openPositions"`
	ClosedPositions []PositionItem        `json:"closedPositions"`
	Holdings        []HoldingItem         `json:"holdings"`
	OpenOrders      []OrderDetailItem     `json:"openOrders"`
	ClosedOrders    []OrderDetailItem     `json:"closedOrders"`
	RejectedOrders  []OrderDetailItem     `json:"rejectedOrders"`
}
