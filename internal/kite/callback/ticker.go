package callback

import (
	"context"
	"log/slog"

	"envoytrade/internal/domain"
	"envoytrade/internal/queue"

	"github.com/google/uuid"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
)

// Conn is the subset of *kiteticker.Ticker MasterTicker needs — narrowed
// to make it fakeable in tests without touching the vendored SDK
// (PLAN.md §1's seam rule). PLAN.md §3.4: gokiteconnect's ticker dials
// through a shared package-level websocket.DefaultDialer it mutates in
// place, so at most one Conn may run per process — the master's.
type Conn interface {
	OnOrderUpdate(f func(order kiteconnect.Order))
	ServeWithContext(ctx context.Context)
	Close() error
}

// MasterTicker is the WS fast path for master fill detection: it
// converts and forwards terminal order updates onto the same
// queue.Publisher[domain.MasterFill] the postback handler uses.
// Delivering the same fill via both paths is safe — InsertMasterFill's
// unique constraint makes the second delivery a no-op.
type MasterTicker struct {
	conn      Conn
	masterID  uuid.UUID
	publisher queue.Publisher[domain.MasterFill]
	logger    *slog.Logger
}

// NewMasterTicker wires conn's order-update callback to publish onto
// publisher for masterID.
func NewMasterTicker(conn Conn, masterID uuid.UUID, publisher queue.Publisher[domain.MasterFill], logger *slog.Logger) *MasterTicker {
	t := &MasterTicker{conn: conn, masterID: masterID, publisher: publisher, logger: logger}
	conn.OnOrderUpdate(t.handleOrderUpdate)
	return t
}

func (t *MasterTicker) log() *slog.Logger {
	if t.logger != nil {
		return t.logger
	}
	return slog.Default()
}

func (t *MasterTicker) handleOrderUpdate(o kiteconnect.Order) {
	if !IsTerminal(o.Status) {
		return
	}
	if err := t.publisher.Publish(context.Background(), ToMasterFill(o, t.masterID)); err != nil {
		t.log().Warn("ticker: master fill queue full, dropping", "order_id", o.OrderID, "error", err)
	}
}

// Start blocks serving the connection until ctx is cancelled.
func (t *MasterTicker) Start(ctx context.Context) {
	t.conn.ServeWithContext(ctx)
}

// Stop closes the underlying connection.
func (t *MasterTicker) Stop() error {
	return t.conn.Close()
}
