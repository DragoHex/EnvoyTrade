package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// DefaultRootURL is the default Zerodha Kite Connect WebSocket URL.
const DefaultRootURL = "wss://ws.kite.trade"

// Config configures a WebSocket connection.
type Config struct {
	APIKey      string
	AccessToken string
	RootURL     string // defaults to DefaultRootURL
}

type textMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Conn provides an isolated, direct WebSocket connection to Zerodha Kite Connect.
// It fulfills callback.Conn and callback.ConnectNotifier without mutating global dialer state.
type Conn struct {
	cfg           Config
	logger        *slog.Logger
	orderCallback func(kiteconnect.Order)
	connectHook   func()

	mu        sync.Mutex
	activeWS  *websocket.Conn
	closed    bool
	wakeClose chan struct{}
}

// NewConn creates a new isolated WebSocket connection client.
func NewConn(cfg Config, logger *slog.Logger) *Conn {
	if cfg.RootURL == "" {
		cfg.RootURL = DefaultRootURL
	}
	return &Conn{
		cfg:       cfg,
		logger:    logger,
		wakeClose: make(chan struct{}),
	}
}

func (c *Conn) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// OnOrderUpdate registers the callback for terminal/non-terminal order events.
func (c *Conn) OnOrderUpdate(f func(order kiteconnect.Order)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orderCallback = f
}

// OnConnect registers an optional hook called when a WebSocket session connects or reconnects.
func (c *Conn) OnConnect(f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connectHook = f
}

// ServeWithContext runs the connection loop until ctx is cancelled or Close() is called.
func (c *Conn) ServeWithContext(ctx context.Context) {
	backoff := 1 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-c.wakeClose:
			return
		default:
		}

		err := c.connectAndRead(ctx)
		if err != nil {
			c.log().Warn("ws: connection closed or failed", "error", err)
		}

		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-c.wakeClose:
			return
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (c *Conn) connectAndRead(ctx context.Context) error {
	u, err := url.Parse(c.cfg.RootURL)
	if err != nil {
		return fmt.Errorf("parse root url: %w", err)
	}

	q := u.Query()
	q.Set("api_key", c.cfg.APIKey)
	q.Set("access_token", c.cfg.AccessToken)
	u.RawQuery = q.Encode()

	// Dedicated dialer instance per connection — never mutates global websocket.DefaultDialer
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	c.activeWS = conn
	hook := c.connectHook
	c.mu.Unlock()

	if hook != nil {
		hook()
	}

	// Read loop
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		if msgType == websocket.TextMessage {
			c.processTextMessage(data)
		}
	}
}

func (c *Conn) processTextMessage(data []byte) {
	var msg textMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "order":
		var order kiteconnect.Order
		if err := json.Unmarshal(msg.Data, &order); err != nil {
			return
		}

		c.mu.Lock()
		cb := c.orderCallback
		c.mu.Unlock()

		if cb != nil {
			cb(order)
		}
	case "error":
		c.log().Warn("ws: received error from broker", "payload", string(msg.Data))
	}
}

// Close gracefully closes the active connection and stops reconnection attempts.
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.wakeClose)

	var err error
	if c.activeWS != nil {
		err = c.activeWS.Close()
	}
	c.mu.Unlock()

	return err
}
