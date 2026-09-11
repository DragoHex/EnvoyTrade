package callback_test

import (
	"context"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/kite/callback"

	"github.com/google/uuid"
	kiteconnect "github.com/zerodha/gokiteconnect/v4"
	"go.uber.org/goleak"
)

type fakeConn struct {
	onOrderUpdate func(kiteconnect.Order)
	serveDone     chan struct{}
	closed        bool
}

func (c *fakeConn) OnOrderUpdate(f func(kiteconnect.Order)) { c.onOrderUpdate = f }

func (c *fakeConn) ServeWithContext(ctx context.Context) {
	<-ctx.Done()
	close(c.serveDone)
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func TestMasterTicker_TerminalOrderIsPublished(t *testing.T) {
	conn := &fakeConn{serveDone: make(chan struct{})}
	masterID := uuid.New()
	pub := &fakePublisher[domain.MasterFill]{}

	callback.NewMasterTicker(conn, masterID, pub, nil)
	conn.onOrderUpdate(kiteconnect.Order{OrderID: "1", Status: domain.TerminalComplete})

	if len(pub.published) != 1 {
		t.Fatalf("published %d, want 1", len(pub.published))
	}
	if pub.published[0].MasterID != masterID {
		t.Errorf("MasterID = %v, want %v", pub.published[0].MasterID, masterID)
	}
}

func TestMasterTicker_NonTerminalOrderIsDropped(t *testing.T) {
	conn := &fakeConn{serveDone: make(chan struct{})}
	pub := &fakePublisher[domain.MasterFill]{}

	callback.NewMasterTicker(conn, uuid.New(), pub, nil)
	conn.onOrderUpdate(kiteconnect.Order{OrderID: "1", Status: "OPEN"})

	if len(pub.published) != 0 {
		t.Fatalf("published %d, want 0", len(pub.published))
	}
}

func TestMasterTicker_StopClosesConnAndStartReturnsCleanly(t *testing.T) {
	defer goleak.VerifyNone(t)

	conn := &fakeConn{serveDone: make(chan struct{})}
	mt := callback.NewMasterTicker(conn, uuid.New(), &fakePublisher[domain.MasterFill]{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		mt.Start(ctx)
		close(done)
	}()

	if err := mt.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !conn.closed {
		t.Fatal("Stop did not close the underlying conn")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

type fakeConnectingConn struct {
	fakeConn
	onConnect func()
}

func (c *fakeConnectingConn) OnConnect(f func()) {
	c.onConnect = f
}

func TestMasterTicker_ReconnectTriggersCatchUpHook(t *testing.T) {
	conn := &fakeConnectingConn{fakeConn: fakeConn{serveDone: make(chan struct{})}}
	pub := &fakePublisher[domain.MasterFill]{}
	mt := callback.NewMasterTicker(conn, uuid.New(), pub, nil)

	hookCalled := 0
	mt.SetCatchUpHook(func(ctx context.Context) error {
		hookCalled++
		return nil
	})

	if conn.onConnect == nil {
		t.Fatal("expected onConnect callback to be registered")
	}

	// First connect: initial connection, should not trigger catch-up hook
	conn.onConnect()
	if hookCalled != 0 {
		t.Fatalf("hook called on initial connect: %d, want 0", hookCalled)
	}

	// Second connect: reconnect! Must trigger catch-up hook
	conn.onConnect()
	if hookCalled != 1 {
		t.Fatalf("hook called on reconnect: %d, want 1", hookCalled)
	}

	// Manual TriggerCatchUp
	if err := mt.TriggerCatchUp(context.Background()); err != nil {
		t.Fatalf("TriggerCatchUp: %v", err)
	}
	if hookCalled != 2 {
		t.Fatalf("hook called after manual trigger: %d, want 2", hookCalled)
	}
}
