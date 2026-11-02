package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kotak"
	"envoytrade/internal/queue"
	"envoytrade/internal/ticker"

	"github.com/DragoHex/gokotakneo/models"
	"github.com/DragoHex/gokotakneo/orderstream"
	"github.com/google/uuid"
)

// SymbolResolver maps Kotak Neo's (exchangeSegment, securityID) to canonical (exchange, tradingsymbol).
type SymbolResolver interface {
	ResolveToSymbol(exchangeSegment, securityID string) (exchange string, tradingsymbol string, err error)
}

// MasterTickerOption configures MasterTicker behavior.
type MasterTickerOption func(*MasterTicker)

// WithRTURL overrides the default real-time WebSocket endpoint.
func WithRTURL(rtURL string) MasterTickerOption {
	return func(t *MasterTicker) {
		t.rtURL = rtURL
	}
}

// WithProxyURL configures a forward proxy for the order stream WebSocket.
func WithProxyURL(proxyURL string) MasterTickerOption {
	return func(t *MasterTicker) {
		t.proxyURL = proxyURL
	}
}

// MasterTicker manages a direct WebSocket connection to Kotak Neo's real-time order stream.
type MasterTicker struct {
	masterID   uuid.UUID
	tradeToken string
	tradeSid   string
	rtURL      string
	proxyURL   string
	queue      queue.Publisher[domain.MasterFill]
	resolver   SymbolResolver
	catchUp    func(context.Context) error
	logger     *slog.Logger

	client *orderstream.Client
	cancel context.CancelFunc
	mu     sync.Mutex
}

// NewMasterTicker creates an initialized MasterTicker for Kotak Neo.
func NewMasterTicker(
	masterID uuid.UUID,
	tradeToken string,
	tradeSid string,
	rtURL string,
	queue queue.Publisher[domain.MasterFill],
	resolver SymbolResolver,
	logger *slog.Logger,
	opts ...MasterTickerOption,
) *MasterTicker {
	t := &MasterTicker{
		masterID:   masterID,
		tradeToken: tradeToken,
		tradeSid:   tradeSid,
		rtURL:      rtURL,
		queue:      queue,
		resolver:   resolver,
		logger:     logger,
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

// Start initiates the order stream WebSocket connection.
func (t *MasterTicker) Start(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()

	callbacks := orderstream.StreamCallbacks{
		OnConnect: func() {
			t.log().Info("kotak ws: master ticker connected", "master_id", t.masterID)
		},
		OnOrderUpdate: func(update orderstream.OrderUpdate) {
			t.handleOrderUpdate(update)
		},
		OnReconnect: func(attempt int, delay time.Duration) {
			t.log().Info("kotak ws: master ticker reconnected", "master_id", t.masterID, "attempt", attempt)
			t.mu.Lock()
			hook := t.catchUp
			t.mu.Unlock()
			if hook != nil {
				go func() {
					if err := hook(context.Background()); err != nil {
						t.log().Warn("kotak ws: catch-up hook failed", "error", err)
					}
				}()
			}
		},
		OnError: func(err error) {
			t.log().Warn("kotak ws: order stream error", "error", err)
		},
	}

	client := orderstream.New(t.rtURL, t.tradeToken, t.tradeSid, callbacks)
	if t.proxyURL != "" {
		client.SetProxyURL(t.proxyURL)
	}
	t.client = client

	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	go func() {
		_ = client.Serve(runCtx)
	}()
}

// Stop terminates the order stream client gracefully.
func (t *MasterTicker) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	if t.client != nil {
		err := t.client.Close()
		t.client = nil
		return err
	}
	return nil
}

func (t *MasterTicker) handleOrderUpdate(update orderstream.OrderUpdate) {
	normStatus := update.NormalizedStatus()
	if !domain.IsTerminal(normStatus) {
		return // Drop non-terminal intermediate updates
	}

	exchange := normalizeExchange(update.ExchangeSegment)
	tradingsymbol := update.TradingSymbol
	if tradingsymbol == "" {
		tradingsymbol = update.Symbol
	}

	if t.resolver != nil && update.InstrumentToken != "" {
		if ex, sym, err := t.resolver.ResolveToSymbol(update.ExchangeSegment, update.InstrumentToken); err == nil && sym != "" {
			exchange = ex
			tradingsymbol = sym
		}
	}

	rawPayload, _ := json.Marshal(update)

	var ordTimestamp time.Time
	if update.OrderTimestamp != "" {
		var bt models.BrokerTime
		if err := json.Unmarshal([]byte(`"`+update.OrderTimestamp+`"`), &bt); err == nil {
			ordTimestamp = bt.Time
		}
	}
	if ordTimestamp.IsZero() {
		ordTimestamp = time.Now()
	}

	fill := domain.MasterFill{
		MasterID:        t.masterID,
		BrokerOrderID:   update.OrderNumber,
		Exchange:        exchange,
		Tradingsymbol:   tradingsymbol,
		TransactionType: normalizeTransactionType(update.TransactionType),
		Product:         normalizeProduct(update.ProductCode),
		OrderType:       normalizeOrderType(update.PriceType),
		FilledQuantity:  models.NormalizeMCXQuantity(tradingsymbol, exchange, update.FilledQuantity),
		AveragePrice:    update.AveragePrice,
		Status:          normStatus,
		OrderTimestamp:  ordTimestamp,
		RawPayload:      rawPayload,
		ReceivedAt:      time.Now(),
		DispatchState:   domain.DispatchPending,
	}

	if err := t.queue.Publish(context.Background(), fill); err != nil {
		t.log().Error("kotak ws: failed to publish master fill", "master_id", t.masterID, "order_no", update.OrderNumber, "error", err)
	}
}

func normalizeTransactionType(tt string) string {
	switch strings.ToUpper(strings.TrimSpace(tt)) {
	case "B", "BUY":
		return "BUY"
	case "S", "SELL":
		return "SELL"
	default:
		return "BUY"
	}
}

func normalizeProduct(prod string) string {
	p := strings.ToUpper(strings.TrimSpace(prod))
	switch p {
	case "CNC":
		return "CNC"
	case "NRML", "MARGIN":
		return "NRML"
	case "MIS", "INTRADAY":
		return "MIS"
	default:
		return p
	}
}

func normalizeOrderType(ot string) string {
	o := strings.ToUpper(strings.TrimSpace(ot))
	switch o {
	case "L", "LIMIT":
		return "LIMIT"
	case "MKT", "MARKET":
		return "MARKET"
	case "SL":
		return "SL"
	case "SL-M", "SLM":
		return "SL-M"
	default:
		return "MARKET"
	}
}

func normalizeExchange(seg string) string {
	s := strings.ToLower(strings.TrimSpace(seg))
	switch {
	case strings.HasPrefix(s, "nse_fo") || strings.HasPrefix(s, "nfo"):
		return "NFO"
	case strings.HasPrefix(s, "bse_fo") || strings.HasPrefix(s, "bfo"):
		return "BFO"
	case strings.HasPrefix(s, "mcx"):
		return "MCX"
	case strings.HasPrefix(s, "bse"):
		return "BSE"
	default:
		return "NSE"
	}
}

// ProxyResolver looks up forward proxy configuration for a static IP address.
type ProxyResolver interface {
	ProxyIPByAddress(ctx context.Context, ip string) (domain.ProxyIP, error)
}

// TickerFactory implements ticker.Factory for Kotak Neo accounts.
type TickerFactory struct {
	queue         queue.Publisher[domain.MasterFill]
	resolver      SymbolResolver
	proxyResolver ProxyResolver
	logger        *slog.Logger
}

// NewMasterTickerFactory creates a new ticker.Factory for Kotak Neo accounts.
func NewMasterTickerFactory(
	queue queue.Publisher[domain.MasterFill],
	resolver SymbolResolver,
	logger *slog.Logger,
) *TickerFactory {
	return &TickerFactory{
		queue:    queue,
		resolver: resolver,
		logger:   logger,
	}
}

// SetProxyResolver configures a proxy lookup resolver for the ticker factory.
func (f *TickerFactory) SetProxyResolver(resolver ProxyResolver) {
	f.proxyResolver = resolver
}

// CreateTicker satisfies ticker.Factory.
func (f *TickerFactory) CreateTicker(
	ctx context.Context,
	masterID uuid.UUID,
	auth domain.AccountAuthInfo,
	catchUp func(context.Context) error,
) (ticker.Ticker, error) {
	if strings.TrimSpace(auth.AccessToken) == "" {
		return nil, nil
	}

	sess, err := kotak.DeserializeTradeSession(auth.AccessToken)
	if err != nil || sess.Token == "" || sess.RTURL == "" {
		return nil, fmt.Errorf("%w: invalid Kotak Neo session token for ticker", domain.ErrAuthExpired)
	}

	var opts []MasterTickerOption
	if f.proxyResolver != nil && auth.IPAddress != "" {
		if proxy, err := f.proxyResolver.ProxyIPByAddress(ctx, auth.IPAddress); err == nil {
			u := &url.URL{
				Scheme: "https",
				Host:   proxy.Host + ":" + strconv.Itoa(proxy.Port),
			}
			if proxy.Username != "" {
				u.User = url.UserPassword(proxy.Username, proxy.Password)
			}
			opts = append(opts, WithProxyURL(u.String()))
		}
	}

	t := NewMasterTicker(
		masterID,
		sess.Token,
		sess.Sid,
		sess.RTURL,
		f.queue,
		f.resolver,
		f.logger,
		opts...,
	)
	t.SetCatchUpHook(catchUp)
	return t, nil
}
