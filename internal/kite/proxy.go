package kite

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// ProxyConfig is what the vendor dashboard hands you per account.
type ProxyConfig struct {
	Scheme       string // "http" or "https"
	Host         string // e.g. "dc46-mum-01.algoip.in"
	Port         int
	ClientID     string
	ClientSecret string
}

func (p ProxyConfig) url() (*url.URL, error) {
	if p.Host == "" {
		return nil, fmt.Errorf("proxy host is required")
	}
	scheme := p.Scheme
	if scheme == "" {
		scheme = "https"
	}
	u := &url.URL{
		Scheme: scheme,
		User:   url.UserPassword(p.ClientID, p.ClientSecret),
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
	}
	return url.Parse(u.String())
}

// RESTClientFor builds an *http.Client that egresses every request
// through this account's dedicated proxy IP (PLAN.md §3.3).
func RESTClientFor(cfg ProxyConfig) (*http.Client, error) {
	proxyURL, err := cfg.url()
	if err != nil {
		return nil, fmt.Errorf("bad proxy config: %w", err)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
			MaxIdleConnsPerHost: 4,
		},
		Timeout: 10 * time.Second,
	}, nil
}

// NewLiveBroker creates a live kite.Broker client for Kite Connect API calls,
// routing through the specified proxy if configured. Mutating calls will be blocked
// if proxyCfg is nil or host is empty.
func NewLiveBroker(apiKey, accessToken string, proxyCfg *ProxyConfig) (*Broker, error) {
	kc := kiteconnect.New(apiKey)
	kc.SetAccessToken(accessToken)
	isProxied := false
	if proxyCfg != nil && proxyCfg.Host != "" {
		httpClient, err := RESTClientFor(*proxyCfg)
		if err != nil {
			return nil, fmt.Errorf("create proxy http client: %w", err)
		}
		kc.SetHTTPClient(httpClient)
		isProxied = true
	}
	return NewBroker(kc, isProxied), nil
}
