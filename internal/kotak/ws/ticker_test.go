package ws_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kotak"
	"envoytrade/internal/kotak/ws"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

var upgrader = websocket.Upgrader{}

type mockFillQueue struct {
	mu        sync.Mutex
	published []domain.MasterFill
}

func (m *mockFillQueue) Publish(ctx context.Context, item domain.MasterFill) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, item)
	return nil
}

func (m *mockFillQueue) Close() error {
	return nil
}

func TestMasterTicker_Execution(t *testing.T) {
	t.Run("Positive: Order Execution Received and Published", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			// Read handshake
			_, _, _ = conn.ReadMessage()

			// Send connected ack
			_ = conn.WriteJSON(map[string]string{
				"type": "cn",
				"ak":   "ok",
			})

			// Send non-terminal order (should be dropped)
			_ = conn.WriteJSON(map[string]interface{}{
				"type": "order",
				"data": map[string]interface{}{
					"nOrdNo": "260216000001",
					"ordSt":  "OPEN",
				},
			})

			// Send terminal order (should be published)
			_ = conn.WriteJSON(map[string]interface{}{
				"type": "order",
				"data": map[string]interface{}{
					"nOrdNo":  "260216000002",
					"trdSym":  "NIFTY26FEB24000CE",
					"exSeg":   "nse_fo",
					"trnsTp":  "B",
					"prod":    "NRML",
					"prcTp":   "L",
					"ordSt":   "COMPLETE",
					"qty":     50,
					"fldQty":  50,
					"avgPrc":  "125.50",
					"ordDtTm": "16-Feb-2026 12:29:31",
				},
			})

			// Hold connection open
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		queue := &mockFillQueue{}
		masterID := uuid.New()

		ticker := ws.NewMasterTicker(
			masterID,
			"trade-tok",
			"trade-sid",
			wsURL,
			queue,
			nil,
			nil,
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticker.Start(ctx)

		// Wait for publication
		var published domain.MasterFill
		for i := 0; i < 50; i++ {
			queue.mu.Lock()
			count := len(queue.published)
			if count > 0 {
				published = queue.published[0]
			}
			queue.mu.Unlock()
			if count > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if published.BrokerOrderID != "260216000002" {
			t.Fatalf("expected order 260216000002, got %+v", published)
		}
		if published.Exchange != "NFO" || published.TransactionType != "BUY" || published.Product != "NRML" {
			t.Errorf("mismatched fill fields: %+v", published)
		}
		if !published.AveragePrice.Equal(decimal.NewFromFloat(125.50)) {
			t.Errorf("AveragePrice = %s, want 125.50", published.AveragePrice)
		}

		_ = ticker.Stop()
	})

	t.Run("Positive: MCX Commodity Order Fill Normalized To Lots", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_, _, _ = conn.ReadMessage()
			_ = conn.WriteJSON(map[string]string{"type": "cn", "ak": "ok"})

			// Send MCX Crude Oil terminal execution: 100 barrels -> 1 lot
			_ = conn.WriteJSON(map[string]interface{}{
				"type": "order",
				"data": map[string]interface{}{
					"nOrdNo":  "261102000496969",
					"trdSym":  "CRUDEOIL17NOV265000PE",
					"exSeg":   "mcx_fo",
					"trnsTp":  "B",
					"prod":    "NRML",
					"prcTp":   "L",
					"ordSt":   "COMPLETE",
					"qty":     100,
					"fldQty":  100,
					"avgPrc":  "6.80",
					"ordDtTm": "02-Nov-2026 18:02:49",
				},
			})

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		queue := &mockFillQueue{}
		masterID := uuid.New()

		ticker := ws.NewMasterTicker(
			masterID,
			"trade-tok",
			"trade-sid",
			wsURL,
			queue,
			nil,
			nil,
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticker.Start(ctx)

		var published domain.MasterFill
		for i := 0; i < 50; i++ {
			queue.mu.Lock()
			count := len(queue.published)
			if count > 0 {
				published = queue.published[0]
			}
			queue.mu.Unlock()
			if count > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		if published.BrokerOrderID != "261102000496969" {
			t.Fatalf("expected order 261102000496969, got %+v", published)
		}
		if published.Exchange != "MCX" {
			t.Errorf("Exchange = %q, want 'MCX'", published.Exchange)
		}
		// Quantity must be normalized to standard lots (100 barrels -> 1 lot)
		if published.FilledQuantity != 1 {
			t.Errorf("FilledQuantity = %d, want 1 (lot)", published.FilledQuantity)
		}

		_ = ticker.Stop()
	})

	t.Run("Positive: Lowercase and Traded Status Captured by WebSocket", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_, _, _ = conn.ReadMessage()
			_ = conn.WriteJSON(map[string]string{"type": "cn", "ak": "ok"})

			// Send non-terminal lowercase open (should be dropped)
			_ = conn.WriteJSON(map[string]interface{}{
				"type": "order",
				"data": map[string]interface{}{
					"nOrdNo": "260216000010",
					"ordSt":  "open",
				},
			})

			// Send lowercase "complete" order (should be published)
			_ = conn.WriteJSON(map[string]interface{}{
				"type": "order",
				"data": map[string]interface{}{
					"nOrdNo":  "260216000011",
					"trdSym":  "CRUDEOIL17NOV265000PE",
					"exSeg":   "mcx_fo",
					"trnsTp":  "B",
					"prod":    "NRML",
					"prcTp":   "L",
					"ordSt":   "complete",
					"qty":     100,
					"fldQty":  100,
					"avgPrc":  "6.80",
					"ordDtTm": "02-Nov-2026 18:49:38",
				},
			})

			// Send "traded" order (should be published as COMPLETE)
			_ = conn.WriteJSON(map[string]interface{}{
				"type": "order",
				"data": map[string]interface{}{
					"nOrdNo":  "260216000012",
					"trdSym":  "CRUDEOIL17NOV265000PE",
					"exSeg":   "mcx_fo",
					"trnsTp":  "S",
					"prod":    "NRML",
					"prcTp":   "L",
					"ordSt":   "traded",
					"qty":     100,
					"fldQty":  100,
					"avgPrc":  "6.70",
					"ordDtTm": "02-Nov-2026 18:49:42",
				},
			})

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}))
		defer server.Close()

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		queue := &mockFillQueue{}
		masterID := uuid.New()

		ticker := ws.NewMasterTicker(
			masterID,
			"trade-tok",
			"trade-sid",
			wsURL,
			queue,
			nil,
			nil,
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticker.Start(ctx)

		// Wait for both published fills
		for i := 0; i < 50; i++ {
			queue.mu.Lock()
			count := len(queue.published)
			queue.mu.Unlock()
			if count >= 2 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}

		queue.mu.Lock()
		defer queue.mu.Unlock()
		if len(queue.published) != 2 {
			t.Fatalf("expected 2 published fills, got %d", len(queue.published))
		}

		fill1 := queue.published[0]
		if fill1.BrokerOrderID != "260216000011" || fill1.Status != "COMPLETE" {
			t.Errorf("fill1: order=%s, status=%s, want COMPLETE", fill1.BrokerOrderID, fill1.Status)
		}

		fill2 := queue.published[1]
		if fill2.BrokerOrderID != "260216000012" || fill2.Status != "COMPLETE" {
			t.Errorf("fill2: order=%s, status=%s, want COMPLETE", fill2.BrokerOrderID, fill2.Status)
		}

		_ = ticker.Stop()
	})
}

type mockProxyResolver struct {
	ip    string
	proxy domain.ProxyIP
}

func (m *mockProxyResolver) ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error) {
	if ip == m.ip {
		return m.proxy, nil
	}
	return domain.ProxyIP{}, domain.ErrNotFound
}

func TestTickerFactory_CreateTicker(t *testing.T) {
	queue := &mockFillQueue{}
	factory := ws.NewMasterTickerFactory(queue, nil, nil)
	masterID := uuid.New()

	t.Run("Positive: Empty AccessToken returns nil, nil to skip", func(t *testing.T) {
		auth := domain.AccountAuthInfo{
			BrokerAccountID: "UCC123",
			AccessToken:     "",
		}
		ticker, err := factory.CreateTicker(context.Background(), masterID, auth, nil)
		if err != nil {
			t.Fatalf("expected nil error on empty token, got %v", err)
		}
		if ticker != nil {
			t.Fatalf("expected nil ticker on empty token, got %v", ticker)
		}
	})

	t.Run("Negative: Invalid JSON token returns ErrAuthExpired", func(t *testing.T) {
		auth := domain.AccountAuthInfo{
			BrokerAccountID: "UCC123",
			AccessToken:     "not-a-json-session",
		}
		ticker, err := factory.CreateTicker(context.Background(), masterID, auth, nil)
		if err == nil || !errors.Is(err, domain.ErrAuthExpired) {
			t.Fatalf("expected ErrAuthExpired, got %v", err)
		}
		if ticker != nil {
			t.Fatalf("expected nil ticker, got %v", ticker)
		}
	})

	t.Run("Positive: Valid session creates MasterTicker with optional proxy resolver", func(t *testing.T) {
		sess := &kotak.TradeSessionJSON{
			Token: "valid-token",
			Sid:   "valid-sid",
			RTURL: "wss://mock.kotakneo.com/stream",
		}
		sessRaw, _ := kotak.SerializeTradeSession(sess)

		proxyResolver := &mockProxyResolver{
			ip: "10.0.0.1",
			proxy: domain.ProxyIP{
				Host: "192.168.1.1",
				Port: 8080,
			},
		}
		factory.SetProxyResolver(proxyResolver)

		auth := domain.AccountAuthInfo{
			BrokerAccountID: "UCC123",
			AccessToken:     sessRaw,
			IPAddress:       "10.0.0.1",
		}

		ticker, err := factory.CreateTicker(context.Background(), masterID, auth, nil)
		if err != nil {
			t.Fatalf("CreateTicker failed: %v", err)
		}
		if ticker == nil {
			t.Fatal("expected non-nil MasterTicker, got nil")
		}
	})

	t.Run("Positive: Reconnection triggers catch-up hook only after successful reconnect", func(t *testing.T) {
		var mu sync.Mutex
		var activeSockets []*websocket.Conn
		var connCount int
		dropFirstConn := make(chan struct{})

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}

			mu.Lock()
			connCount++
			currentCount := connCount
			activeSockets = append(activeSockets, conn)
			mu.Unlock()

			// Read handshake
			_, _, _ = conn.ReadMessage()

			// Send ACK
			_ = conn.WriteJSON(map[string]string{
				"ak":   "ok",
				"type": "cn",
				"msg":  "connected",
			})

			if currentCount == 1 {
				<-dropFirstConn
				conn.Close()
				return
			}

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					break
				}
			}
		}))
		defer server.Close()
		defer func() {
			mu.Lock()
			for _, s := range activeSockets {
				_ = s.Close()
			}
			mu.Unlock()
		}()

		var catchUpCalls int32
		hookDone := make(chan struct{}, 1)

		q := &mockFillQueue{}
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		ticker := ws.NewMasterTicker(masterID, "token", "sid", wsURL, q, nil, nil)

		ticker.SetCatchUpHook(func(ctx context.Context) error {
			atomic.AddInt32(&catchUpCalls, 1)
			select {
			case hookDone <- struct{}{}:
			default:
			}
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticker.Start(ctx)
		defer ticker.Stop()

		// Wait briefly after initial connect
		time.Sleep(100 * time.Millisecond)

		// On initial connect, catch-up hook must NOT be invoked
		if calls := atomic.LoadInt32(&catchUpCalls); calls != 0 {
			t.Fatalf("expected catchUpCalls == 0 on initial connect, got %d", calls)
		}

		// Trigger disconnect of the first connection
		close(dropFirstConn)

		// Wait for reconnection and catch-up hook invocation
		select {
		case <-hookDone:
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for catch-up hook invocation upon reconnection")
		}

		if calls := atomic.LoadInt32(&catchUpCalls); calls != 1 {
			t.Errorf("expected catchUpCalls == 1, got %d", calls)
		}
	})
}
