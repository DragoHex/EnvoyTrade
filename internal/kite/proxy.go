package kite

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
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
			MaxIdleConnsPerHost: 4,
		},
		Timeout: 10 * time.Second,
	}, nil
}
