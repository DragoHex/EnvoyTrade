package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const defaultRootURI = "ws://localhost:8089"

// Ticker provides WebSocket streaming access for live order updates matching kiteticker.Ticker.
type Ticker struct {
	apiKey      string
	accessToken string
	rootURI     string

	mu        sync.Mutex
	conn      *websocket.Conn
	closed    bool
	wakeClose chan struct{}

	onOrderUpdate func(Order)
	onConnect     func()
	onError       func(error)
	onClose       func(int, string)
}

type textMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// NewTicker creates a new Ticker instance with given apiKey and accessToken.
func NewTicker(apiKey, accessToken string) *Ticker {
	return &Ticker{
		apiKey:      apiKey,
		accessToken: accessToken,
		rootURI:     defaultRootURI,
		wakeClose:   make(chan struct{}),
	}
}

// SetRootURI overrides the default WebSocket root URI.
func (t *Ticker) SetRootURI(rootURI string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rootURI = strings.TrimRight(rootURI, "/")
}

// OnOrderUpdate registers a callback for order updates.
func (t *Ticker) OnOrderUpdate(f func(order Order)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onOrderUpdate = f
}

// OnConnect registers a hook called whenever the WebSocket connects or reconnects.
func (t *Ticker) OnConnect(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onConnect = f
}

// OnError registers a callback invoked on errors.
func (t *Ticker) OnError(f func(err error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onError = f
}

// OnClose registers a callback called when the connection closes.
func (t *Ticker) OnClose(f func(code int, reason string)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onClose = f
}

// Serve starts the connection loop with background context.
func (t *Ticker) Serve() {
	t.ServeWithContext(context.Background())
}

// ServeWithContext runs the WebSocket connection loop until ctx is cancelled or Close() is called.
func (t *Ticker) ServeWithContext(ctx context.Context) {
	backoff := 50 * time.Millisecond
	const maxBackoff = 5 * time.Second

	for {
		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-t.wakeClose:
			return
		default:
		}

		err := t.connectAndRead(ctx)
		if err != nil {
			t.mu.Lock()
			onErr := t.onError
			t.mu.Unlock()
			if onErr != nil {
				onErr(err)
			}
		}

		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-t.wakeClose:
			return
		case <-time.After(backoff):
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (t *Ticker) connectAndRead(ctx context.Context) error {
	t.mu.Lock()
	root := t.rootURI
	t.mu.Unlock()

	u, err := url.Parse(root)
	if err != nil {
		return fmt.Errorf("parse root url: %w", err)
	}

	q := u.Query()
	q.Set("api_key", t.apiKey)
	q.Set("access_token", t.accessToken)
	u.RawQuery = q.Encode()

	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	t.conn = conn
	connectHook := t.onConnect
	t.mu.Unlock()

	if connectHook != nil {
		connectHook()
	}

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			t.mu.Lock()
			closeHook := t.onClose
			t.mu.Unlock()
			if closeHook != nil {
				code := websocket.CloseGoingAway
				reason := ""
				if closeErr, ok := err.(*websocket.CloseError); ok {
					code = closeErr.Code
					reason = closeErr.Text
				}
				closeHook(code, reason)
			}
			return err
		}

		if msgType == websocket.TextMessage {
			t.processTextMessage(data)
		}
	}
}

func (t *Ticker) processTextMessage(data []byte) {
	var msg textMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "order":
		var order Order
		if err := json.Unmarshal(msg.Data, &order); err == nil {
			t.mu.Lock()
			cb := t.onOrderUpdate
			t.mu.Unlock()
			if cb != nil {
				cb(order)
			}
		}
	case "error":
		var errData any
		_ = json.Unmarshal(msg.Data, &errData)
		t.mu.Lock()
		errCb := t.onError
		t.mu.Unlock()
		if errCb != nil {
			errCb(fmt.Errorf("ticker error: %v", errData))
		}
	}
}

// Close gracefully closes the WebSocket connection and terminates the Serve loop.
func (t *Ticker) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.wakeClose)

	var err error
	if t.conn != nil {
		err = t.conn.Close()
	}
	t.mu.Unlock()

	return err
}
