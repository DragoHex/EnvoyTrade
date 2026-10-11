package kite

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sync"
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

// TransportPool manages persistent, reusable *http.Transport instances keyed by proxy URL.
// It ensures that TCP connections and TLS sessions through dedicated static proxies are kept
// warm and reused across order placement, portfolio syncs, and reconciliation checks.
type TransportPool struct {
	mu         sync.RWMutex
	transports map[string]*http.Transport
}

// DefaultTransportPool is the package-level transport pool shared across the application.
var DefaultTransportPool = NewTransportPool()

// NewTransportPool initializes an empty TransportPool.
func NewTransportPool() *TransportPool {
	return &TransportPool{
		transports: make(map[string]*http.Transport),
	}
}

// GetClient returns an *http.Client configured with a persistent *http.Transport for the given proxy.
func (p *TransportPool) GetClient(cfg ProxyConfig) (*http.Client, error) {
	proxyURL, err := cfg.url()
	if err != nil {
		return nil, fmt.Errorf("bad proxy config: %w", err)
	}

	key := proxyURL.String()

	p.mu.RLock()
	tr, ok := p.transports[key]
	p.mu.RUnlock()
	if ok {
		return &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
		}, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if tr, ok = p.transports[key]; ok {
		return &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
		}, nil
	}

	tr = &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
	p.transports[key] = tr

	return &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}, nil
}

// RESTClientFor builds or reuses an *http.Client that egresses requests
// through this account's dedicated proxy IP using DefaultTransportPool (PLAN.md §3.3).
func RESTClientFor(cfg ProxyConfig) (*http.Client, error) {
	return DefaultTransportPool.GetClient(cfg)
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
