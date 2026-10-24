package archive

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parquet-go/parquet-go"
)

// Restorer handles restoring archived trade execution data from Parquet files into PostgreSQL.
type Restorer struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewRestorer creates a new Restorer instance.
func NewRestorer(pool *pgxpool.Pool, logger *slog.Logger) *Restorer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Restorer{
		pool:   pool,
		logger: logger,
	}
}

// Restore selectively loads Parquet trade data within the date range [fromDate, uptoDate]
// into PostgreSQL using idempotent inserts and resynchronizes primary key sequences.
func (r *Restorer) Restore(ctx context.Context, archiveDir string, fromDate *time.Time, uptoDate *time.Time) (*RestoreSummary, error) {
	summary := &RestoreSummary{}
	if fromDate != nil {
		summary.FromDate = fromDate.In(tradingLoc).Format("2006-01-02")
	}
	if uptoDate != nil {
		summary.UptoDate = uptoDate.In(tradingLoc).Format("2006-01-02")
	}

	if fromDate != nil && uptoDate != nil && fromDate.After(*uptoDate) {
		return nil, fmt.Errorf("invalid date range: from date (%s) cannot be after upto date (%s)", summary.FromDate, summary.UptoDate)
	}

	if archiveDir == "" {
		archiveDir = "/var/backups/envoytrade/archive"
	}

	// 1. Discover all matching date partitions across tables
	partitions, err := r.discoverDatePartitions(archiveDir, summary.FromDate, summary.UptoDate)
	if err != nil {
		return nil, fmt.Errorf("discover partitions: %w", err)
	}

	if len(partitions) == 0 {
		r.logger.Info("no matching parquet partitions found to restore",
			"archiveDir", archiveDir,
			"from", summary.FromDate,
			"upto", summary.UptoDate,
		)
		return summary, nil
	}

	summary.PartitionsRestored = partitions

	// 2. Forward Foreign Key Order Restore
	// Step A: Restore master_fills
	mfCount, err := r.restoreMasterFills(ctx, archiveDir, partitions)
	if err != nil {
		return nil, fmt.Errorf("restore master_fills: %w", err)
	}
	summary.MasterFillsRestored = mfCount

	// Step B: Restore follower_orders
	foCount, err := r.restoreFollowerOrders(ctx, archiveDir, partitions)
	if err != nil {
		return nil, fmt.Errorf("restore follower_orders: %w", err)
	}
	summary.FollowerOrdersRestored = foCount

	// Step C: Restore order_events
	oeCount, err := r.restoreOrderEvents(ctx, archiveDir, partitions)
	if err != nil {
		return nil, fmt.Errorf("restore order_events: %w", err)
	}
	summary.OrderEventsRestored = oeCount

	// 3. Resynchronize PostgreSQL serial sequences
	if err := r.resynchronizeSequences(ctx); err != nil {
		return nil, fmt.Errorf("resynchronize sequences: %w", err)
	}

	r.logger.Info("restore complete",
		"master_fills", summary.MasterFillsRestored,
		"follower_orders", summary.FollowerOrdersRestored,
		"order_events", summary.OrderEventsRestored,
		"partitions", summary.PartitionsRestored,
	)

	return summary, nil
}

func (r *Restorer) discoverDatePartitions(archiveDir, fromDate, uptoDate string) ([]string, error) {
	dateSet := make(map[string]struct{})
	tables := []string{"master_fills", "follower_orders", "order_events"}

	for _, tbl := range tables {
		tblDir := filepath.Join(archiveDir, tbl)
		entries, err := os.ReadDir(tblDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, "date=") {
				continue
			}
			dateStr := strings.TrimPrefix(name, "date=")
			if fromDate != "" && dateStr < fromDate {
				continue
			}
			if uptoDate != "" && dateStr > uptoDate {
				continue
			}
			dateSet[dateStr] = struct{}{}
		}
	}

	dates := make([]string, 0, len(dateSet))
	for d := range dateSet {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	return dates, nil
}

func (r *Restorer) restoreMasterFills(ctx context.Context, archiveDir string, partitions []string) (int, error) {
	var totalRestored int
	for _, d := range partitions {
		file := filepath.Join(archiveDir, "master_fills", fmt.Sprintf("date=%s", d), "data.parquet")
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return totalRestored, fmt.Errorf("stat %s: %w", file, err)
		}

		rows, err := parquet.ReadFile[MasterFillParquet](file)
		if err != nil {
			return totalRestored, fmt.Errorf("read %s: %w", file, err)
		}

		if len(rows) == 0 {
			continue
		}

		batch := &pgx.Batch{}
		query := `
			INSERT INTO master_fills (
				id, master_id, broker_order_id, exchange, tradingsymbol,
				instrument_token, transaction_type, product, order_type, filled_quantity,
				average_price, status, order_timestamp, raw_payload, received_at,
				dispatch_state, dispatched_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15,
				$16, $17
			) ON CONFLICT (id) DO NOTHING;
		`
		for _, row := range rows {
			var dispAt *time.Time
			if row.DispatchedAt != nil {
				t := MicrosToTime(*row.DispatchedAt)
				dispAt = &t
			}
			batch.Queue(query,
				row.ID,
				row.MasterID,
				row.BrokerOrderID,
				row.Exchange,
				row.Tradingsymbol,
				row.InstrumentToken,
				row.TransactionType,
				row.Product,
				row.OrderType,
				row.FilledQuantity,
				row.AveragePrice,
				row.Status,
				MicrosToTime(row.OrderTimestamp),
				row.RawPayload,
				MicrosToTime(row.ReceivedAt),
				row.DispatchState,
				dispAt,
			)
		}

		affected, err := execBatch(ctx, r.pool, batch, len(rows), "master fill")
		if err != nil {
			return totalRestored, err
		}
		totalRestored += affected
	}
	return totalRestored, nil
}

func (r *Restorer) restoreFollowerOrders(ctx context.Context, archiveDir string, partitions []string) (int, error) {
	var totalRestored int
	for _, d := range partitions {
		file := filepath.Join(archiveDir, "follower_orders", fmt.Sprintf("date=%s", d), "data.parquet")
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return totalRestored, fmt.Errorf("stat %s: %w", file, err)
		}

		rows, err := parquet.ReadFile[FollowerOrderParquet](file)
		if err != nil {
			return totalRestored, fmt.Errorf("read %s: %w", file, err)
		}

		if len(rows) == 0 {
			continue
		}

		batch := &pgx.Batch{}
		query := `
			INSERT INTO follower_orders (
				id, master_fill_id, follower_id, idempotency_tag, intended_qty,
				lot_size, sizing_reason, placed_qty, broker_order_id, terminal_status,
				filled_qty, average_price, attempt_count, last_error, origin,
				tradingsymbol, exchange, product, transaction_type, order_type,
				created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12, $13, $14, $15,
				$16, $17, $18, $19, $20,
				$21, $22
			) ON CONFLICT (id) DO NOTHING;
		`
		for _, row := range rows {
			var placedQty *int
			if row.PlacedQty != nil {
				pq := int(*row.PlacedQty)
				placedQty = &pq
			}
			batch.Queue(query,
				row.ID,
				row.MasterFillID,
				row.FollowerID,
				row.IdempotencyTag,
				row.IntendedQty,
				row.LotSize,
				row.SizingReason,
				placedQty,
				row.BrokerOrderID,
				row.TerminalStatus,
				row.FilledQty,
				row.AveragePrice,
				row.AttemptCount,
				row.LastError,
				row.Origin,
				row.Tradingsymbol,
				row.Exchange,
				row.Product,
				row.TransactionType,
				row.OrderType,
				MicrosToTime(row.CreatedAt),
				MicrosToTime(row.UpdatedAt),
			)
		}

		affected, err := execBatch(ctx, r.pool, batch, len(rows), "follower order")
		if err != nil {
			return totalRestored, err
		}
		totalRestored += affected
	}
	return totalRestored, nil
}

func (r *Restorer) restoreOrderEvents(ctx context.Context, archiveDir string, partitions []string) (int, error) {
	var totalRestored int
	for _, d := range partitions {
		file := filepath.Join(archiveDir, "order_events", fmt.Sprintf("date=%s", d), "data.parquet")
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return totalRestored, fmt.Errorf("stat %s: %w", file, err)
		}

		rows, err := parquet.ReadFile[OrderEventParquet](file)
		if err != nil {
			return totalRestored, fmt.Errorf("read %s: %w", file, err)
		}

		if len(rows) == 0 {
			continue
		}

		batch := &pgx.Batch{}
		query := `
			INSERT INTO order_events (
				id, follower_order_id, master_fill_id, account_id, event_type,
				from_status, to_status, http_status, latency_ms, payload, occurred_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10, $11
			) ON CONFLICT (id) DO NOTHING;
		`
		for _, row := range rows {
			var httpStatus *int
			if row.HTTPStatus != nil {
				hs := int(*row.HTTPStatus)
				httpStatus = &hs
			}
			var latencyMS *int
			if row.LatencyMS != nil {
				lms := int(*row.LatencyMS)
				latencyMS = &lms
			}
			batch.Queue(query,
				row.ID,
				row.FollowerOrderID,
				row.MasterFillID,
				row.AccountID,
				row.EventType,
				row.FromStatus,
				row.ToStatus,
				httpStatus,
				latencyMS,
				row.Payload,
				MicrosToTime(row.OccurredAt),
			)
		}

		affected, err := execBatch(ctx, r.pool, batch, len(rows), "order event")
		if err != nil {
			return totalRestored, err
		}
		totalRestored += affected
	}
	return totalRestored, nil
}

func execBatch(ctx context.Context, pool *pgxpool.Pool, batch *pgx.Batch, expectedCount int, entityName string) (int, error) {
	br := pool.SendBatch(ctx, batch)
	var total int
	for i := 0; i < expectedCount; i++ {
		cmdTag, err := br.Exec()
		if err != nil {
			br.Close()
			return total, fmt.Errorf("insert %s: %w", entityName, err)
		}
		total += int(cmdTag.RowsAffected())
	}
	return total, br.Close()
}

func (r *Restorer) resynchronizeSequences(ctx context.Context) error {
	queries := []string{
		`SELECT setval('master_fills_id_seq', GREATEST(COALESCE((SELECT MAX(id) FROM master_fills), 1), 1));`,
		`SELECT setval('follower_orders_id_seq', GREATEST(COALESCE((SELECT MAX(id) FROM follower_orders), 1), 1));`,
		`SELECT setval('order_events_id_seq', GREATEST(COALESCE((SELECT MAX(id) FROM order_events), 1), 1));`,
	}

	for _, q := range queries {
		if _, err := r.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("exec %s: %w", q, err)
		}
	}
	return nil
}
