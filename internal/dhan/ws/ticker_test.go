package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"envoytrade/internal/dhan"
	dhanws "envoytrade/internal/dhan/ws"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type fakePublisher[T any] struct {
	published []T
}

func (f *fakePublisher[T]) Publish(ctx context.Context, ev T) error {
	f.published = append(f.published, ev)
	return nil
}

func setupTestResolver(t *testing.T) *dhan.InstrumentResolver {
	t.Helper()
	resolver := dhan.NewInstrumentResolver(nil)
	csvData := `SEM_EXM_EXCH_ID,SEM_SEGMENT,SEM_SMST_SECURITY_ID,SEM_TRADING_SYMBOL,SEM_LOT_UNITS
NSE,E,11536,TATASTEEL,1
NSE,D,53376,NIFTY-Nov2026-21000-PE,65
`
	if err := resolver.LoadFromReader(strings.NewReader(csvData)); err != nil {
		t.Fatalf("setupTestResolver failed: %v", err)
	}
	return resolver
}

func TestMasterTicker_ReceivesOrderAndPublishes(t *testing.T) {
	var (
		receivedClientID string
		receivedToken    string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedClientID = r.Header.Get("client-id")
		receivedToken = r.Header.Get("access-token")

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Send Dhan WebSocket order update frame
		orderPayload := map[string]any{
			"dhanClientId":        "1000000001",
			"orderId":             "dhan-ws-order-001",
			"correlationId":       "tag-ws-1",
			"orderStatus":         "TRADED",
			"transactionType":     "BUY",
			"exchangeSegment":     "NSE_EQ",
			"productType":         "CNC",
			"orderType":           "LIMIT",
			"tradingSymbol":       "TATASTEEL",
			"securityId":          "11536",
			"quantity":            50,
			"filledQty":           50,
			"price":               120.50,
			"averageTradedPrice":  120.50,
		}
		data, _ := json.Marshal(orderPayload)
		_ = conn.WriteMessage(websocket.TextMessage, data)

		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	masterID := uuid.New()
	pub := &fakePublisher[domain.MasterFill]{}
	resolver := setupTestResolver(t)

	ticker := dhanws.NewMasterTicker(
		"1000000001",
		"test-jwt-token",
		masterID,
		pub,
		resolver,
		nil,
		dhanws.WithFeedURL(wsURL),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ticker.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	_ = ticker.Stop()

	if receivedClientID != "1000000001" || receivedToken != "test-jwt-token" {
		t.Errorf("headers mismatch: client-id=%s, token=%s", receivedClientID, receivedToken)
	}

	if len(pub.published) != 1 {
		t.Fatalf("published count = %d, want 1", len(pub.published))
	}
	fill := pub.published[0]
	if fill.MasterID != masterID || fill.BrokerOrderID != "dhan-ws-order-001" || fill.FilledQuantity != 50 {
		t.Errorf("master fill mismatch: %+v", fill)
	}
	if fill.Status != domain.TerminalComplete {
		t.Errorf("status = %s, want %s", fill.Status, domain.TerminalComplete)
	}
}

func TestTickerFactory_CreateTicker(t *testing.T) {
	pub := &fakePublisher[domain.MasterFill]{}
	resolver := setupTestResolver(t)
	factory := dhanws.NewTickerFactory(pub, resolver, nil)

	// 1. Missing credentials returns nil, nil
	tk, err := factory.CreateTicker(context.Background(), uuid.New(), domain.AccountAuthInfo{}, nil)
	if err != nil || tk != nil {
		t.Errorf("expected nil, nil for empty auth, got tk=%v, err=%v", tk, err)
	}

	// 2. Valid credentials returns Ticker instance
	auth := domain.AccountAuthInfo{
		ApiKey:      "1000000001",
		AccessToken: "test-token",
	}
	tk, err = factory.CreateTicker(context.Background(), uuid.New(), auth, nil)
	if err != nil || tk == nil {
		t.Errorf("expected valid ticker, got tk=%v, err=%v", tk, err)
	}
}

func TestMasterTicker_ResolvesSymbolAndExchangeFromSecurityID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Dhan sends an F&O order update
		orderPayload := map[string]any{
			"dhanClientId":        "1000000001",
			"orderId":             "dhan-fno-001",
			"correlationId":       "tag-fno-1",
			"orderStatus":         "TRADED",
			"transactionType":     "BUY",
			"exchangeSegment":     "NSE_FNO",
			"productType":         "INTRADAY",
			"orderType":           "MARKET",
			"tradingSymbol":       "NIFTY-Nov2026-21000-PE",
			"securityId":          "53376",
			"quantity":            65,
			"filledQty":           65,
			"price":               25.50,
			"averageTradedPrice":  25.50,
		}
		data, _ := json.Marshal(orderPayload)
		_ = conn.WriteMessage(websocket.TextMessage, data)

		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	masterID := uuid.New()
	pub := &fakePublisher[domain.MasterFill]{}
	resolver := setupTestResolver(t)

	ticker := dhanws.NewMasterTicker(
		"1000000001",
		"test-jwt-token",
		masterID,
		pub,
		resolver,
		nil,
		dhanws.WithFeedURL(wsURL),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ticker.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	_ = ticker.Stop()

	if len(pub.published) != 1 {
		t.Fatalf("published count = %d, want 1", len(pub.published))
	}
	fill := pub.published[0]
	if fill.Exchange != "NFO" {
		t.Errorf("exchange = %s, want NFO", fill.Exchange)
	}
	if fill.Tradingsymbol != "NIFTY-Nov2026-21000-PE" {
		t.Errorf("tradingsymbol = %s, want NIFTY-Nov2026-21000-PE", fill.Tradingsymbol)
	}
	if fill.FilledQuantity != 65 {
		t.Errorf("filledQuantity = %d, want 65", fill.FilledQuantity)
	}
}
