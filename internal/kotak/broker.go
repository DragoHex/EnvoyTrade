package kotak

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	gokotak "github.com/DragoHex/gokotakneo"
	"github.com/DragoHex/gokotakneo/models"
	"github.com/shopspring/decimal"
)

// API abstracts the *gokotak.Client methods required by Broker adapter.
type API interface {
	PlaceOrder(ctx context.Context, params models.OrderParams) (*models.OrderResponse, error)
	ModifyOrder(ctx context.Context, params models.OrderModifyParams) (*models.OrderResponse, error)
	CancelOrder(ctx context.Context, orderNo string, isAMO bool, tradingSymbol string) (*models.OrderResponse, error)
	GetOrders(ctx context.Context) ([]models.Order, error)
	GetOrderHistory(ctx context.Context, orderNo string) ([]models.Order, error)
	GetPositions(ctx context.Context) ([]models.Position, error)
	GetHoldings(ctx context.Context) ([]models.Holding, error)
	GetLimits(ctx context.Context, seg, exch, prod string) (*models.Limits, error)
}

// InstrumentLookup maps canonical (exchange, tradingsymbol) to Kotak's segment and security details.
type InstrumentLookup interface {
	ResolveToSecurityID(exchange, tradingsymbol string) (exchangeSegment string, securityID string, lotSize int, err error)
}

// Broker implements broker.Broker for Kotak Neo.
type Broker struct {
	api      API
	proxied  bool
	resolver InstrumentLookup
}

// NewBroker constructs a Kotak Neo Broker adapter.
func NewBroker(api API, proxied bool, resolver InstrumentLookup) *Broker {
	return &Broker{
		api:      api,
		proxied:  proxied,
		resolver: resolver,
	}
}

// NewLiveBroker constructs a live Kotak Neo Broker instance.
func NewLiveBroker(consumerKey string, session *TradeSessionJSON, proxyURL string, resolver InstrumentLookup) (*Broker, error) {
	var opts []gokotak.Option
	proxied := false
	if strings.TrimSpace(proxyURL) != "" {
		opts = append(opts, gokotak.WithProxyURL(strings.TrimSpace(proxyURL)))
		proxied = true
	}

	client := gokotak.New(consumerKey, opts...)
	if session != nil {
		client.SetGateways(session.BaseURL, session.FeedURL, session.RTURL)
		client.SetSessionTokens(session.Token, session.Sid)
	}

	return NewBroker(client, proxied, resolver), nil
}

// IsProxied returns true if this broker instance routes egress through a static proxy.
func (b *Broker) IsProxied() bool {
	return b.proxied
}

func mapProductToKotak(product string) string {
	switch strings.ToUpper(strings.TrimSpace(product)) {
	case "CNC":
		return "CNC"
	case "MIS", "INTRADAY":
		return "MIS"
	case "NRML", "MARGIN":
		return "NRML"
	case "MTF":
		return "MTF"
	default:
		return "MIS"
	}
}

func mapOrderTypeToKotak(orderType string) string {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "MARKET", "MKT":
		return "MKT"
	case "LIMIT", "L":
		return "L"
	case "SL":
		return "SL"
	case "SL-M", "SLM":
		return "SL-M"
	default:
		return "MKT"
	}
}

func mapTransactionTypeToKotak(tt string) string {
	switch strings.ToUpper(strings.TrimSpace(tt)) {
	case "BUY", "B":
		return "B"
	case "SELL", "S":
		return "S"
	default:
		return "B"
	}
}

// PlaceOrder translates broker.OrderParams to Kotak Neo OrderParams and sends the order.
func (b *Broker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	if !b.proxied {
		return broker.OrderResponse{}, fmt.Errorf("kotak place order: %w", domain.ErrUnproxiedNotAllowed)
	}

	segment := "nse_cm"
	if b.resolver != nil && params.Exchange != "" && params.Tradingsymbol != "" {
		if seg, _, _, err := b.resolver.ResolveToSecurityID(params.Exchange, params.Tradingsymbol); err == nil && seg != "" {
			segment = seg
		}
	} else if strings.EqualFold(params.Exchange, "NFO") || strings.EqualFold(params.Exchange, "NSE_FO") {
		segment = "nse_fo"
	} else if strings.EqualFold(params.Exchange, "BFO") || strings.EqualFold(params.Exchange, "BSE_FO") {
		segment = "bse_fo"
	} else if strings.EqualFold(params.Exchange, "MCX") || strings.EqualFold(params.Exchange, "MCX_FO") {
		segment = "mcx_fo"
	} else if strings.EqualFold(params.Exchange, "BSE") {
		segment = "bse_cm"
	}

	priceStr := "0"
	ot := mapOrderTypeToKotak(params.OrderType)
	if ot == "L" || ot == "SL" {
		priceStr = strconv.FormatFloat(params.Price, 'f', -1, 64)
	}

	trigPriceStr := "0"
	if ot == "SL" || ot == "SL-M" {
		trigPriceStr = strconv.FormatFloat(params.TriggerPrice, 'f', -1, 64)
	}

	ts := strings.ToUpper(strings.TrimSpace(params.Tradingsymbol))
	if strings.EqualFold(segment, "nse_cm") && !strings.Contains(ts, "-") {
		ts = ts + "-EQ"
	}

	kParams := models.OrderParams{
		Am: "NO", // Regular orders only
		Dq: "0",
		Es: segment,
		Mp: "0",
		Pc: mapProductToKotak(params.Product),
		Pf: "N",
		Pr: priceStr,
		Pt: ot,
		Qt: strconv.Itoa(params.Quantity),
		Rt: "DAY",
		Tp: trigPriceStr,
		Ts: ts,
		Tt: mapTransactionTypeToKotak(params.TransactionType),
	}

	resp, err := b.api.PlaceOrder(ctx, kParams)
	if err != nil {
		return broker.OrderResponse{}, translateBrokerError(err)
	}
	if resp == nil {
		return broker.OrderResponse{}, fmt.Errorf("%w: nil response from broker", domain.ErrBrokerUnreachable)
	}

	return broker.OrderResponse{
		OrderID: resp.OrderNumber,
	}, nil
}

// CancelOrder cancels an open Kotak Neo order by orderID.
func (b *Broker) CancelOrder(ctx context.Context, variety, orderID string) (broker.OrderResponse, error) {
	resp, err := b.api.CancelOrder(ctx, orderID, false, "")
	if err != nil {
		return broker.OrderResponse{}, translateBrokerError(err)
	}
	if resp == nil {
		return broker.OrderResponse{}, fmt.Errorf("%w: nil response from broker", domain.ErrBrokerUnreachable)
	}
	return broker.OrderResponse{OrderID: resp.OrderNumber}, nil
}

// GetOrders retrieves all orders for the current day.
func (b *Broker) GetOrders(ctx context.Context) ([]broker.Order, error) {
	kOrders, err := b.api.GetOrders(ctx)
	if err != nil {
		return nil, translateBrokerError(err)
	}

	out := make([]broker.Order, 0, len(kOrders))
	for _, o := range kOrders {
		out = append(out, mapOrder(o))
	}
	return out, nil
}

// GetOpenOrders retrieves only non-terminal open orders.
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	orders, err := b.GetOrders(ctx)
	if err != nil {
		return nil, err
	}
	var open []broker.Order
	for _, o := range orders {
		if !domain.IsTerminal(o.Status) {
			open = append(open, o)
		}
	}
	return open, nil
}

// GetOrderHistory retrieves state transitions for an order.
func (b *Broker) GetOrderHistory(ctx context.Context, orderID string) ([]broker.Order, error) {
	kOrders, err := b.api.GetOrderHistory(ctx, orderID)
	if err != nil {
		return nil, translateBrokerError(err)
	}
	out := make([]broker.Order, 0, len(kOrders))
	for _, o := range kOrders {
		out = append(out, mapOrder(o))
	}
	return out, nil
}

// GetPositions retrieves all current intraday and overnight positions.
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	kPositions, err := b.api.GetPositions(ctx)
	if err != nil {
		return nil, translateBrokerError(err)
	}

	out := make([]broker.Position, 0, len(kPositions))
	for _, p := range kPositions {
		avgPrice := 0.0
		if p.FilledBuyQty > 0 && !p.BuyAmount.IsZero() {
			f, _ := p.BuyAmount.Div(decimal.NewFromInt(int64(p.FilledBuyQty))).Float64()
			avgPrice = f
		}

		out = append(out, broker.Position{
			Product:       p.Product,
			Tradingsymbol: p.TradingSymbol,
			Exchange:      normalizeExchange(p.ExchangeSegment),
			Quantity:      p.Quantity,
			AveragePrice:  avgPrice,
		})
	}
	return out, nil
}

func mapOrder(o models.Order) broker.Order {
	return broker.Order{
		OrderID:         o.OrderNumber,
		Tradingsymbol:   o.TradingSymbol,
		Exchange:        normalizeExchange(o.ExchangeSegment),
		TransactionType: o.TransactionType,
		OrderType:       o.PriceType,
		Product:         o.ProductCode,
		Quantity:        o.Quantity,
		FilledQuantity:  o.FilledQuantity,
		Price:           o.Price,
		TriggerPrice:    o.TriggerPrice,
		AveragePrice:    o.AveragePrice,
		Status:          strings.ToUpper(o.Status),
		StatusMessage:   o.RejectionReason,
		OrderTimestamp:  o.OrderTimestamp.Time,
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

func translateBrokerError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "100008") || strings.Contains(msg, "ip not whitelisted"):
		return fmt.Errorf("kotak static ip not whitelisted: %w", err)
	case strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "token expired") || strings.Contains(msg, "invalid token"):
		return fmt.Errorf("%w: %v", domain.ErrAuthExpired, err)
	case strings.Contains(msg, "404") || strings.Contains(msg, "not found"):
		return fmt.Errorf("%w: %v", domain.ErrNotFound, err)
	case strings.Contains(msg, "1004") || strings.Contains(msg, "insufficient funds") || strings.Contains(msg, "rms"):
		return fmt.Errorf("kotak rms rejection: %w", err)
	case strings.Contains(msg, "unreachable") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "timeout") || strings.Contains(msg, "502") || strings.Contains(msg, "503"):
		return fmt.Errorf("%w: %v", domain.ErrBrokerUnreachable, err)
	default:
		return err
	}
}
