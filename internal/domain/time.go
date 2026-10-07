package domain

import "time"

// IST is the Indian Standard Time zone (UTC+05:30).
// Indian equity and commodity markets (NSE, BSE, MCX) operate strictly in IST.
var IST = time.FixedZone("IST", 5*3600+1800)

// FormatIST returns the timestamp formatted as "YYYY-MM-DD HH:MM:SS" in IST.
func FormatIST(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(IST).Format("2006-01-02 15:04:05")
}

// FormatISTTime returns the time component formatted as "HH:MM:SS" in IST.
func FormatISTTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(IST).Format("15:04:05")
}

// FormatISTISO returns the timestamp formatted as RFC3339 in IST ("2006-01-02T15:04:05+05:30").
func FormatISTISO(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(IST).Format(time.RFC3339)
}

// NowIST returns the current wall-clock time in IST.
func NowIST() time.Time {
	return time.Now().In(IST)
}
