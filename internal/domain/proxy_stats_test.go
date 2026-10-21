package domain_test

import (
	"testing"
	"time"

	"envoytrade/internal/domain"
	"github.com/google/uuid"
)

func TestComputeProxyIPStats(t *testing.T) {
	refTime := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	accountID := uuid.New()

	t.Run("nil slice returns zero stats", func(t *testing.T) {
		stats := domain.ComputeProxyIPStats(nil, refTime)
		if stats != (domain.ProxyIPStats{}) {
			t.Errorf("expected zero stats, got %+v", stats)
		}
	})

	t.Run("empty slice returns zero stats", func(t *testing.T) {
		stats := domain.ComputeProxyIPStats([]domain.ProxyIP{}, refTime)
		if stats != (domain.ProxyIPStats{}) {
			t.Errorf("expected zero stats, got %+v", stats)
		}
	})

	t.Run("positive mixed inventory scenario", func(t *testing.T) {
		ips := []domain.ProxyIP{
			// 1. IPv4, Assigned, Active (> 15 days)
			{
				IPAddress:         "10.0.0.1",
				IPType:            "ipv4",
				IsAssigned:        true,
				AssignedAccountID: &accountID,
				ValidUntil:        refTime.AddDate(0, 1, 0), // +30 days
				ExpiryStatus:      domain.ExpiryActive,
			},
			// 2. IPv4, Unassigned, ExpiringSoon (10 days)
			{
				IPAddress:    "10.0.0.2",
				IPType:       "ipv4",
				IsAssigned:   false,
				ValidUntil:   refTime.Add(10 * 24 * time.Hour),
				ExpiryStatus: domain.ExpiryExpiringSoon,
			},
			// 3. IPv6, Assigned, ExpiringSoon (5 days) -> increments AssignedExpiring
			{
				IPAddress:         "2401:c080::1",
				IPType:            "ipv6",
				IsAssigned:        true,
				AssignedAccountID: &accountID,
				ValidUntil:        refTime.Add(5 * 24 * time.Hour),
				ExpiryStatus:      domain.ExpiryExpiringSoon,
			},
			// 4. IPv6, Assigned, Expired (-2 days) -> increments AssignedExpiring
			{
				IPAddress:         "2401:c080::2",
				IPType:            "ipv6",
				IsAssigned:        true,
				AssignedAccountID: &accountID,
				ValidUntil:        refTime.Add(-2 * 24 * time.Hour),
				ExpiryStatus:      domain.ExpiryExpired,
			},
			// 5. IPv4, Unassigned, Expired (-10 days) -> does NOT increment AssignedExpiring
			{
				IPAddress:    "10.0.0.5",
				IPType:       "ipv4",
				IsAssigned:   false,
				ValidUntil:   refTime.Add(-10 * 24 * time.Hour),
				ExpiryStatus: domain.ExpiryExpired,
			},
		}

		stats := domain.ComputeProxyIPStats(ips, refTime)

		if stats.TotalCount != 5 {
			t.Errorf("TotalCount = %d, want 5", stats.TotalCount)
		}
		if stats.AssignedCount != 3 {
			t.Errorf("AssignedCount = %d, want 3", stats.AssignedCount)
		}
		if stats.AvailableCount != 2 {
			t.Errorf("AvailableCount = %d, want 2", stats.AvailableCount)
		}
		if stats.IPv4Count != 3 {
			t.Errorf("IPv4Count = %d, want 3", stats.IPv4Count)
		}
		if stats.IPv6Count != 2 {
			t.Errorf("IPv6Count = %d, want 2", stats.IPv6Count)
		}
		if stats.ExpiringSoonCount != 2 {
			t.Errorf("ExpiringSoonCount = %d, want 2", stats.ExpiringSoonCount)
		}
		if stats.ExpiredCount != 2 {
			t.Errorf("ExpiredCount = %d, want 2", stats.ExpiredCount)
		}
		if stats.AssignedExpiring != 2 {
			t.Errorf("AssignedExpiring = %d, want 2", stats.AssignedExpiring)
		}
	})

	t.Run("edge case: IP type casing and whitespace normalization", func(t *testing.T) {
		ips := []domain.ProxyIP{
			{IPAddress: "1.1.1.1", IPType: "IPv4", ExpiryStatus: domain.ExpiryActive},
			{IPAddress: "2.2.2.2", IPType: "  ipv4  ", ExpiryStatus: domain.ExpiryActive},
			{IPAddress: "3.3.3.3", IPType: "IPV6", ExpiryStatus: domain.ExpiryActive},
			{IPAddress: "4.4.4.4", IPType: "unknown", ExpiryStatus: domain.ExpiryActive},
		}

		stats := domain.ComputeProxyIPStats(ips, refTime)

		if stats.TotalCount != 4 {
			t.Errorf("TotalCount = %d, want 4", stats.TotalCount)
		}
		if stats.IPv4Count != 2 {
			t.Errorf("IPv4Count = %d, want 2", stats.IPv4Count)
		}
		if stats.IPv6Count != 1 {
			t.Errorf("IPv6Count = %d, want 1", stats.IPv6Count)
		}
	})

	t.Run("edge case: missing ExpiryStatus dynamically computed from ValidUntil", func(t *testing.T) {
		ips := []domain.ProxyIP{
			// ExpiryStatus is empty string, but ValidUntil is 3 days in future -> should detect expiring soon
			{
				IPAddress:         "10.0.0.10",
				IPType:            "ipv4",
				IsAssigned:        true,
				AssignedAccountID: &accountID,
				ValidUntil:        refTime.Add(3 * 24 * time.Hour),
				ExpiryStatus:      "",
			},
			// ExpiryStatus is empty string, ValidUntil is in past -> should detect expired
			{
				IPAddress:    "10.0.0.11",
				IPType:       "ipv4",
				IsAssigned:   false,
				ValidUntil:   refTime.Add(-1 * 24 * time.Hour),
				ExpiryStatus: "",
			},
		}

		stats := domain.ComputeProxyIPStats(ips, refTime)

		if stats.ExpiringSoonCount != 1 {
			t.Errorf("ExpiringSoonCount = %d, want 1", stats.ExpiringSoonCount)
		}
		if stats.ExpiredCount != 1 {
			t.Errorf("ExpiredCount = %d, want 1", stats.ExpiredCount)
		}
		if stats.AssignedExpiring != 1 {
			t.Errorf("AssignedExpiring = %d, want 1", stats.AssignedExpiring)
		}
	})
}
