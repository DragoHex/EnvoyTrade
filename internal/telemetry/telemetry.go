package telemetry

import (
	"context"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// KillSwitchProvider provides the status of the emergency fan-out kill switch.
type KillSwitchProvider interface {
	KillSwitchStatus() domain.KillSwitchStatus
}

// TickerProvider provides IDs of active master WebSocket tickers.
type TickerProvider interface {
	ActiveMasterIDs() []uuid.UUID
}

// QueueMeasurer measures buffer length and capacity of an in-memory queue.
type QueueMeasurer interface {
	Len() int
	Cap() int
}

// WorkerCounter counts active follower worker goroutines.
type WorkerCounter interface {
	Count() int
}

// SystemResourcesProvider provides system-level CPU, memory, and runtime metrics.
type SystemResourcesProvider interface {
	SystemResources() domain.SystemResources
}

// Collector collects in-memory pipeline health telemetry with zero SQL queries and zero STW pauses.
type Collector struct {
	engine      KillSwitchProvider
	tickerMgr   TickerProvider
	masterQueue QueueMeasurer
	updateQueue QueueMeasurer
	workers     WorkerCounter
	dbStat      func() domain.DBPoolStatus
	sysProvider SystemResourcesProvider
}

// NewCollector constructs a Collector with the given components.
func NewCollector(
	engine KillSwitchProvider,
	tickerMgr TickerProvider,
	masterQueue QueueMeasurer,
	updateQueue QueueMeasurer,
	workers WorkerCounter,
	dbStat func() domain.DBPoolStatus,
	sysProvider SystemResourcesProvider,
) *Collector {
	return &Collector{
		engine:      engine,
		tickerMgr:   tickerMgr,
		masterQueue: masterQueue,
		updateQueue: updateQueue,
		workers:     workers,
		dbStat:      dbStat,
		sysProvider: sysProvider,
	}
}

// PipelineStatus returns an instantaneous in-memory snapshot of system telemetry.
func (c *Collector) PipelineStatus(ctx context.Context) domain.PipelineStatus {
	var status domain.PipelineStatus

	if c.engine != nil {
		status.KillSwitch = c.engine.KillSwitchStatus()
	}

	if c.tickerMgr != nil {
		ids := c.tickerMgr.ActiveMasterIDs()
		status.MasterTickers = make([]domain.TickerStatus, len(ids))
		for i, id := range ids {
			status.MasterTickers[i] = domain.TickerStatus{
				MasterID: id,
				Active:   true,
			}
		}
	} else {
		status.MasterTickers = []domain.TickerStatus{}
	}

	if c.masterQueue != nil {
		status.Queues.MasterFillDepth = c.masterQueue.Len()
		status.Queues.MasterFillCap = c.masterQueue.Cap()
	}

	if c.updateQueue != nil {
		status.Queues.OrderUpdateDepth = c.updateQueue.Len()
		status.Queues.OrderUpdateCap = c.updateQueue.Cap()
	}

	if c.workers != nil {
		status.WorkerCount = c.workers.Count()
	}

	if c.dbStat != nil {
		status.DBPool = c.dbStat()
	}

	if c.sysProvider != nil {
		status.SystemResources = c.sysProvider.SystemResources()
	}

	return status
}
