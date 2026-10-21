package telemetry_test

import (
	"testing"
	"time"

	"envoytrade/internal/telemetry"
)

func TestSystemSampler_ReadResources(t *testing.T) {
	sampler := telemetry.NewSystemSampler(50 * time.Millisecond)
	defer sampler.Stop()

	// Wait briefly for at least one sample tick
	time.Sleep(75 * time.Millisecond)

	res := sampler.SystemResources()

	if res.GoroutineCount <= 0 {
		t.Fatalf("expected positive goroutine count, got %d", res.GoroutineCount)
	}
	if res.HeapAllocMB <= 0 {
		t.Fatalf("expected positive heap alloc MB, got %f", res.HeapAllocMB)
	}
	if res.SysMemoryMB <= 0 {
		t.Fatalf("expected positive sys memory MB, got %f", res.SysMemoryMB)
	}
	if res.CPUPercent < 0 || res.CPUPercent > 100 {
		t.Fatalf("expected CPU percent between 0 and 100, got %f", res.CPUPercent)
	}
}

func TestSystemSampler_Lifecycle(t *testing.T) {
	sampler := telemetry.NewSystemSampler(10 * time.Millisecond)

	// Stopping once
	sampler.Stop()

	// Stopping multiple times must be safe (idempotent)
	sampler.Stop()

	// SystemResources after stop still returns valid memory snapshot without panicking
	res := sampler.SystemResources()
	if res.GoroutineCount <= 0 {
		t.Fatalf("expected positive goroutine count after stop, got %d", res.GoroutineCount)
	}
}
