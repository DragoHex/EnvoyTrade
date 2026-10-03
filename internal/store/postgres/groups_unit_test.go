package postgres

import "testing"

func TestStatusFromAccountStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "active", want: "ok"},
		{input: "ok", want: "ok"},
		{input: "error", want: "error"},
		{input: "paused", want: "error"},
		{input: "disabled", want: "error"},
		{input: "", want: "error"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := statusFromAccountStatus(tc.input)
			if got != tc.want {
				t.Errorf("statusFromAccountStatus(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
