package callback

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountLookup resolves the account a postback belongs to.
type AccountLookup interface {
	AccountByBrokerUserID(ctx context.Context, brokerUserID string) (id uuid.UUID, role string, apiSecret string, err error)
}

// SymbolResolver maps Dhan's (exchangeSegment, securityID) to canonical (exchange, tradingsymbol).
type SymbolResolver interface {
	ResolveToSymbol(exchangeSegment, securityID string) (exchange string, tradingsymbol string, err error)
}

// dhanPostbackPayload represents Dhan's webhook order execution report.
type dhanPostbackPayload struct {
	DhanClientID       string          `json:"dhanClientId"`
	DhanClientIDAlt    string          `json:"dhan_client_id"`
	OrderID            string          `json:"orderId"`
	CorrelationID      string          `json:"correlationId"`
	OrderStatus        string          `json:"orderStatus"`
	TransactionType    string          `json:"transactionType"`
	ExchangeSegment    string          `json:"exchangeSegment"`
	ProductType        string          `json:"productType"`
	OrderType          string          `json:"orderType"`
	TradingSymbol      string          `json:"tradingSymbol"`
	SecurityID         string          `json:"securityId"`
	Quantity           int32           `json:"quantity"`
	FilledQty          int32           `json:"filledQty"`
	Price              decimal.Decimal `json:"price"`
	TriggerPrice       decimal.Decimal `json:"triggerPrice"`
	AverageTradedPrice decimal.Decimal `json:"averageTradedPrice"`
	CreateTime         string          `json:"createTime"`
	UpdateTime         string          `json:"updateTime"`
}

// Handler is an http.Handler for Dhan's webhook postback.
type Handler struct {
	Accounts        AccountLookup
	MasterFills     queue.Publisher[domain.MasterFill]
	FollowerUpdates queue.Publisher[domain.OrderUpdate]
	Resolver        SymbolResolver
	Logger          *slog.Logger
}

func (h *Handler) log() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// MapTerminalStatus maps Dhan's order status to EnvoyTrade's canonical terminal status.
func MapTerminalStatus(rawStatus string) string {
	s := strings.ToUpper(strings.TrimSpace(rawStatus))
	switch s {
	case "TRADED":
		return domain.TerminalComplete
	case "REJECTED":
		return domain.TerminalRejected
	case "CANCELLED", "CANCELED":
		return domain.TerminalCancelled
	default:
		return s
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p dhanPostbackPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		h.log().Warn("dhan postback: malformed json body", "error", err)
		http.Error(w, "malformed body", http.StatusBadRequest)
		return
	}

	clientID := strings.TrimSpace(p.DhanClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(p.DhanClientIDAlt)
	}

	ctx := r.Context()
	var accountID uuid.UUID
	var role string
	var err error

	if bal, ok := h.Accounts.(interface {
		AccountByBrokerAndUserID(ctx context.Context, brokerName, brokerUserID string) (uuid.UUID, string, string, error)
	}); ok {
		accountID, role, _, err = bal.AccountByBrokerAndUserID(ctx, "dhan", clientID)
		if errors.Is(err, domain.ErrNotFound) {
			accountID, role, _, err = h.Accounts.AccountByBrokerUserID(ctx, clientID)
		}
	} else {
		accountID, role, _, err = h.Accounts.AccountByBrokerUserID(ctx, clientID)
	}

	if err != nil {
		h.log().Warn("dhan postback: unknown account, dropping", "client_id", clientID, "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	canonicalStatus := MapTerminalStatus(p.OrderStatus)
	if !domain.IsTerminal(canonicalStatus) {
		h.log().Debug("dhan postback: non-terminal status ignored", "order_id", p.OrderID, "status", p.OrderStatus)
		w.WriteHeader(http.StatusOK)
		return
	}

	var exchange, symbol string
	if h.Resolver != nil && p.ExchangeSegment != "" && p.SecurityID != "" {
		if ex, sym, err := h.Resolver.ResolveToSymbol(p.ExchangeSegment, p.SecurityID); err == nil {
			exchange = ex
			symbol = sym
		}
	}
	if symbol == "" {
		symbol = p.TradingSymbol
	}
	if exchange == "" {
		exchange = "NSE"
		if strings.HasPrefix(p.ExchangeSegment, "BSE") {
			exchange = "BSE"
		} else if strings.HasPrefix(p.ExchangeSegment, "MCX") {
			exchange = "MCX"
		}
		if strings.Contains(p.ExchangeSegment, "FNO") && exchange == "NSE" {
			exchange = "NFO"
		}
	}

	raw, _ := json.Marshal(p)

	ts := domain.ParseIST(p.CreateTime)
	if ts.IsZero() {
		ts = domain.NowIST()
	}

	avgPrice := p.AverageTradedPrice
	if avgPrice.IsZero() && !p.Price.IsZero() {
		avgPrice = p.Price
	}

	switch role {
	case "master":
		if h.MasterFills == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		fill := domain.MasterFill{
			MasterID:        accountID,
			BrokerOrderID:   p.OrderID,
			Exchange:        exchange,
			Tradingsymbol:   symbol,
			TransactionType: p.TransactionType,
			Product:         p.ProductType,
			OrderType:       p.OrderType,
			FilledQuantity:  int(p.FilledQty),
			Price:           p.Price,
			TriggerPrice:    p.TriggerPrice,
			AveragePrice:    avgPrice,
			Status:          canonicalStatus,
			OrderTimestamp:  ts,
			RawPayload:      raw,
			Tag:             p.CorrelationID,
		}
		if err := h.MasterFills.Publish(ctx, fill); err != nil {
			h.log().Warn("dhan postback: failed to publish master fill", "error", err)
		}

	case "follower":
		if h.FollowerUpdates == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		update := domain.OrderUpdate{
			FollowerID:      accountID,
			BrokerOrderID:   p.OrderID,
			Tradingsymbol:   symbol,
			Exchange:        exchange,
			Product:         p.ProductType,
			TransactionType: p.TransactionType,
			OrderType:       p.OrderType,
			Quantity:        int(p.Quantity),
			Status:          canonicalStatus,
			FilledQuantity:  int(p.FilledQty),
			AveragePrice:    avgPrice,
			OrderTimestamp:  ts,
			Tag:             p.CorrelationID,
			RawPayload:      raw,
		}
		if err := h.FollowerUpdates.Publish(ctx, update); err != nil {
			h.log().Warn("dhan postback: failed to publish follower update", "error", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}
