package domain_test

import (
	"testing"

	"envoytrade/internal/domain"
)

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		// Positive cases
		{"TerminalComplete", domain.TerminalComplete, true},
		{"TerminalRejected", domain.TerminalRejected, true},
		{"TerminalCancelled", domain.TerminalCancelled, true},

		// Negative cases
		{"Open", "OPEN", false},
		{"TriggerPending", "TRIGGER PENDING", false},
		{"ModifyPending", "MODIFY PENDING", false},
		{"ValidationPending", "VALIDATION PENDING", false},
		{"PutOrderReqReceived", "PUT ORDER REQ RECEIVED", false},

		// Edge cases
		{"EmptyString", "", false},
		{"LowerCaseComplete", "complete", false},
		{"UnknownStatus", "UNKNOWN", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.IsTerminal(tt.status)
			if got != tt.expected {
				t.Errorf("IsTerminal(%q) = %v; want %v", tt.status, got, tt.expected)
			}
		})
	}
}
