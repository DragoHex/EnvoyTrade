package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrIPAlreadyAssigned is returned when attempting to assign a proxy IP
// that is already bound to another account (enforcing 1:1 assignment).
var ErrIPAlreadyAssigned = errors.New("domain: IP address is already assigned to another account")

// ProxyIP models a dedicated static proxy IP from the AlgoIP pool for
// Kite Connect broker egress (PLAN.md §3.3, docs/APIs/proxy_ips.md).
type ProxyIP struct {
	IPAddress           string     `json:"ipAddress"`
	IPType              string     `json:"ipType"` // "ipv4" or "ipv6"
	Host                string     `json:"host"`
	Port                int        `json:"port"`
	Username            string     `json:"-"`
	Password            string     `json:"-"`
	ValidFrom           time.Time  `json:"validFrom"`
	ValidUntil          time.Time  `json:"validUntil"`
	Plan                string     `json:"plan"`
	CreatedAt           time.Time  `json:"createdAt"`
	IsAssigned          bool       `json:"isAssigned"`
	AssignedAccountID   *uuid.UUID `json:"assignedAccountId,omitempty"`
	AssignedAccountName *string    `json:"assignedAccountName,omitempty"`
}
