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
	Price           decimal.Decimal
	TriggerPrice    decimal.Decimal
	AveragePrice    decimal.Decimal
	Status          string
	OrderTimestamp  time.Time
	RawPayload      []byte
	Tag             string
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
	CloneFactor    decimal.Decimal
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
	ID              int64
	MasterFillID    int64
	FollowerID      uuid.UUID
	IdempotencyTag  string
	IntendedQty     int
	LotSize         int
	SizingReason    SizingReason
	PlacedQty       *int
	BrokerOrderID   string
	TerminalStatus  string
	FilledQty       int
	AveragePrice    decimal.Decimal
	AttemptCount    int
	LastError       string
	Origin          string
	Tradingsymbol   string
	Exchange        string
	Product         string
	TransactionType string
	OrderType       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
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

// ErrBrokerUnreachable is returned when an upstream broker API or network call fails.
var ErrBrokerUnreachable = errors.New("domain: broker is not reachable")

// ErrAuthExpired is returned when a broker session token or API credentials have expired.
var ErrAuthExpired = errors.New("domain: broker account login has expired")

// ErrMasterHasOpenPositions is returned when a master account swap is attempted
// while the current master account still has open positions.
var ErrMasterHasOpenPositions = errors.New("domain: master has open positions")

// ErrUnproxiedNotAllowed is returned when an order routing or execution operation
// is attempted on a broker that is not configured with a static proxy.
var ErrUnproxiedNotAllowed = errors.New("domain: unproxied order routing is not allowed: IP needs to be set for this operation")

// ErrIPRequired is returned when an account operation requires a dedicated IP address.
var ErrIPRequired = errors.New("domain: IP address is required for follower accounts")

// ErrAccountDisabled is returned when an operation cannot be performed
// because the follower account is disabled.
var ErrAccountDisabled = errors.New("domain: follower account is disabled")


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
	ApiKey          string
	ApiSecret       string
	Active          bool
	Status          string
	IPAddress       string
	AuthStatus      string
	AuthError       string
	GroupID         *uuid.UUID
	GroupName       *string
	MasterID        *uuid.UUID
	CloneFactor     *decimal.Decimal
	MaxQtyPerOrder  *int
	Enabled         bool
}

// AccountAuthInfo holds authentication credentials and session details for an account.
type AccountAuthInfo struct {
	ID                  uuid.UUID
	Role                string
	Broker              string
	BrokerAccountID     string
	ApiKey              string
	ApiSecret           string
	IPAddress           string
	EncryptedPassword   string
	EncryptedTotpSecret string
	AccessToken         string
	TokenExpiresAt      *time.Time
	AuthStatus          string
	AuthError           string
}

// PositionSyncParam represents one position row to sync.
type PositionSyncParam struct {
	Product      string
	Instrument   string
	Quantity     int
	BuyPrice     decimal.Decimal
	SellPrice    decimal.Decimal
	BuyQuantity  int
	SellQuantity int
	Ltp          decimal.Decimal
	Mtm          decimal.Decimal
	Pnl          decimal.Decimal
	Action       string
}

// HoldingSyncParam represents one holding row to sync.
type HoldingSyncParam struct {
	Instrument       string
	SellableQuantity int
	BuyAveragePrice  decimal.Decimal
	Ltp              decimal.Decimal
	Pnl              decimal.Decimal
	Action           string
}

// MarginSyncParam represents account margin and summary metrics to sync.
type MarginSyncParam struct {
	NetQty          int
	TotalMtm        decimal.Decimal
	RealizedPnl     decimal.Decimal
	AccountValue    decimal.Decimal
	AvailableCash   *decimal.Decimal
	AvailableMargin *decimal.Decimal
	Status          string
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

// OrderUpdate is a status change or terminal execution report for a follower order,
// delivered by broker postback or WS after placement, or on external/direct execution.
type OrderUpdate struct {
	FollowerID      uuid.UUID
	BrokerOrderID   string
	Tradingsymbol   string
	Exchange        string
	Product         string
	TransactionType string
	OrderType       string
	Quantity        int
	Status          string
	FilledQuantity  int
	AveragePrice    decimal.Decimal
	OrderTimestamp  time.Time
	Tag             string
	RawPayload      []byte
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
	Price           float64
	TriggerPrice    float64
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

// GroupFollower is one follower row inside a GroupDetail with its summary metrics.
type GroupFollower struct {
	AccountID            uuid.UUID
	Name                 string
	BrokerAccountID      string
	Enabled              bool
	Status               string
	NetQty               int
	OpenPositionsCount   int
	ClosedPositionsCount int
	OpenOrdersCount      int
	TotalMtm             decimal.Decimal
	AvailableCash        *decimal.Decimal
	AvailableMargin      *decimal.Decimal
}

// GroupDetail is the full Dashboard GroupCard payload for one group.
type GroupDetail struct {
	GroupID                    uuid.UUID
	GroupName                  string
	MasterID                   uuid.UUID
	MasterAccountID            string
	MasterName                 string
	MasterActive               bool
	MasterNetQty               int
	MasterOpenPositionsCount   int
	MasterClosedPositionsCount int
	MasterOpenOrdersCount      int
	MasterTotalMtm             decimal.Decimal
	MasterAvailableCash        *decimal.Decimal
	MasterAvailableMargin      *decimal.Decimal
	Followers                  []GroupFollower
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
	NetQty               int              `json:"netQty"`
	OpenPositionsCount   int              `json:"openPositionsCount"`
	ClosedPositionsCount int              `json:"closedPositionsCount"`
	PendingOrdersCount   int              `json:"pendingOrdersCount"`
	TotalMtm             decimal.Decimal  `json:"totalMtm"`
	RealizedPnl          decimal.Decimal  `json:"realizedPnl"`
	AccountValue         decimal.Decimal  `json:"accountValue"`
	AvailableCash        *decimal.Decimal `json:"availableCash,omitempty"`
	AvailableMargin      *decimal.Decimal `json:"availableMargin,omitempty"`
	Status               string           `json:"status"`
}

// PositionItem represents an open or closed position row.
type PositionItem struct {
	Product    string          `json:"product"`
	Instrument string          `json:"instrument"`
	Qty        int             `json:"qty"`
	AvgPrice   string          `json:"avgPrice"`
	Ltp        decimal.Decimal `json:"ltp"`
	Mtm        decimal.Decimal `json:"mtm"`
	Pnl        decimal.Decimal `json:"pnl"`
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

// SquareOffRequest carries optional symbol filters for square-off operations.
type SquareOffRequest struct {
	Symbols []string `json:"symbols,omitempty"`
}

// SquareOffOrder represents an order placed as part of a square-off operation.
type SquareOffOrder struct {
	AccountID     uuid.UUID `json:"account_id"`
	Role          string    `json:"role"`
	BrokerOrderID string    `json:"broker_order_id"`
	Exchange      string    `json:"exchange"`
	Tradingsymbol string    `json:"tradingsymbol"`
	Product       string    `json:"product"`
	Side          string    `json:"side"` // BUY | SELL
	Quantity      int       `json:"quantity"`
	Status        string    `json:"status"`
}

// SquareOffResult is the execution receipt returned by square-off endpoints.
type SquareOffResult struct {
	Action                      string           `json:"action"`
	Status                      string           `json:"status"` // completed | partial | empty
	GroupID                     *uuid.UUID       `json:"group_id,omitempty"`
	AccountID                   *uuid.UUID       `json:"account_id,omitempty"`
	Role                        string           `json:"role,omitempty"`
	FollowersAffected           int              `json:"followers_affected"`
	CancelledOrders             int              `json:"cancelled_orders"`
	PositionsSquaredOff         int              `json:"positions_squared_off"`
	Orders                      []SquareOffOrder `json:"orders"`
	Errors                      []string         `json:"errors,omitempty"`
}

// RebalanceRequest carries target follower IDs for cluster rebalance operations.
type RebalanceRequest struct {
	FollowerIDs []uuid.UUID `json:"follower_ids,omitempty"`
}

// SymbolDrift represents the position drift for a single instrument on a follower.
type SymbolDrift struct {
	Exchange      string `json:"exchange"`
	Tradingsymbol string `json:"tradingsymbol"`
	Product       string `json:"product"`
	LotSize       int    `json:"lot_size"`
	MasterQty     int    `json:"master_qty"`
	TargetQty     int    `json:"target_qty"`
	FollowerQty   int    `json:"follower_qty"`
	DriftQty      int    `json:"drift_qty"`
	Action        string `json:"action"` // BUY | SELL
}

// FollowerDrift represents all drifting positions for a given follower account.
type FollowerDrift struct {
	AccountID       uuid.UUID       `json:"account_id"`
	AccountName     string          `json:"account_name"`
	BrokerAccountID string          `json:"broker_account_id"`
	Enabled         bool            `json:"enabled"`
	CloneFactor     decimal.Decimal `json:"clone_factor"`
	Symbols         []SymbolDrift   `json:"symbols"`
}

// GroupRebalanceDiff represents the preview diff of all drifting followers in a group.
type GroupRebalanceDiff struct {
	GroupID            uuid.UUID       `json:"group_id"`
	MasterID           uuid.UUID       `json:"master_id"`
	FollowersEvaluated int             `json:"followers_evaluated"`
	FollowersWithDrift int             `json:"followers_with_drift"`
	Drifts             []FollowerDrift `json:"drifts"`
}

// RebalanceOrder represents an order placed to resolve position drift.
type RebalanceOrder struct {
	AccountID     uuid.UUID `json:"account_id"`
	Role          string    `json:"role"`
	BrokerOrderID string    `json:"broker_order_id"`
	Exchange      string    `json:"exchange"`
	Tradingsymbol string    `json:"tradingsymbol"`
	Product       string    `json:"product"`
	Side          string    `json:"side"` // BUY | SELL
	Quantity      int       `json:"quantity"`
	Status        string    `json:"status"`
}

// RebalanceResult is the execution receipt returned by rebalance endpoints.
type RebalanceResult struct {
	Action            string           `json:"action"`
	Status            string           `json:"status"` // completed | partial | empty
	GroupID           *uuid.UUID       `json:"group_id,omitempty"`
	AccountID         *uuid.UUID       `json:"account_id,omitempty"`
	FollowersAffected int              `json:"followers_affected"`
	CancelledOrders   int              `json:"cancelled_orders"`
	OrdersPlaced      int              `json:"orders_placed"`
	Orders            []RebalanceOrder `json:"orders"`
	Errors            []string         `json:"errors,omitempty"`
}

