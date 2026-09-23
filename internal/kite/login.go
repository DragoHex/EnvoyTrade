package kite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// LoginConfig allows customizing endpoints for testing.
type LoginConfig struct {
	KiteWebBaseURL string // defaults to "https://kite.zerodha.com"
	KiteAPIBaseURL string // defaults to "https://api.kite.trade"
}

type kiteAPIResponse[T any] struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	ErrorType string `json:"error_type"`
	Data      T      `json:"data"`
}

type loginData struct {
	RequestID  string   `json:"request_id"`
	TwoFAType  string   `json:"twofa_type"`
	TwoFATypes []string `json:"twofa_types"`
}

type twoFAData struct {
	UserID       string `json:"user_id"`
	RequestToken string `json:"request_token"`
}

type tokenData struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"

// HeadlessLogin logs into Zerodha Kite headlessly using username, password, TOTP secret,
// API key, and API secret to retrieve a fresh Kite Connect access_token.
func HeadlessLogin(ctx context.Context, httpClient *http.Client, userID, password, totpSecret, apiKey, apiSecret string) (string, error) {
	return HeadlessLoginWithConfig(ctx, httpClient, userID, password, totpSecret, apiKey, apiSecret, LoginConfig{
		KiteWebBaseURL: "https://kite.zerodha.com",
		KiteAPIBaseURL: "https://api.kite.trade",
	})
}

// HeadlessLoginWithConfig performs the 5-step automated authentication against configurable base URLs.
func HeadlessLoginWithConfig(
	ctx context.Context,
	httpClient *http.Client,
	userID, password, totpSecret, apiKey, apiSecret string,
	cfg LoginConfig,
) (string, error) {
	if cfg.KiteWebBaseURL == "" {
		cfg.KiteWebBaseURL = "https://kite.zerodha.com"
	}
	if cfg.KiteAPIBaseURL == "" {
		cfg.KiteAPIBaseURL = "https://api.kite.trade"
	}

	client := httpClient
	if client == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return "", fmt.Errorf("init cookie jar: %w", err)
		}
		client = &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
		}
	} else if client.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return "", fmt.Errorf("init cookie jar: %w", err)
		}
		client.Jar = jar
	}

	// Step 0: Initialize connect login session to obtain sess_id and session cookies
	var sessID string
	initURL := fmt.Sprintf("%s/connect/login?api_key=%s&v=3", strings.TrimRight(cfg.KiteWebBaseURL, "/"), url.QueryEscape(apiKey))
	if initReq, err := http.NewRequestWithContext(ctx, http.MethodGet, initURL, nil); err == nil {
		initReq.Header.Set("User-Agent", browserUserAgent)
		initClient := &http.Client{
			Jar:       client.Jar,
			Transport: client.Transport,
			Timeout:   client.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if sid := req.URL.Query().Get("sess_id"); sid != "" {
					sessID = sid
				}
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
		if initResp, err := initClient.Do(initReq); err == nil {
			_ = initResp.Body.Close()
			if sessID == "" {
				if loc := initResp.Header.Get("Location"); loc != "" {
					if parsedLoc, err := url.Parse(loc); err == nil {
						sessID = parsedLoc.Query().Get("sess_id")
					}
				}
			}
			if sessID == "" && initResp.Request != nil && initResp.Request.URL != nil {
				sessID = initResp.Request.URL.Query().Get("sess_id")
			}
		}
	}

	// Step 1: POST /api/login with user_id and password
	loginVals := url.Values{}
	loginVals.Set("user_id", userID)
	loginVals.Set("password", password)

	loginReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(cfg.KiteWebBaseURL, "/")+"/api/login",
		strings.NewReader(loginVals.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("create login request: %w", err)
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", browserUserAgent)

	loginResp, err := client.Do(loginReq)
	if err != nil {
		return "", fmt.Errorf("kite web login: %w", err)
	}
	defer loginResp.Body.Close()

	var loginResult kiteAPIResponse[loginData]
	if err := json.NewDecoder(loginResp.Body).Decode(&loginResult); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if loginResult.Status != "success" || loginResult.Data.RequestID == "" {
		errMsg := loginResult.Message
		if errMsg == "" {
			errMsg = "login failed"
		}
		return "", fmt.Errorf("kite login error: %s", errMsg)
	}
	requestID := loginResult.Data.RequestID

	// Step 2: Compute current 6-digit TOTP
	totpCode, err := GenerateTOTP(totpSecret, time.Now())
	if err != nil {
		return "", fmt.Errorf("generate totp: %w", err)
	}

	twofaType := loginResult.Data.TwoFAType
	if twofaType == "" && len(loginResult.Data.TwoFATypes) > 0 {
		twofaType = loginResult.Data.TwoFATypes[0]
	}
	if twofaType == "" {
		twofaType = "totp"
	}

	// Step 3: POST /api/twofa with request_id, user_id, and totp
	twoFAVals := url.Values{}
	twoFAVals.Set("user_id", userID)
	twoFAVals.Set("request_id", requestID)
	twoFAVals.Set("twofa_value", totpCode)
	twoFAVals.Set("twofa_type", twofaType)
	twoFAVals.Set("skip_session", "true")

	twoFAReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(cfg.KiteWebBaseURL, "/")+"/api/twofa",
		strings.NewReader(twoFAVals.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("create twofa request: %w", err)
	}
	twoFAReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	twoFAReq.Header.Set("User-Agent", browserUserAgent)

	twoFAResp, err := client.Do(twoFAReq)
	if err != nil {
		return "", fmt.Errorf("kite twofa request: %w", err)
	}
	defer twoFAResp.Body.Close()

	var twoFAResult kiteAPIResponse[twoFAData]
	if err := json.NewDecoder(twoFAResp.Body).Decode(&twoFAResult); err != nil {
		return "", fmt.Errorf("decode twofa response: %w", err)
	}
	if twoFAResult.Status != "success" {
		errMsg := twoFAResult.Message
		if errMsg == "" {
			errMsg = "twofa verification failed"
		}
		if strings.Contains(strings.ToLower(errMsg), "2fa type is not available") {
			return "", fmt.Errorf("kite 2fa error: %s (External TOTP is not enabled on this Zerodha account. Please open Kite Profile -> Password & Security -> Enable External TOTP and use that secret key)", errMsg)
		}
		return "", fmt.Errorf("kite 2fa error: %s", errMsg)
	}

	requestToken := twoFAResult.Data.RequestToken
	if requestToken == "" {
		// Step 4: Extract request_token by completing authorization and following redirect
		var publicToken string
		if parsedURL, err := url.Parse(cfg.KiteWebBaseURL); err == nil && client.Jar != nil {
			for _, cookie := range client.Jar.Cookies(parsedURL) {
				if cookie.Name == "public_token" {
					publicToken = cookie.Value
					break
				}
			}
		}

		noRedirectClient := &http.Client{
			Jar:       client.Jar,
			Transport: client.Transport,
			Timeout:   client.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if token := req.URL.Query().Get("request_token"); token != "" {
					requestToken = token
					return http.ErrUseLastResponse
				}
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				return nil
			},
		}

		// Strategy 1: Call /api/connect/app/authorize (internal Kite API)
		authVals := map[string]string{
			"api_key": apiKey,
			"sess_id": sessID,
		}
		if authJSON, err := json.Marshal(authVals); err == nil {
			if authReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.KiteWebBaseURL, "/")+"/api/connect/app/authorize", bytes.NewReader(authJSON)); err == nil {
				authReq.Header.Set("Content-Type", "application/json")
				authReq.Header.Set("User-Agent", browserUserAgent)
				if authResp, err := client.Do(authReq); err == nil {
					_ = authResp.Body.Close()
				}
			}
		}

		// Strategy 2: Form POST to /connect/finish with sess_id, api_key, authorize
		finishVals := url.Values{}
		finishVals.Set("api_key", apiKey)
		if sessID != "" {
			finishVals.Set("sess_id", sessID)
		}
		if publicToken != "" {
			finishVals.Set("authorize", publicToken)
		} else {
			finishVals.Set("authorize", "true")
		}
		if finishReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.KiteWebBaseURL, "/")+"/connect/finish", strings.NewReader(finishVals.Encode())); err == nil {
			finishReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			finishReq.Header.Set("User-Agent", browserUserAgent)
			if finishResp, err := noRedirectClient.Do(finishReq); err == nil {
				defer finishResp.Body.Close()
				if requestToken == "" {
					if loc := finishResp.Header.Get("Location"); loc != "" {
						if parsedLoc, err := url.Parse(loc); err == nil {
							requestToken = parsedLoc.Query().Get("request_token")
						}
					}
				}
				if requestToken == "" && finishResp.Request != nil && finishResp.Request.URL != nil {
					requestToken = finishResp.Request.URL.Query().Get("request_token")
				}
				if requestToken == "" {
					bodyBytes, _ := io.ReadAll(finishResp.Body)
					if len(bodyBytes) > 0 {
						re := regexp.MustCompile(`request_token=([a-zA-Z0-9]+)`)
						if m := re.FindSubmatch(bodyBytes); len(m) > 1 {
							requestToken = string(m[1])
						}
					}
				}
			}
		}

		// Strategy 3: GET /connect/login with sess_id & skip_session=true
		if requestToken == "" {
			connectVals := url.Values{}
			connectVals.Set("api_key", apiKey)
			if sessID != "" {
				connectVals.Set("sess_id", sessID)
			}
			connectVals.Set("skip_session", "true")
			connectURL := fmt.Sprintf("%s/connect/login?%s", strings.TrimRight(cfg.KiteWebBaseURL, "/"), connectVals.Encode())
			connectReq, err := http.NewRequestWithContext(ctx, http.MethodGet, connectURL, nil)
			if err != nil {
				return "", fmt.Errorf("create connect login request: %w", err)
			}
			connectReq.Header.Set("User-Agent", browserUserAgent)

			connectResp, err := noRedirectClient.Do(connectReq)
			if err != nil {
				return "", fmt.Errorf("connect login request: %w", err)
			}
			defer connectResp.Body.Close()

			if requestToken == "" {
				location := connectResp.Header.Get("Location")
				if location != "" {
					if parsedLoc, err := url.Parse(location); err == nil {
						requestToken = parsedLoc.Query().Get("request_token")
					}
				}
			}
			if requestToken == "" && connectResp.Request != nil && connectResp.Request.URL != nil {
				requestToken = connectResp.Request.URL.Query().Get("request_token")
			}

			bodyBytes, _ := io.ReadAll(connectResp.Body)
			if requestToken == "" && len(bodyBytes) > 0 {
				re := regexp.MustCompile(`request_token=([a-zA-Z0-9]+)`)
				if m := re.FindSubmatch(bodyBytes); len(m) > 1 {
					requestToken = string(m[1])
				}
			}

			if requestToken == "" {
				var errResp kiteAPIResponse[any]
				if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Message != "" {
					return "", fmt.Errorf("connect login failed: %s", errResp.Message)
				}
				preview := string(bodyBytes)
				if len(preview) > 200 {
					preview = preview[:200]
				}
				return "", fmt.Errorf("missing request_token from connect/login (HTTP %d, sess_id=%s, body: %q)", connectResp.StatusCode, sessID, preview)
			}
		}
	}

	// Step 5: Exchange request_token for access_token: POST /session/token
	checksumHash := sha256.Sum256([]byte(apiKey + requestToken + apiSecret))
	checksum := hex.EncodeToString(checksumHash[:])

	tokenVals := url.Values{}
	tokenVals.Set("api_key", apiKey)
	tokenVals.Set("request_token", requestToken)
	tokenVals.Set("checksum", checksum)

	tokenReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(cfg.KiteAPIBaseURL, "/")+"/session/token",
		strings.NewReader(tokenVals.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("User-Agent", browserUserAgent)

	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("session token request: %w", err)
	}
	defer tokenResp.Body.Close()

	var tokenResult kiteAPIResponse[tokenData]
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenResult); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResult.Status != "success" || tokenResult.Data.AccessToken == "" {
		errMsg := tokenResult.Message
		if errMsg == "" {
			errMsg = "token exchange failed"
		}
		return "", fmt.Errorf("token exchange error: %s", errMsg)
	}

	return tokenResult.Data.AccessToken, nil
}
