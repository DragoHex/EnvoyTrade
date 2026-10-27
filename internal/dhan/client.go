package dhan

import (
	"net/http"
	"time"

	"github.com/DragoHex/godhan"
)

// DefaultHTTPTransport returns a production-tuned *http.Transport with ForceAttemptHTTP2: true.
// ForceAttemptHTTP2 ensures that Go's transport properly activates HTTP/2 ALPN handling
// and transparently frames requests/responses when routed through HTTPS forward proxies.
func DefaultHTTPTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// DefaultHTTPClient constructs a standard HTTP client with DefaultHTTPTransport and a 15-second timeout.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: DefaultHTTPTransport(),
	}
}

// BuildClientOptions constructs godhan functional options with a ForceAttemptHTTP2-enabled
// base HTTP transport, ensuring proper HTTP/2 ALPN handling even when proxied through forward proxies.
func BuildClientOptions(proxyCfg *ProxyConfig, extraOpts ...godhan.Option) []godhan.Option {
	opts := []godhan.Option{
		godhan.WithHTTPClient(DefaultHTTPClient()),
	}
	if proxyCfg != nil && proxyCfg.Host != "" {
		opts = append(opts, godhan.WithProxy(*proxyCfg))
	}
	opts = append(opts, extraOpts...)
	return opts
}
