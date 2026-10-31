package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"
	"envoytrade/internal/ticker"

	dhanticker "github.com/DragoHex/godhan/ticker"
	"github.com/google/uuid"
)

// SymbolResolver maps Dhan's (exchangeSegment, securityID) to canonical (exchange, tradingsymbol).
type SymbolResolver interface {
	ResolveToSymbol(exchangeSegment, securityID string) (exchange string, tradingsymbol string, err error)
}

// MasterTickerOption configures MasterTicker behavior.
type MasterTickerOption func(*MasterTicker)

// WithFeedURL overrides the default WebSocket endpoint for testing or private clusters.
func WithFeedURL(feedURL string) MasterTickerOption {
	return func(t *MasterTicker) {
		t.feedURL = feedURL
	}
}

// MasterTicker manages a direct WebSocket connection to Dhan's real-time feed for a master account.
type MasterTicker struct {
	clientID    string
	accessToken string
	masterID    uuid.UUID
	queue       queue.Publisher[domain.MasterFill]
	resolver    SymbolResolver
	catchUp     func(context.Context) error
	logger      *slog.Logger
	feedURL     string

	client *dhanticker.Ticker
	cancel context.CancelFunc
	mu     sync.Mutex
}

// NewMasterTicker creates a new initialized MasterTicker.
func NewMasterTicker(
	clientID string,
	accessToken string,
	masterID uuid.UUID,
	queue queue.Publisher[domain.MasterFill],
	resolver SymbolResolver,
	logger *slog.Logger,
	opts ...MasterTickerOption,
) *MasterTicker {
	t := &MasterTicker{
		clientID:    clientID,
		accessToken: accessToken,
		masterID:    masterID,
		queue:       queue,
		resolver:    resolver,
		logger:      logger,
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

func (t *MasterTicker) log() *slog.Logger {
	if t.logger != nil {
		return t.logger
	}
	return slog.Default()
}

// SetCatchUpHook sets the reconciliation hook invoked upon WebSocket reconnection.
func (t *MasterTicker) SetCatchUpHook(hook func(context.Context) error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.catchUp = hook
}

// Start initiates the WebSocket reader and reconnect watchdog loops in the background.
func (t *MasterTicker) Start(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var tickerOpts []dhanticker.TickerOption
	if t.feedURL != "" {
		tickerOpts = append(tickerOpts, dhanticker.WithTickerURI(t.feedURL))
	}

	client := dhanticker.NewTicker(t.clientID, t.accessToken, tickerOpts...)
	t.client = client

	client.OnConnect(func() {
		t.log().Info("dhan ws: master ticker connected", "master_id", t.masterID)
	})

	client.OnOrderUpdate(func(update dhanticker.OrderUpdate) {
		t.handleOrderUpdate(update)
	})

	client.OnReconnect(func(attempt int, delay time.Duration) {
		t.log().Info("dhan ws: master ticker reconnected", "master_id", t.masterID, "attempt", attempt)
		t.mu.Lock()
		hook := t.catchUp
		t.mu.Unlock()
		if hook != nil {
			go func() {
				if err := hook(context.Background()); err != nil {
					t.log().Warn("dhan ws: catch-up hook failed", "error", err)
				}
			}()
		}
	})

	client.OnError(func(err error) {
		t.log().Warn("dhan ws: ticker error", "error", err)
	})

	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	go func() {
		_ = client.ServeWithContext(runCtx)
	}()
}

// Stop closes the WebSocket connection and terminates the reader goroutine.
func (t *MasterTicker) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}
	if t.client != nil {
		_ = t.client.Close()
	}
	return nil
}

func (t *MasterTicker) handleOrderUpdate(update dhanticker.OrderUpdate) {
	status := strings.ToUpper(strings.TrimSpace(update.OrderStatus))
	var canonicalStatus string
	switch status {
	case "TRADED":
		canonicalStatus = domain.TerminalComplete
	case "REJECTED":
		canonicalStatus = domain.TerminalRejected
	case "CANCELLED", "CANCELED":
		canonicalStatus = domain.TerminalCancelled
	default:
		// Drop non-terminal intermediate updates (e.g. TRANSIT, PENDING)
		return
	}

	var exchange, symbol string
	if t.resolver != nil && update.ExchangeSegment != "" && update.SecurityID != "" {
		if ex, sym, err := t.resolver.ResolveToSymbol(update.ExchangeSegment, update.SecurityID); err == nil {
			exchange = ex
			symbol = sym
		}
	}
	if symbol == "" {
		symbol = update.TradingSymbol
	}
	if exchange == "" {
		exchange = "NSE"
		if strings.HasPrefix(update.ExchangeSegment, "BSE") {
			exchange = "BSE"
		} else if strings.HasPrefix(update.ExchangeSegment, "MCX") {
			exchange = "MCX"
		}
		if strings.Contains(update.ExchangeSegment, "FNO") && exchange == "NSE" {
			exchange = "NFO"
		}
	}

	raw, _ := json.Marshal(update)

	ts := domain.ParseIST(update.CreateTime)
	if ts.IsZero() {
		ts = domain.NowIST()
	}

	avgPrice := update.AverageTradedPrice
	if avgPrice.IsZero() && !update.Price.IsZero() {
		avgPrice = update.Price
	}

	fill := domain.MasterFill{
		MasterID:        t.masterID,
		BrokerOrderID:   update.OrderID,
		Exchange:        exchange,
		Tradingsymbol:   symbol,
		TransactionType: update.TransactionType,
		Product:         update.ProductType,
		OrderType:       update.OrderType,
		FilledQuantity:  int(update.FilledQty),
		Price:           update.Price,
		TriggerPrice:    update.TriggerPrice,
		AveragePrice:    avgPrice,
		Status:          canonicalStatus,
		OrderTimestamp:  ts,
		RawPayload:      raw,
		Tag:             update.CorrelationID,
	}

	if t.queue != nil {
		if err := t.queue.Publish(context.Background(), fill); err != nil {
			t.log().Warn("dhan ws: failed to publish master fill", "error", err)
		}
	}
}

// TickerFactory produces master WebSocket ticker instances for Dhan master accounts.
type TickerFactory struct {
	queue    queue.Publisher[domain.MasterFill]
	resolver SymbolResolver
	logger   *slog.Logger
	feedURL  string
}

// NewTickerFactory creates a new Dhan TickerFactory.
func NewTickerFactory(queue queue.Publisher[domain.MasterFill], resolver SymbolResolver, logger *slog.Logger) *TickerFactory {
	return &TickerFactory{
		queue:    queue,
		resolver: resolver,
		logger:   logger,
	}
}

// SetFeedURL overrides default Dhan feed URL for testing.
func (f *TickerFactory) SetFeedURL(url string) {
	f.feedURL = url
}

// CreateTicker creates an isolated direct WebSocket ticker for a Dhan master account.
// If auth credentials (ApiKey or AccessToken) are missing, it returns nil, nil to skip.
func (f *TickerFactory) CreateTicker(
	ctx context.Context,
	masterID uuid.UUID,
	auth domain.AccountAuthInfo,
	catchUp func(context.Context) error,
) (ticker.Ticker, error) {
	clientID := strings.TrimSpace(auth.BrokerAccountID)
	if clientID == "" {
		clientID = strings.TrimSpace(auth.ApiKey)
	}
	token := strings.TrimSpace(auth.AccessToken)

	if clientID == "" || token == "" {
		return nil, nil
	}

	opts := []MasterTickerOption{}
	if f.feedURL != "" {
		opts = append(opts, WithFeedURL(f.feedURL))
	}

	masterTicker := NewMasterTicker(clientID, token, masterID, f.queue, f.resolver, f.logger, opts...)
	if catchUp != nil {
		masterTicker.SetCatchUpHook(catchUp)
	}

	return masterTicker, nil
}
