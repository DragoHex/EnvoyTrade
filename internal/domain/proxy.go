package domain

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrIPAlreadyAssigned is returned when attempting to assign a proxy IP
// that is already bound to another account (enforcing 1:1 assignment).
var ErrIPAlreadyAssigned = errors.New("domain: IP address is already assigned to another account")

// ExpiryStatus represents the expiration lifecycle state of a proxy IP.
type ExpiryStatus string

const (
	ExpiryActive       ExpiryStatus = "active"        // > 15 days remaining
	ExpiryExpiringSoon ExpiryStatus = "expiring_soon" // <= 15 days and >= 0
	ExpiryExpired      ExpiryStatus = "expired"       // < 0 days remaining
)

// ComputeExpiry calculates the days remaining and expiration status relative to reference time.
func ComputeExpiry(validUntil time.Time, now time.Time) (int, ExpiryStatus) {
	if validUntil.IsZero() {
		return 0, ExpiryExpired
	}
	diff := validUntil.Sub(now)
	days := int(diff.Hours() / 24)
	if diff < 0 {
		return days, ExpiryExpired
	}
	if days <= 15 {
		return days, ExpiryExpiringSoon
	}
	return days, ExpiryActive
}

// ProxyIP models a dedicated static proxy IP from the AlgoIP pool for
// Kite Connect broker egress (PLAN.md §3.3, docs/APIs/proxy_ips.md).
type ProxyIP struct {
	IPAddress           string       `json:"ipAddress"`
	IPType              string       `json:"ipType"` // "ipv4" or "ipv6"
	Host                string       `json:"host"`
	Port                int          `json:"port"`
	Username            string       `json:"username,omitempty"`
	Password            string       `json:"-"`
	WhitelistedBrokers  string       `json:"whitelistedBrokers"`
	ValidFrom           time.Time    `json:"validFrom"`
	ValidUntil          time.Time    `json:"validUntil"`
	Plan                string       `json:"plan"`
	CreatedAt           time.Time    `json:"createdAt"`
	IsAssigned          bool         `json:"isAssigned"`
	AssignedAccountID   *uuid.UUID   `json:"assignedAccountId,omitempty"`
	AssignedAccountName *string      `json:"assignedAccountName,omitempty"`
	DaysRemaining       int          `json:"daysRemaining"`
	ExpiryStatus        ExpiryStatus `json:"expiryStatus"`
}

// ProxyIPInput models the JSON format imported in bulk (e.g. ips.json) or single creation.
type ProxyIPInput struct {
	IP                      string `json:"ip"`
	Host                    string `json:"host"`
	Port                    int    `json:"port"`
	UserID                  string `json:"user_id"`
	Username                string `json:"username"`
	Password                string `json:"password"`
	WhitelistedBrokers      string `json:"whitelisted_brokers,omitempty"`
	WhitelistedBrokersCamel string `json:"whitelistedBrokers,omitempty"`
	WhitelistedIP           string `json:"whitelisted_ip,omitempty"`
	ValidFrom          string `json:"valid_from"`
	ValidUntil         string `json:"valid_until"`
	Plan               string `json:"plan"`
	Status             string `json:"status,omitempty"`
	Type               string `json:"type"`
}

// ProxyIPStats provides aggregated inventory and expiration metrics for the admin portal.
type ProxyIPStats struct {
	TotalCount        int `json:"totalCount"`
	AssignedCount     int `json:"assignedCount"`
	AvailableCount    int `json:"availableCount"`
	IPv4Count         int `json:"ipv4Count"`
	IPv6Count         int `json:"ipv6Count"`
	ExpiringSoonCount int `json:"expiringSoonCount"`
	ExpiredCount      int `json:"expiredCount"`
	AssignedExpiring  int `json:"assignedExpiring"`
}

// ComputeProxyIPStats aggregates inventory and expiration status counts from a slice of ProxyIPs.
// It performs a single pass over the slice in Go memory, avoiding redundant SQL aggregate queries.
func ComputeProxyIPStats(ips []ProxyIP, refTime ...time.Time) ProxyIPStats {
	var stats ProxyIPStats
	stats.TotalCount = len(ips)

	now := time.Now()
	if len(refTime) > 0 && !refTime[0].IsZero() {
		now = refTime[0]
	}

	for _, ip := range ips {
		if ip.IsAssigned {
			stats.AssignedCount++
		} else {
			stats.AvailableCount++
		}

		cleanType := strings.ToLower(strings.TrimSpace(ip.IPType))
		switch cleanType {
		case "ipv4":
			stats.IPv4Count++
		case "ipv6":
			stats.IPv6Count++
		}

		status := ip.ExpiryStatus
		if status == "" && !ip.ValidUntil.IsZero() {
			_, status = ComputeExpiry(ip.ValidUntil, now)
		}

		switch status {
		case ExpiryExpiringSoon:
			stats.ExpiringSoonCount++
			if ip.IsAssigned {
				stats.AssignedExpiring++
			}
		case ExpiryExpired:
			stats.ExpiredCount++
			if ip.IsAssigned {
				stats.AssignedExpiring++
			}
		}
	}

	return stats
}

// BulkUploadResult provides the outcome of an IP import batch.
type BulkUploadResult struct {
	Total    int      `json:"total"`
	Inserted int      `json:"inserted"`
	Updated  int      `json:"updated"`
	Errors   []string `json:"errors"`
}

// ProxyTestResult provides the outcome of a single-IP connectivity probe.
type ProxyTestResult struct {
	IPAddress string `json:"ipAddress"`
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latencyMs"`
	EgressIP  string `json:"egressIp,omitempty"`
	Error     string `json:"error,omitempty"`
}

// KillSwitchStatus models the state of the emergency fan-out kill switch.
type KillSwitchStatus struct {
	Engaged   bool       `json:"engaged"`
	EngagedAt *time.Time `json:"engagedAt,omitempty"`
	EngagedBy string     `json:"engagedBy,omitempty"`
	Reason    string     `json:"reason,omitempty"`
}

// ParseProxyTimestamp parses timestamps in IST format "2006-01-02 15:04:05 (IST)"
// or standard RFC3339 format into a time.Time in UTC.
func ParseProxyTimestamp(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}

	// 1. Check for "(IST)" suffix
	clean := strings.TrimSuffix(trimmed, " (IST)")
	clean = strings.TrimSuffix(clean, "(IST)")
	clean = strings.TrimSpace(clean)

	// Try standard date-time formats in IST
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, clean, IST); err == nil {
			return t.UTC(), nil
		}
	}

	// Try RFC3339 / ISO8601
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("unrecognized timestamp format %q", raw)
}

// ParseAndValidateProxyIP normalizes and validates a ProxyIPInput into a domain.ProxyIP.
func ParseAndValidateProxyIP(input ProxyIPInput, now time.Time) (ProxyIP, error) {
	ipStr := strings.TrimSpace(input.IP)
	if ipStr == "" {
		return ProxyIP{}, errors.New("IP address is required")
	}

	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return ProxyIP{}, fmt.Errorf("invalid IP address format %q", ipStr)
	}

	ipType := strings.ToLower(strings.TrimSpace(input.Type))
	if ipType == "" {
		if strings.Contains(ipStr, ":") {
			ipType = "ipv6"
		} else {
			ipType = "ipv4"
		}
	} else if ipType != "ipv4" && ipType != "ipv6" {
		return ProxyIP{}, fmt.Errorf("invalid IP type %q, must be ipv4 or ipv6", input.Type)
	}

	host := strings.TrimSpace(input.Host)
	if host == "" {
		return ProxyIP{}, errors.New("host is required")
	}

	port := input.Port
	if port <= 0 {
		port = 443
	}

	username := strings.TrimSpace(input.UserID)
	if username == "" {
		username = strings.TrimSpace(input.Username)
	}

	validFrom, err := ParseProxyTimestamp(input.ValidFrom)
	if err != nil {
		return ProxyIP{}, fmt.Errorf("invalid valid_from: %w", err)
	}
	if validFrom.IsZero() {
		validFrom = now
	}

	validUntil, err := ParseProxyTimestamp(input.ValidUntil)
	if err != nil {
		return ProxyIP{}, fmt.Errorf("invalid valid_until: %w", err)
	}
	if validUntil.IsZero() {
		validUntil = validFrom.AddDate(0, 3, 0)
	}

	plan := strings.ToUpper(strings.TrimSpace(input.Plan))
	if plan == "" {
		plan = "QUARTERLY"
	}

	daysRemaining, expiryStatus := ComputeExpiry(validUntil, now)

	rawBrokers := strings.TrimSpace(input.WhitelistedBrokers)
	if rawBrokers == "" {
		rawBrokers = strings.TrimSpace(input.WhitelistedBrokersCamel)
	}

	return ProxyIP{
		IPAddress:          ipStr,
		IPType:             ipType,
		Host:               host,
		Port:               port,
		Username:           username,
		Password:           strings.TrimSpace(input.Password),
		WhitelistedBrokers:  NormalizeWhitelistedBrokers(rawBrokers),
		ValidFrom:          validFrom,
		ValidUntil:         validUntil,
		Plan:               plan,
		CreatedAt:          now,
		DaysRemaining:      daysRemaining,
		ExpiryStatus:       expiryStatus,
	}, nil
}

// NormalizeWhitelistedBrokers normalizes broker names:
// "zerodha" / "kite" -> "Zerodha"
// "testbroker" / "test_broker" -> "TestBroker"
// Multiple comma-separated brokers are trimmed and normalized.
func NormalizeWhitelistedBrokers(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		lower := strings.ToLower(item)
		switch lower {
		case "zerodha", "kite":
			normalized = append(normalized, "Zerodha")
		case "dhan":
			normalized = append(normalized, "Dhan")
		case "testbroker", "test_broker":
			normalized = append(normalized, "TestBroker")
		default:
			normalized = append(normalized, item)
		}
	}
	return strings.Join(normalized, ", ")
}

// MatchesBroker checks if an account's broker is whitelisted on this proxy IP.
// If WhitelistedBrokers is empty, all brokers are allowed (backwards compatible).
func (p ProxyIP) MatchesBroker(broker string) bool {
	if strings.TrimSpace(p.WhitelistedBrokers) == "" {
		return true
	}
	target := strings.ToLower(strings.TrimSpace(broker))
	if target == "kite" {
		target = "zerodha"
	} else if target == "test_broker" {
		target = "testbroker"
	}

	parts := strings.Split(p.WhitelistedBrokers, ",")
	for _, part := range parts {
		cand := strings.ToLower(strings.TrimSpace(part))
		if cand == "kite" {
			cand = "zerodha"
		} else if cand == "test_broker" {
			cand = "testbroker"
		}
		if cand == target {
			return true
		}
	}
	return false
}

// BrokerPriority returns the candidate priority of this proxy IP for the given broker:
// - 0: Incompatible (not whitelisted for this broker; must be excluded)
// - 1: Broker-specialized (explicitly whitelisted for this broker)
// - 2: All brokers / unrestricted (whitelisted_brokers is empty; fallback)
func (p ProxyIP) BrokerPriority(broker string) int {
	if !p.MatchesBroker(broker) {
		return 0
	}
	if strings.TrimSpace(p.WhitelistedBrokers) != "" {
		return 1
	}
	return 2
}


