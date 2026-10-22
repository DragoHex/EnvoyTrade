package telemetry

import (
	"math"
	"runtime"
	"runtime/metrics"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"envoytrade/internal/domain"
)

// SystemSampler periodically samples process CPU utilization and reads lock-free runtime memory metrics.
type SystemSampler struct {
	interval   time.Duration
	cpuBits    atomic.Uint64
	done       chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
	metricDesc []metrics.Sample
}

// NewSystemSampler initializes and starts a background CPU sampler ticker.
func NewSystemSampler(interval time.Duration) *SystemSampler {
	if interval <= 0 {
		interval = 3 * time.Second
	}

	s := &SystemSampler{
		interval: interval,
		done:     make(chan struct{}),
		metricDesc: []metrics.Sample{
			{Name: "/memory/classes/heap/objects:bytes"},
			{Name: "/memory/classes/total:bytes"},
			{Name: "/gc/cycles/total:gc-cycles"},
		},
	}

	s.wg.Add(1)
	go s.sampleLoop()

	return s
}

func getProcessCPUTime() (float64, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0, err
	}
	uSec := float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6
	sSec := float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6
	return uSec + sSec, nil
}

func (s *SystemSampler) sampleLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	prevTime := time.Now()
	prevCPU, _ := getProcessCPUTime()
	numCPU := float64(runtime.NumCPU())
	if numCPU <= 0 {
		numCPU = 1
	}

	for {
		select {
		case <-s.done:
			return
		case now := <-ticker.C:
			currCPU, err := getProcessCPUTime()
			if err == nil {
				deltaWall := now.Sub(prevTime).Seconds()
				deltaCPU := currCPU - prevCPU

				if deltaWall > 0 && deltaCPU >= 0 {
					pct := (deltaCPU / (deltaWall * numCPU)) * 100.0
					if pct < 0 {
						pct = 0
					} else if pct > 100 {
						pct = 100
					}
					s.cpuBits.Store(math.Float64bits(pct))
				}
				prevCPU = currCPU
				prevTime = now
			}
		}
	}
}

// SystemResources returns instantaneous in-memory process resource stats with zero STW pauses.
func (s *SystemSampler) SystemResources() domain.SystemResources {
	// 1. Read atomic CPU percentage
	cpuPercent := math.Float64frombits(s.cpuBits.Load())

	// 2. Read lock-free runtime memory samples
	// Clone samples to avoid data races across concurrent callers
	samples := make([]metrics.Sample, len(s.metricDesc))
	copy(samples, s.metricDesc)
	metrics.Read(samples)

	var heapBytes, sysBytes uint64
	var gcCycles uint64

	if samples[0].Value.Kind() == metrics.KindUint64 {
		heapBytes = samples[0].Value.Uint64()
	}
	if samples[1].Value.Kind() == metrics.KindUint64 {
		sysBytes = samples[1].Value.Uint64()
	}
	if samples[2].Value.Kind() == metrics.KindUint64 {
		gcCycles = samples[2].Value.Uint64()
	}

	return domain.SystemResources{
		CPUPercent:     cpuPercent,
		HeapAllocMB:    math.Round((float64(heapBytes)/(1024*1024))*10) / 10,
		SysMemoryMB:    math.Round((float64(sysBytes)/(1024*1024))*10) / 10,
		GoroutineCount: runtime.NumGoroutine(),
		GCCycles:       uint32(gcCycles),
	}
}

// Stop cleanly terminates the background sampling ticker. Safe to call multiple times.
func (s *SystemSampler) Stop() {
	s.stopOnce.Do(func() {
		close(s.done)
		s.wg.Wait()
	})
}
