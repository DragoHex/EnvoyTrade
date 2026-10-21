package telemetry

import (
	"sync"

	"envoytrade/internal/domain"
)

// ExecutionFeed is an in-memory, thread-safe ring buffer storing recent order execution events.
type ExecutionFeed struct {
	mu       sync.RWMutex
	capacity int
	buffer   []domain.ExecutionEvent
}

// NewExecutionFeed constructs an ExecutionFeed capped at capacity.
func NewExecutionFeed(capacity int) *ExecutionFeed {
	if capacity <= 0 {
		capacity = 25
	}
	return &ExecutionFeed{
		capacity: capacity,
		buffer:   make([]domain.ExecutionEvent, 0, capacity),
	}
}

// Record appends a new execution event, evicting the oldest event if capacity is reached.
func (f *ExecutionFeed) Record(event domain.ExecutionEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.buffer) < f.capacity {
		f.buffer = append(f.buffer, event)
	} else {
		copy(f.buffer, f.buffer[1:])
		f.buffer[len(f.buffer)-1] = event
	}
}

// Recent returns the recorded events in reverse chronological order (newest first).
func (f *ExecutionFeed) Recent() []domain.ExecutionEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()

	n := len(f.buffer)
	res := make([]domain.ExecutionEvent, n)
	for i := 0; i < n; i++ {
		res[i] = f.buffer[n-1-i]
	}
	return res
}
