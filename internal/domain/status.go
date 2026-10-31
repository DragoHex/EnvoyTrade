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

// RequiresDailySession returns true for brokers whose access tokens strictly expire every day
// at a fixed cutoff (e.g. Zerodha Kite Connect at 06:00 AM IST).
// For such brokers, missing token_expires_at implies an inactive daily session.
// For other brokers (e.g. Dhan with 24h JWTs, long-lived API tokens, or testbroker),
// an authenticated session without an explicit expiration timestamp is not treated as expired.
func RequiresDailySession(broker string) bool {
	b := strings.ToLower(strings.TrimSpace(broker))
	return b == "zerodha" || b == "kite"
}

// SanitizeAuthError cleans and formats an authentication error message for user-facing display.
// It guarantees that no backend API calls, URLs, HTTP endpoints, method signatures,
// or internal payload dumps are ever exposed to the user.
func SanitizeAuthError(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)

	// Check if the message contains backend API traces
	hasBackendAPITraces := strings.Contains(lower, "http://") ||
		strings.Contains(lower, "https://") ||
		strings.Contains(lower, "post ") ||
		strings.Contains(lower, "get ") ||
		strings.Contains(lower, "/api/") ||
		strings.Contains(lower, "connect/login") ||
		strings.Contains(lower, "session/token") ||
		strings.Contains(lower, "sess_id") ||
		strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "get dhan") ||
		strings.Contains(lower, "dhan api error") ||
		strings.Contains(lower, "kite web login") ||
		strings.Contains(lower, "dhan sync failed") ||
		strings.Contains(lower, "connect login") ||
		strings.Contains(lower, "kite login error") ||
		strings.Contains(lower, "kite 2fa error") ||
		strings.Contains(lower, "kotak-neo api error")

	// If no backend API traces and already a clean user-facing message, preserve it
	if !hasBackendAPITraces {
		return raw
	}

	// Translate technical / backend API errors into clear, safe user-facing explanations
	switch {
	case strings.Contains(lower, "user not found") || strings.Contains(lower, "ucc") || strings.Contains(lower, "10564"):
		return "Invalid Kotak Neo Client ID (UCC) or Trade API not enabled for this user"

	case strings.Contains(lower, "mobilenumber") || strings.Contains(lower, "mobile number"):
		return "Invalid Kotak Neo registered mobile number"

	case strings.Contains(lower, "mpin"):
		return "Invalid Kotak Neo MPIN"

	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "token expired") || strings.Contains(lower, "invalid token") ||
		strings.Contains(lower, "token revoked"):
		return "Broker session expired or token invalid"

	case strings.Contains(lower, "totp") || strings.Contains(lower, "2fa") ||
		strings.Contains(lower, "twofa") || strings.Contains(lower, "external totp"):
		return "Invalid 2FA TOTP code or external TOTP not enabled"

	case strings.Contains(lower, "password") || strings.Contains(lower, "credentials") ||
		strings.Contains(lower, "user id") || strings.Contains(lower, "user_id") ||
		strings.Contains(lower, "login error"):
		return "Invalid broker credentials or password"

	case strings.Contains(lower, "500") || strings.Contains(lower, "502") ||
		strings.Contains(lower, "503") || strings.Contains(lower, "504") ||
		strings.Contains(lower, "gateway") || strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "dial") || strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "econnrefused") || strings.Contains(lower, "dh-1111") ||
		strings.Contains(lower, "server error") || strings.Contains(lower, "service unavailable"):
		return "Broker server temporarily unavailable"

	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests"):
		return "Broker rate limit exceeded"

	case strings.Contains(lower, "missing") || strings.Contains(lower, "required"):
		return "Missing broker API key or credentials"

	default:
		return "Authentication failed with broker"
	}
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

