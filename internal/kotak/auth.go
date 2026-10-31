package kotak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite"

	gokotak "github.com/DragoHex/gokotakneo"
)

var (
	// ErrInvalidInput indicates validation failure on input arguments.
	ErrInvalidInput = errors.New("kotak: invalid input")

	mobileDigitsRegex = regexp.MustCompile(`^[0-9]{10}$`)
)

// NormalizeMobileNumber validates and formats an Indian mobile number into canonical +91XXXXXXXXXX format.
func NormalizeMobileNumber(raw string) (string, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	if strings.HasPrefix(cleaned, "+91") {
		digits := strings.TrimPrefix(cleaned, "+91")
		if !mobileDigitsRegex.MatchString(digits) {
			return "", fmt.Errorf("%w: mobile number must contain exactly 10 digits after +91", ErrInvalidInput)
		}
		return cleaned, nil
	}

	if strings.HasPrefix(cleaned, "91") && len(cleaned) == 12 {
		digits := strings.TrimPrefix(cleaned, "91")
		if !mobileDigitsRegex.MatchString(digits) {
			return "", fmt.Errorf("%w: mobile number must contain exactly 10 digits after 91", ErrInvalidInput)
		}
		return "+" + cleaned, nil
	}

	if mobileDigitsRegex.MatchString(cleaned) {
		return "+91" + cleaned, nil
	}

	return "", fmt.Errorf("%w: mobile number %q must be a valid 10-digit Indian number", ErrInvalidInput, raw)
}

// NextKotakExpiry returns the next 06:00:00 AM IST token expiration boundary.
func NextKotakExpiry(now time.Time) time.Time {
	nowIST := now.In(domain.IST)
	today0600 := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 6, 0, 0, 0, domain.IST)
	if nowIST.Before(today0600) {
		return today0600
	}
	return today0600.AddDate(0, 0, 1)
}

// NextRenewerRunTime returns the next 06:30:00 AM IST daily renewal execution time.
func NextRenewerRunTime(now time.Time) time.Time {
	nowIST := now.In(domain.IST)
	today0630 := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 6, 30, 0, 0, domain.IST)
	if nowIST.Before(today0630) {
		return today0630
	}
	return today0630.AddDate(0, 0, 1)
}

// TradeSessionJSON holds the active session credentials and dynamic gateways returned by Kotak Neo.
type TradeSessionJSON struct {
	Token        string `json:"token"`
	Sid          string `json:"sid"`
	BaseURL      string `json:"baseUrl"`
	FeedURL      string `json:"feedUrl"`
	RTURL        string `json:"rtUrl"`
	GreetingName string `json:"greetingName,omitempty"`
	UCC          string `json:"ucc,omitempty"`
}

// HeadlessLoginCredentials contains the parameters required for Kotak Neo 2-step authentication.
type HeadlessLoginCredentials struct {
	ConsumerKey string
	Mobile      string
	UCC         string
	MPIN        string
	TOTPSecret  string
	ProxyURL    string
}

// HeadlessLogin executes Step 1 (TOTP) and Step 2 (MPIN) authentication against Kotak Neo.
func HeadlessLogin(ctx context.Context, creds HeadlessLoginCredentials) (*TradeSessionJSON, error) {
	normMobile, err := NormalizeMobileNumber(creds.Mobile)
	if err != nil {
		return nil, err
	}

	var opts []gokotak.Option
	if strings.TrimSpace(creds.ProxyURL) != "" {
		opts = append(opts, gokotak.WithProxyURL(strings.TrimSpace(creds.ProxyURL)))
	}

	client := gokotak.New(strings.TrimSpace(creds.ConsumerKey), opts...)

	// Generate TOTP for current time
	totpCode, err := kite.GenerateTOTP(creds.TOTPSecret, time.Now())
	if err != nil {
		return nil, fmt.Errorf("generate totp: %w", err)
	}

	// Step 1: Login with TOTP
	viewSession, err := client.LoginTOTP(ctx, normMobile, strings.TrimSpace(creds.UCC), totpCode)
	if err != nil {
		// Clock drift retry with t - 30s
		if driftCode, driftErr := kite.GenerateTOTP(creds.TOTPSecret, time.Now().Add(-30*time.Second)); driftErr == nil {
			viewSession, err = client.LoginTOTP(ctx, normMobile, strings.TrimSpace(creds.UCC), driftCode)
		}
		if err != nil {
			return nil, translateAuthError(err)
		}
	}

	// Step 2: Validate MPIN
	tradeSession, err := client.ValidateMPIN(ctx, viewSession.Token, viewSession.Sid, strings.TrimSpace(creds.MPIN))
	if err != nil {
		return nil, translateAuthError(err)
	}

	session := &TradeSessionJSON{
		Token:        tradeSession.Token,
		Sid:          tradeSession.Sid,
		BaseURL:      tradeSession.BaseURL,
		FeedURL:      tradeSession.FeedURL,
		RTURL:        tradeSession.RTURL,
		GreetingName: viewSession.GreetingName,
		UCC:          creds.UCC,
	}

	return session, nil
}

func translateAuthError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid") || strings.Contains(msg, "401") || strings.Contains(msg, "expired") {
		return fmt.Errorf("%w: %s", domain.ErrAuthExpired, err.Error())
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504") {
		return fmt.Errorf("%w: %s", domain.ErrBrokerUnreachable, err.Error())
	}
	return err
}

// SerializeTradeSession serializes session data to JSON for accounts.access_token storage.
func SerializeTradeSession(sess *TradeSessionJSON) (string, error) {
	bytes, err := json.Marshal(sess)
	if err != nil {
		return "", fmt.Errorf("marshal trade session: %w", err)
	}
	return string(bytes), nil
}

// DeserializeTradeSession parses JSON from accounts.access_token.
func DeserializeTradeSession(raw string) (*TradeSessionJSON, error) {
	var sess TradeSessionJSON
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, fmt.Errorf("unmarshal trade session: %w", err)
	}
	return &sess, nil
}
