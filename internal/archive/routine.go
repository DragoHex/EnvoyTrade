package archive

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"
)

// ArchiverBackend abstracts Archiver for periodic execution and testing.
type ArchiverBackend interface {
	Archive(ctx context.Context, cutoff time.Time, archiveDir string) (*ArchiveSummary, error)
}

// RoutineConfig holds configuration for the periodic archival routine.
type RoutineConfig struct {
	Interval   time.Duration
	ArchiveDir string
	CutoffHook func() time.Time
}

// Routine executes periodic archival sweeps in the background.
type Routine struct {
	archiver ArchiverBackend
	config   RoutineConfig
	logger   *slog.Logger
}

// DefaultCutoff returns yesterday 23:59:59.999999999 in tradingLoc (Asia/Kolkata),
// or parses ARCHIVE_CUTOFF if set (e.g. "now", YYYY-MM-DD, or RFC3339).
func DefaultCutoff() time.Time {
	if env := os.Getenv("ARCHIVE_CUTOFF"); env != "" {
		if env == "now" {
			return time.Now().In(tradingLoc)
		}
		if t, err := time.ParseInLocation("2006-01-02", env, tradingLoc); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, tradingLoc)
		}
		if t, err := time.Parse(time.RFC3339, env); err == nil {
			return t
		}
	}
	now := time.Now().In(tradingLoc)
	yesterday := now.AddDate(0, 0, -1)
	return time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 999999999, tradingLoc)
}

// NextRunTime calculates the next scheduled archival run.
// If interval is configured (> 0), it returns now + interval.
// Otherwise, it defaults to the next daily 01:00 AM IST (after MCX commodity close).
func NextRunTime(now time.Time, interval time.Duration) time.Time {
	if interval > 0 {
		return now.Add(interval)
	}
	nowIST := now.In(tradingLoc)
	nextRun := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 1, 0, 0, 0, tradingLoc)
	if !nowIST.Before(nextRun) {
		nextRun = nextRun.Add(24 * time.Hour)
	}
	return nextRun
}

// NewRoutine initializes a new periodic archival routine.
func NewRoutine(archiver ArchiverBackend, cfg RoutineConfig, logger *slog.Logger) *Routine {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Interval <= 0 {
		if envInterval := os.Getenv("ARCHIVE_INTERVAL"); envInterval != "" {
			if d, err := time.ParseDuration(envInterval); err == nil && d > 0 {
				cfg.Interval = d
			}
		}
	}
	if cfg.ArchiveDir == "" {
		cfg.ArchiveDir = os.Getenv("ARCHIVE_DIR")
		if cfg.ArchiveDir == "" {
			cfg.ArchiveDir = "/var/backups/envoytrade/archive"
		}
	}
	if cfg.CutoffHook == nil {
		cfg.CutoffHook = DefaultCutoff
	}
	return &Routine{
		archiver: archiver,
		config:   cfg,
		logger:   logger.With("component", "archive_routine"),
	}
}

// Config returns the active routine configuration.
func (r *Routine) Config() RoutineConfig {
	return r.config
}

// RunOnce executes a single archival sweep.
func (r *Routine) RunOnce(ctx context.Context) (*ArchiveSummary, error) {
	cutoff := r.config.CutoffHook()
	summary, err := r.archiver.Archive(ctx, cutoff, r.config.ArchiveDir)
	if err != nil {
		r.logger.Error("periodic archival failed", "cutoff", cutoff, "dir", r.config.ArchiveDir, "error", err)
		return nil, err
	}
	if summary != nil && (summary.MasterFillsArchived > 0 || summary.FollowerOrdersArchived > 0 || summary.OrderEventsArchived > 0) {
		r.logger.Info("periodic archival completed",
			"cutoff", summary.CutoffDate,
			"master_fills", summary.MasterFillsArchived,
			"follower_orders", summary.FollowerOrdersArchived,
			"order_events", summary.OrderEventsArchived,
			"partitions", summary.PartitionDates,
		)
	}
	return summary, nil
}

// Run starts the periodic archival loop and blocks until ctx is canceled.
func (r *Routine) Run(ctx context.Context) error {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("periodic archival routine panicked", "panic", rec)
		}
	}()

	scheduleDesc := "daily 01:00 AM IST"
	if r.config.Interval > 0 {
		scheduleDesc = r.config.Interval.String()
	}
	r.logger.Info("periodic archival routine started",
		"schedule", scheduleDesc,
		"dir", r.config.ArchiveDir,
	)

	// Initial sweep on startup to clear any pending backlogged orders
	if _, err := r.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Error("initial archival sweep failed", "error", err)
	}

	for {
		nextRun := NextRunTime(time.Now(), r.config.Interval)
		waitDur := time.Until(nextRun)
		r.logger.Info("scheduled next periodic archival run",
			"target_ist", nextRun.In(tradingLoc).Format("2006-01-02 15:04:05 MST"),
			"wait", waitDur.String(),
		)

		timer := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			timer.Stop()
			r.logger.Info("periodic archival routine stopped")
			return ctx.Err()
		case <-timer.C:
			if _, err := r.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				r.logger.Error("scheduled archival sweep failed", "error", err)
			}
		}
	}
}
