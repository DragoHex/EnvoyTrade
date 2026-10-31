package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/domain"
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
}
