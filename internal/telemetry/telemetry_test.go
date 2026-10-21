package telemetry_test

import (
	"context"
	"testing"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/telemetry"

	"github.com/google/uuid"
)

type mockKillSwitchProvider struct {
	status domain.KillSwitchStatus
}

func (m *mockKillSwitchProvider) KillSwitchStatus() domain.KillSwitchStatus {
	return m.status
}

type mockTickerProvider struct {
	ids []uuid.UUID
}

func (m *mockTickerProvider) ActiveMasterIDs() []uuid.UUID {
	return m.ids
}

type mockQueueMeasurer struct {
	length   int
	capacity int
}

func (m *mockQueueMeasurer) Len() int { return m.length }
func (m *mockQueueMeasurer) Cap() int { return m.capacity }

type mockWorkerCounter struct {
	count int
}

func (m *mockWorkerCounter) Count() int { return m.count }

type mockSystemResourcesProvider struct {
	res domain.SystemResources
}

func (m *mockSystemResourcesProvider) SystemResources() domain.SystemResources {
	return m.res
}

func TestCollector_PipelineStatus(t *testing.T) {
	now := time.Now().UTC()
	ks := &mockKillSwitchProvider{
		status: domain.KillSwitchStatus{
			Engaged:   true,
			EngagedAt: &now,
			EngagedBy: "admin@envoytrade.com",
			Reason:    "Test trigger",
		},
	}
	id1 := uuid.New()
	tp := &mockTickerProvider{ids: []uuid.UUID{id1}}
	mq := &mockQueueMeasurer{length: 5, capacity: 1024}
	uq := &mockQueueMeasurer{length: 2, capacity: 512}
	wc := &mockWorkerCounter{count: 4}
	dbStat := func() domain.DBPoolStatus {
		return domain.DBPoolStatus{
			AcquiredConns: 3,
			IdleConns:     7,
			TotalConns:    10,
			MaxConns:      20,
		}
	}
	sys := &mockSystemResourcesProvider{
		res: domain.SystemResources{
			CPUPercent:     12.5,
			HeapAllocMB:    24.2,
			SysMemoryMB:    48.0,
			GoroutineCount: 32,
			GCCycles:       10,
		},
	}

	col := telemetry.NewCollector(ks, tp, mq, uq, wc, dbStat, sys)
	status := col.PipelineStatus(context.Background())

	if !status.KillSwitch.Engaged || status.KillSwitch.Reason != "Test trigger" {
		t.Fatalf("unexpected killswitch status: %+v", status.KillSwitch)
	}
	if len(status.MasterTickers) != 1 || status.MasterTickers[0].MasterID != id1 {
		t.Fatalf("unexpected master tickers: %+v", status.MasterTickers)
	}
	if status.Queues.MasterFillDepth != 5 || status.Queues.MasterFillCap != 1024 {
		t.Fatalf("unexpected master queue stats: %+v", status.Queues)
	}
	if status.Queues.OrderUpdateDepth != 2 || status.Queues.OrderUpdateCap != 512 {
		t.Fatalf("unexpected order update queue stats: %+v", status.Queues)
	}
	if status.WorkerCount != 4 {
		t.Fatalf("expected worker count 4, got %d", status.WorkerCount)
	}
	if status.DBPool.TotalConns != 10 || status.DBPool.AcquiredConns != 3 {
		t.Fatalf("unexpected db pool stats: %+v", status.DBPool)
	}
	if status.SystemResources.CPUPercent != 12.5 || status.SystemResources.HeapAllocMB != 24.2 || status.SystemResources.GoroutineCount != 32 {
		t.Fatalf("unexpected system resources: %+v", status.SystemResources)
	}
}
