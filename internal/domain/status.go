package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AccountStatus represents the operational status of an account.
type AccountStatus string

const (
	AccountStatusActive  AccountStatus = "active"
	AccountStatusError   AccountStatus = "error"
	AccountStatusPending AccountStatus = "pending"
)

// ErrInvalidAccountStatus is returned when an unrecognized account status is provided.
var ErrInvalidAccountStatus = errors.New("invalid account status: must be 'active', 'ok', 'pending', or 'error'")

// ParseAccountStatus parses and validates an account status string.
// Accepts "active", "ok", "pending", or "error". Both "active" and "ok" are normalized to AccountStatusActive.
func ParseAccountStatus(s string) (AccountStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "active", "ok":
		return AccountStatusActive, nil
	case "pending":
		return AccountStatusPending, nil
	case "error":
		return AccountStatusError, nil
	default:
		return "", ErrInvalidAccountStatus
	}
}

// IsValid checks if an AccountStatus is a recognized valid status.
func (s AccountStatus) IsValid() bool {
	return s == AccountStatusActive || s == AccountStatusError || s == AccountStatusPending
}

// ToAPI returns the client-facing API representation ("ok", "pending", or "error").
func (s AccountStatus) ToAPI() string {
	if s == AccountStatusActive {
		return "ok"
	}
	if s == AccountStatusPending {
		return "pending"
	}
	return "error"
}

// TickerStatus represents the operational status of a master account's WebSocket ticker.
type TickerStatus struct {
	MasterID uuid.UUID `json:"masterId"`
	Active   bool      `json:"active"`
}

// QueueStatus represents in-memory queue metrics.
type QueueStatus struct {
	MasterFillDepth  int `json:"masterFillDepth"`
	MasterFillCap    int `json:"masterFillCap"`
	OrderUpdateDepth int `json:"orderUpdateDepth"`
	OrderUpdateCap   int `json:"orderUpdateCap"`
}

// DBPoolStatus represents in-memory connection pool counters.
type DBPoolStatus struct {
	AcquiredConns int32 `json:"acquiredConns"`
	IdleConns     int32 `json:"idleConns"`
	TotalConns    int32 `json:"totalConns"`
	MaxConns      int32 `json:"maxConns"`
}

// SystemResources represents process-level CPU, memory, and runtime metrics.
type SystemResources struct {
	CPUPercent     float64 `json:"cpuPercent"`
	HeapAllocMB    float64 `json:"heapAllocMb"`
	SysMemoryMB    float64 `json:"sysMemoryMb"`
	GoroutineCount int     `json:"goroutineCount"`
	GCCycles       uint32  `json:"gcCycles"`
}

// PipelineStatus models the overall in-memory system health and telemetry.
type PipelineStatus struct {
	KillSwitch      KillSwitchStatus `json:"killSwitch"`
	MasterTickers   []TickerStatus   `json:"masterTickers"`
	Queues          QueueStatus      `json:"queues"`
	WorkerCount     int              `json:"workerCount"`
	DBPool          DBPoolStatus     `json:"dbPool"`
	SystemResources SystemResources  `json:"systemResources"`
}

// AttentionAccount represents an account that is unauthenticated or has an expired token.
type AttentionAccount struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	BrokerAccountID string     `json:"brokerAccountId"`
	Role            string     `json:"role"`
	AuthStatus      string     `json:"authStatus"`
	AuthError       string     `json:"authError"`
	TokenExpiresAt  *time.Time `json:"tokenExpiresAt,omitempty"`
}

// BrokerReadinessSummary summarizes account readiness for live market trading.
type BrokerReadinessSummary struct {
	MasterReady       bool               `json:"masterReady"`
	TotalFollowers    int                `json:"totalFollowers"`
	FollowersReady    int                `json:"followersReady"`
	AttentionAccounts []AttentionAccount `json:"attentionAccounts"`
}

// OrderIssue represents a failed, rejected, or lot-sized-down follower order.
type OrderIssue struct {
	ID              int64     `json:"id"`
	CreatedAt       time.Time `json:"createdAt"`
	FollowerName    string    `json:"followerName"`
	BrokerAccountID string    `json:"brokerAccountId"`
	Tradingsymbol   string    `json:"tradingsymbol"`
	Exchange        string    `json:"exchange"`
	IntendedQty     int       `json:"intendedQty"`
	PlacedQty       int       `json:"placedQty"`
	SizingReason    string    `json:"sizingReason"`
	TerminalStatus  string    `json:"terminalStatus"`
	LastError       string    `json:"lastError"`
}

// ExecutionEvent represents an instantaneous master fill and fan-out dispatch event.
type ExecutionEvent struct {
	Timestamp       time.Time `json:"timestamp"`
	Tradingsymbol   string    `json:"tradingsymbol"`
	Exchange        string    `json:"exchange"`
	TransactionType string    `json:"transactionType"`
	MasterQty       int       `json:"masterQty"`
	Price           float64   `json:"price"`
	FollowersCount  int       `json:"followersCount"`
	SkippedCount    int       `json:"skippedCount"`
	LatencyMs       int64     `json:"latencyMs"`
}

// BulkProxyTestSummary summarizes the on-demand probe of all assigned proxies.
type BulkProxyTestSummary struct {
	TotalTested  int               `json:"totalTested"`
	SuccessCount int               `json:"successCount"`
	FailureCount int               `json:"failureCount"`
	AvgLatencyMs int               `json:"avgLatencyMs"`
	Results      []ProxyTestResult `json:"results"`
}

