package archive_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/archive"
)

type fakeArchiverBackend struct {
	mu           sync.Mutex
	archiveCalls int
	lastCutoff   time.Time
	lastDir      string
	returnErr    error
	returnSum    *archive.ArchiveSummary
}

func (f *fakeArchiverBackend) Archive(ctx context.Context, cutoff time.Time, archiveDir string) (*archive.ArchiveSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.archiveCalls++
	f.lastCutoff = cutoff
	f.lastDir = archiveDir
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	if f.returnSum != nil {
		return f.returnSum, nil
	}
	return &archive.ArchiveSummary{
		CutoffDate:             cutoff.Format("2006-01-02"),
		MasterFillsArchived:    5,
		FollowerOrdersArchived: 10,
		OrderEventsArchived:    15,
	}, nil
}

func (f *fakeArchiverBackend) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.archiveCalls
}

func TestRoutine_Defaults(t *testing.T) {
	backend := &fakeArchiverBackend{}
	r := archive.NewRoutine(backend, archive.RoutineConfig{}, slog.Default())
	cfg := r.Config()

	// Default interval is 0 (runs daily at 01:00 AM IST)
	if cfg.Interval != 0 {
		t.Errorf("Interval = %v, want 0", cfg.Interval)
	}
	if cfg.ArchiveDir != "/var/backups/envoytrade/archive" {
		t.Errorf("ArchiveDir = %q, want /var/backups/envoytrade/archive", cfg.ArchiveDir)
	}
	if cfg.CutoffHook == nil {
		t.Fatal("expected non-nil CutoffHook")
	}

	cutoff := cfg.CutoffHook()
	if cutoff.IsZero() {
		t.Error("expected non-zero cutoff")
	}
}

func TestNextRunTime(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Kolkata")
	// Test 1: Before 01:00 AM IST (e.g. 00:30)
	t1 := time.Date(2026, 11, 2, 0, 30, 0, 0, loc)
	next1 := archive.NextRunTime(t1, 0)
	want1 := time.Date(2026, 11, 2, 1, 0, 0, 0, loc)
	if !next1.Equal(want1) {
		t.Errorf("NextRunTime(%v, 0) = %v, want %v", t1, next1, want1)
	}

	// Test 2: After 01:00 AM IST (e.g. 08:00)
	t2 := time.Date(2026, 11, 2, 8, 0, 0, 0, loc)
	next2 := archive.NextRunTime(t2, 0)
	want2 := time.Date(2026, 11, 3, 1, 0, 0, 0, loc)
	if !next2.Equal(want2) {
		t.Errorf("NextRunTime(%v, 0) = %v, want %v", t2, next2, want2)
	}

	// Test 3: Interval > 0
	interval := 15 * time.Minute
	next3 := archive.NextRunTime(t2, interval)
	if !next3.Equal(t2.Add(interval)) {
		t.Errorf("NextRunTime with interval = %v, want %v", next3, t2.Add(interval))
	}
}

func TestRoutine_EnvArchiveDir(t *testing.T) {
	t.Setenv("ARCHIVE_DIR", "/custom/archive")
	backend := &fakeArchiverBackend{}
	r := archive.NewRoutine(backend, archive.RoutineConfig{}, slog.Default())
	if r.Config().ArchiveDir != "/custom/archive" {
		t.Errorf("ArchiveDir = %q, want /custom/archive", r.Config().ArchiveDir)
	}
}

func TestDefaultCutoff_EnvOverride(t *testing.T) {
	t.Setenv("ARCHIVE_CUTOFF", "2026-10-15")
	c := archive.DefaultCutoff()
	if c.Format("2006-01-02") != "2026-10-15" {
		t.Errorf("cutoff = %v, want 2026-10-15", c)
	}

	t.Setenv("ARCHIVE_CUTOFF", "now")
	cNow := archive.DefaultCutoff()
	if time.Since(cNow) > time.Second {
		t.Errorf("expected near now, got %v", cNow)
	}
}

func TestRoutine_RunOnce_Success(t *testing.T) {
	backend := &fakeArchiverBackend{}
	fixedCutoff := time.Date(2026, 10, 20, 23, 59, 59, 0, time.UTC)
	cfg := archive.RoutineConfig{
		Interval:   time.Minute,
		ArchiveDir: "/tmp/test-archive",
		CutoffHook: func() time.Time { return fixedCutoff },
	}
	r := archive.NewRoutine(backend, cfg, slog.Default())

	sum, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce err: %v", err)
	}
	if sum == nil || sum.MasterFillsArchived != 5 {
		t.Errorf("unexpected summary: %+v", sum)
	}

	if backend.calls() != 1 {
		t.Errorf("calls = %d, want 1", backend.calls())
	}
	if !backend.lastCutoff.Equal(fixedCutoff) {
		t.Errorf("cutoff = %v, want %v", backend.lastCutoff, fixedCutoff)
	}
	if backend.lastDir != "/tmp/test-archive" {
		t.Errorf("dir = %q, want /tmp/test-archive", backend.lastDir)
	}
}

func TestRoutine_RunOnce_Error(t *testing.T) {
	expectedErr := errors.New("disk full")
	backend := &fakeArchiverBackend{returnErr: expectedErr}
	r := archive.NewRoutine(backend, archive.RoutineConfig{}, slog.Default())

	_, err := r.RunOnce(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Errorf("RunOnce err = %v, want %v", err, expectedErr)
	}
}

func TestRoutine_Run_ContextCancel(t *testing.T) {
	backend := &fakeArchiverBackend{}
	r := archive.NewRoutine(backend, archive.RoutineConfig{
		Interval: 10 * time.Millisecond,
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.Run(ctx)
	}()

	// Wait for initial sweep
	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run err = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("routine did not exit on context cancellation")
	}

	if backend.calls() < 1 {
		t.Errorf("expected at least 1 call, got %d", backend.calls())
	}
}

func TestRoutine_Run_Ticks(t *testing.T) {
	backend := &fakeArchiverBackend{}
	r := archive.NewRoutine(backend, archive.RoutineConfig{
		Interval: 15 * time.Millisecond,
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()

	_ = r.Run(ctx)

	// Should have initial sweep + multiple ticks (at least 2 calls total)
	if backend.calls() < 2 {
		t.Errorf("expected at least 2 archival calls, got %d", backend.calls())
	}
}
