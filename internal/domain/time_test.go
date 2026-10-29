package domain_test

import (
	"testing"
	"time"

	"envoytrade/internal/domain"
)

func TestIST_Offset(t *testing.T) {
	now := time.Now().In(domain.IST)
	_, offset := now.Zone()
	wantOffset := 5*3600 + 1800 // +05:30 = 19800 seconds
	if offset != wantOffset {
		t.Fatalf("zone offset = %d, want %d (+05:30)", offset, wantOffset)
	}
}

func TestFormatIST(t *testing.T) {
	// 2026-10-07 03:00:00 UTC = 2026-10-07 08:30:00 IST
	utcTime := time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC)
	formatted := domain.FormatIST(utcTime)
	want := "2026-10-07 08:30:00"
	if formatted != want {
		t.Errorf("FormatIST(%v) = %q, want %q", utcTime, formatted, want)
	}

	if domain.FormatIST(time.Time{}) != "" {
		t.Errorf("FormatIST(zero) = %q, want empty string", domain.FormatIST(time.Time{}))
	}
}

func TestFormatISTTime(t *testing.T) {
	utcTime := time.Date(2026, 10, 7, 3, 45, 12, 0, time.UTC)
	formatted := domain.FormatISTTime(utcTime)
	want := "09:15:12"
	if formatted != want {
		t.Errorf("FormatISTTime(%v) = %q, want %q", utcTime, formatted, want)
	}

	if domain.FormatISTTime(time.Time{}) != "" {
		t.Errorf("FormatISTTime(zero) = %q, want empty string", domain.FormatISTTime(time.Time{}))
	}
}

func TestFormatISTISO(t *testing.T) {
	utcTime := time.Date(2026, 10, 7, 3, 45, 0, 0, time.UTC)
	formatted := domain.FormatISTISO(utcTime)
	want := "2026-10-07T09:15:00+05:30"
	if formatted != want {
		t.Errorf("FormatISTISO(%v) = %q, want %q", utcTime, formatted, want)
	}

	if domain.FormatISTISO(time.Time{}) != "" {
		t.Errorf("FormatISTISO(zero) = %q, want empty string", domain.FormatISTISO(time.Time{}))
	}
}

func TestNowIST(t *testing.T) {
	nowIST := domain.NowIST()
	_, offset := nowIST.Zone()
	if offset != 19800 {
		t.Errorf("NowIST() zone offset = %d, want 19800", offset)
	}
}

func TestParseIST(t *testing.T) {
	tests := []struct {
		input       string
		wantYear    int
		wantMonth   time.Month
		wantDay     int
		wantHour    int
		wantMinute  int
		wantSecond  int
		wantZero    bool
		wantOffset  int
	}{
		{
			input:      "2026-10-29 09:15:30",
			wantYear:   2026,
			wantMonth:  time.October,
			wantDay:    29,
			wantHour:   9,
			wantMinute: 15,
			wantSecond: 30,
			wantOffset: 19800,
		},
		{
			input:      "2026-10-29 09:15:30.500",
			wantYear:   2026,
			wantMonth:  time.October,
			wantDay:    29,
			wantHour:   9,
			wantMinute: 15,
			wantSecond: 30,
			wantOffset: 19800,
		},
		{
			input:      "2026-10-29T09:15:30.000",
			wantYear:   2026,
			wantMonth:  time.October,
			wantDay:    29,
			wantHour:   9,
			wantMinute: 15,
			wantSecond: 30,
			wantOffset: 19800,
		},
		{
			input:      "2026-10-29",
			wantYear:   2026,
			wantMonth:  time.October,
			wantDay:    29,
			wantOffset: 19800,
		},
		{
			input:    "",
			wantZero: true,
		},
		{
			input:    "invalid-time",
			wantZero: true,
		},
	}

	for _, tc := range tests {
		got := domain.ParseIST(tc.input)
		if tc.wantZero {
			if !got.IsZero() {
				t.Errorf("ParseIST(%q) = %v, want zero time", tc.input, got)
			}
			continue
		}
		if got.Year() != tc.wantYear || got.Month() != tc.wantMonth || got.Day() != tc.wantDay ||
			got.Hour() != tc.wantHour || got.Minute() != tc.wantMinute || got.Second() != tc.wantSecond {
			t.Errorf("ParseIST(%q) = %v, want %d-%d-%d %d:%d:%d",
				tc.input, got, tc.wantYear, tc.wantMonth, tc.wantDay, tc.wantHour, tc.wantMinute, tc.wantSecond)
		}
		_, offset := got.Zone()
		if offset != tc.wantOffset {
			t.Errorf("ParseIST(%q) zone offset = %d, want %d", tc.input, offset, tc.wantOffset)
		}
	}
}
