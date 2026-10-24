package archive

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

var tradingLoc *time.Location

func init() {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		tradingLoc = time.UTC
	} else {
		tradingLoc = loc
	}
}

// SetTradingLocation allows overriding the location (useful for tests).
func SetTradingLocation(loc *time.Location) {
	if loc != nil {
		tradingLoc = loc
	}
}

// Archiver handles offloading terminal orders to Parquet files and purging them from PostgreSQL.
type Archiver struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewArchiver creates a new Archiver instance.
func NewArchiver(pool *pgxpool.Pool, logger *slog.Logger) *Archiver {
	if logger == nil {
		logger = slog.Default()
	}
	return &Archiver{
		pool:   pool,
		logger: logger,
	}
}

// Archive extracts terminal orders created on or before cutoff, writes them to Hive-partitioned
// Parquet files, and deletes them from PostgreSQL within an atomic transaction.
func (a *Archiver) Archive(ctx context.Context, cutoff time.Time, archiveDir string) (*ArchiveSummary, error) {
	summary := &ArchiveSummary{
		CutoffDate: cutoff.In(tradingLoc).Format("2006-01-02 15:04:05 MST"),
	}

	if archiveDir == "" {
		archiveDir = "/var/backups/envoytrade/archive"
	}

	// 1. Fetch terminal follower orders
	followerOrders, err := a.fetchTerminalFollowerOrders(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("fetch follower orders: %w", err)
	}

	foIDs := make([]int64, len(followerOrders))
	for i, r := range followerOrders {
		foIDs[i] = r.ID
	}

	// 2. Fetch master fills ready to archive (must have no non-terminal follower orders remaining)
	masterFills, err := a.fetchEligibleMasterFills(ctx, cutoff, foIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch master fills: %w", err)
	}

	mfIDs := make([]int64, len(masterFills))
	for i, r := range masterFills {
		mfIDs[i] = r.ID
	}

	// 3. Fetch order events ready to archive (all events for foIDs and mfIDs)
	orderEvents, err := a.fetchEligibleOrderEvents(ctx, cutoff, foIDs, mfIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch order events: %w", err)
	}

	if len(followerOrders) == 0 && len(masterFills) == 0 && len(orderEvents) == 0 {
		a.logger.Info("no eligible terminal orders to archive", "cutoff", cutoff)
		return summary, nil
	}

	// 4. Partition and write Parquet files to disk
	partitionDatesMap := make(map[string]struct{})
	recordPartitions := func(dates, files []string) {
		for _, d := range dates {
			partitionDatesMap[d] = struct{}{}
		}
		summary.FilesWritten = append(summary.FilesWritten, files...)
	}

	if d, f, err := writeTablePartitions(archiveDir, "follower_orders", followerOrders,
		func(r FollowerOrderParquet) int64 { return r.CreatedAt },
		func(r FollowerOrderParquet) int64 { return r.ID },
	); err != nil {
		return nil, err
	} else {
		recordPartitions(d, f)
	}

	if d, f, err := writeTablePartitions(archiveDir, "master_fills", masterFills,
		func(r MasterFillParquet) int64 { return r.ReceivedAt },
		func(r MasterFillParquet) int64 { return r.ID },
	); err != nil {
		return nil, err
	} else {
		recordPartitions(d, f)
	}

	if d, f, err := writeTablePartitions(archiveDir, "order_events", orderEvents,
		func(r OrderEventParquet) int64 { return r.OccurredAt },
		func(r OrderEventParquet) int64 { return r.ID },
	); err != nil {
		return nil, err
	} else {
		recordPartitions(d, f)
	}

	// 5. Atomic PostgreSQL delete in reverse Foreign Key order
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// A. Delete order_events
	if len(orderEvents) > 0 {
		oeIDs := make([]int64, len(orderEvents))
		for i, r := range orderEvents {
			oeIDs[i] = r.ID
		}
		cmdTag, err := tx.Exec(ctx, "DELETE FROM order_events WHERE id = ANY($1)", oeIDs)
		if err != nil {
			return nil, fmt.Errorf("delete order_events: %w", err)
		}
		summary.OrderEventsArchived = int(cmdTag.RowsAffected())
	}

	// B. Delete follower_orders
	if len(followerOrders) > 0 {
		foIDs := make([]int64, len(followerOrders))
		for i, r := range followerOrders {
			foIDs[i] = r.ID
		}
		cmdTag, err := tx.Exec(ctx, "DELETE FROM follower_orders WHERE id = ANY($1)", foIDs)
		if err != nil {
			return nil, fmt.Errorf("delete follower_orders: %w", err)
		}
		summary.FollowerOrdersArchived = int(cmdTag.RowsAffected())
	}

	// C. Delete master_fills
	if len(masterFills) > 0 {
		mfIDs := make([]int64, len(masterFills))
		for i, r := range masterFills {
			mfIDs[i] = r.ID
		}
		cmdTag, err := tx.Exec(ctx, "DELETE FROM master_fills WHERE id = ANY($1)", mfIDs)
		if err != nil {
			return nil, fmt.Errorf("delete master_fills: %w", err)
		}
		summary.MasterFillsArchived = int(cmdTag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit purge transaction: %w", err)
	}

	for d := range partitionDatesMap {
		summary.PartitionDates = append(summary.PartitionDates, d)
	}
	sort.Strings(summary.PartitionDates)

	a.logger.Info("archive complete",
		"follower_orders", summary.FollowerOrdersArchived,
		"master_fills", summary.MasterFillsArchived,
		"order_events", summary.OrderEventsArchived,
		"partitions", summary.PartitionDates,
	)

	return summary, nil
}

func (a *Archiver) fetchTerminalFollowerOrders(ctx context.Context, cutoff time.Time) ([]FollowerOrderParquet, error) {
	query := `
		SELECT 
			id, master_fill_id, follower_id::text, idempotency_tag, intended_qty,
			lot_size, sizing_reason, placed_qty, broker_order_id, terminal_status,
			filled_qty, average_price::text, attempt_count, last_error, origin,
			tradingsymbol, exchange, product, transaction_type, order_type,
			created_at, updated_at
		FROM follower_orders
		WHERE created_at <= $1
		  AND (terminal_status IS NOT NULL OR intended_qty = 0)
		ORDER BY id ASC;
	`
	rows, err := a.pool.Query(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FollowerOrderParquet
	for rows.Next() {
		var (
			r                                FollowerOrderParquet
			createdAt, updatedAt             time.Time
			avgPrice                         *string
			placedQty                        *int
			attemptCount, filledQty, intQty  int
			lotSize, sizingReason            int
		)
		err := rows.Scan(
			&r.ID, &r.MasterFillID, &r.FollowerID, &r.IdempotencyTag, &intQty,
			&lotSize, &sizingReason, &placedQty, &r.BrokerOrderID, &r.TerminalStatus,
			&filledQty, &avgPrice, &attemptCount, &r.LastError, &r.Origin,
			&r.Tradingsymbol, &r.Exchange, &r.Product, &r.TransactionType, &r.OrderType,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan follower order: %w", err)
		}

		r.IntendedQty = int32(intQty)
		r.LotSize = int32(lotSize)
		r.SizingReason = int32(sizingReason)
		r.FilledQty = int32(filledQty)
		r.AttemptCount = int32(attemptCount)
		if placedQty != nil {
			pq := int32(*placedQty)
			r.PlacedQty = &pq
		}
		r.AveragePrice = avgPrice
		r.CreatedAt = TimeToMicros(createdAt)
		r.UpdatedAt = TimeToMicros(updatedAt)

		result = append(result, r)
	}

	return result, rows.Err()
}

func (a *Archiver) fetchEligibleMasterFills(ctx context.Context, cutoff time.Time, foIDs []int64) ([]MasterFillParquet, error) {
	query := `
		SELECT 
			id, master_id::text, broker_order_id, exchange, tradingsymbol,
			instrument_token, transaction_type, product, order_type, filled_quantity,
			average_price::text, status, order_timestamp, raw_payload::text, received_at,
			dispatch_state, dispatched_at
		FROM master_fills mf
		WHERE received_at <= $1
		  AND dispatch_state IN ('dispatched', 'dead')
		  AND NOT EXISTS (
			  SELECT 1 FROM follower_orders fo
			  WHERE fo.master_fill_id = mf.id
			    AND fo.id != ALL($2)
		  )
		ORDER BY id ASC;
	`
	rows, err := a.pool.Query(ctx, query, cutoff, foIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MasterFillParquet
	for rows.Next() {
		var (
			r                       MasterFillParquet
			filledQty               int
			orderTimestamp, recvAt  time.Time
			dispatchedAt            *time.Time
		)
		err := rows.Scan(
			&r.ID, &r.MasterID, &r.BrokerOrderID, &r.Exchange, &r.Tradingsymbol,
			&r.InstrumentToken, &r.TransactionType, &r.Product, &r.OrderType, &filledQty,
			&r.AveragePrice, &r.Status, &orderTimestamp, &r.RawPayload, &recvAt,
			&r.DispatchState, &dispatchedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan master fill: %w", err)
		}

		r.FilledQuantity = int32(filledQty)
		r.OrderTimestamp = TimeToMicros(orderTimestamp)
		r.ReceivedAt = TimeToMicros(recvAt)
		if dispatchedAt != nil {
			da := TimeToMicros(*dispatchedAt)
			r.DispatchedAt = &da
		}

		result = append(result, r)
	}

	return result, rows.Err()
}

func (a *Archiver) fetchEligibleOrderEvents(
	ctx context.Context,
	cutoff time.Time,
	foIDs []int64,
	mfIDs []int64,
) ([]OrderEventParquet, error) {
	if len(foIDs) == 0 && len(mfIDs) == 0 {
		return nil, nil
	}

	query := `
		SELECT 
			id, follower_order_id, master_fill_id, account_id::text, event_type,
			from_status, to_status, http_status, latency_ms, payload::text, occurred_at
		FROM order_events
		WHERE follower_order_id = ANY($1)
		   OR (follower_order_id IS NULL AND master_fill_id = ANY($2))
		ORDER BY id ASC;
	`
	rows, err := a.pool.Query(ctx, query, foIDs, mfIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OrderEventParquet
	for rows.Next() {
		var (
			r           OrderEventParquet
			httpStatus  *int
			latencyMS   *int
			occurredAt  time.Time
		)
		err := rows.Scan(
			&r.ID, &r.FollowerOrderID, &r.MasterFillID, &r.AccountID, &r.EventType,
			&r.FromStatus, &r.ToStatus, &httpStatus, &latencyMS, &r.Payload, &occurredAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan order event: %w", err)
		}

		if httpStatus != nil {
			hs := int32(*httpStatus)
			r.HTTPStatus = &hs
		}
		if latencyMS != nil {
			lms := int32(*latencyMS)
			r.LatencyMS = &lms
		}
		r.OccurredAt = TimeToMicros(occurredAt)

		result = append(result, r)
	}

	return result, rows.Err()
}

func groupRowsByDate[T any](rows []T, timeMicros func(T) int64) map[string][]T {
	m := make(map[string][]T)
	for _, r := range rows {
		t := MicrosToTime(timeMicros(r)).In(tradingLoc)
		d := t.Format("2006-01-02")
		m[d] = append(m[d], r)
	}
	return m
}

func writeTablePartitions[T any](
	archiveDir string,
	tableName string,
	rows []T,
	timeMicros func(T) int64,
	idFunc func(T) int64,
) ([]string, []string, error) {
	byDate := groupRowsByDate(rows, timeMicros)
	var dates []string
	var files []string
	for d, partitionRows := range byDate {
		dates = append(dates, d)
		dir := filepath.Join(archiveDir, tableName, fmt.Sprintf("date=%s", d))
		file := filepath.Join(dir, "data.parquet")
		if err := writeParquetWithMerge(file, partitionRows, idFunc); err != nil {
			return nil, nil, fmt.Errorf("write %s parquet (%s): %w", tableName, file, err)
		}
		files = append(files, file)
	}
	return dates, files, nil
}

// writeParquetWithMerge writes rows to a Parquet file, creating directories if needed.
// If the target file already exists, it merges new rows with existing rows by ID.
func writeParquetWithMerge[T any](filePath string, newRows []T, idFunc func(T) int64) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create partition dir: %w", err)
	}

	mergedRows := newRows
	if _, err := os.Stat(filePath); err == nil {
		// File already exists -> read existing and merge
		existingRows, err := parquet.ReadFile[T](filePath)
		if err != nil {
			return fmt.Errorf("read existing parquet file %s for merge: %w", filePath, err)
		}
		existingMap := make(map[int64]T, len(existingRows))
		for _, r := range existingRows {
			existingMap[idFunc(r)] = r
		}
		for _, r := range newRows {
			existingMap[idFunc(r)] = r
		}
		mergedRows = make([]T, 0, len(existingMap))
		for _, r := range existingMap {
			mergedRows = append(mergedRows, r)
		}
		sort.Slice(mergedRows, func(i, j int) bool {
			return idFunc(mergedRows[i]) < idFunc(mergedRows[j])
		})
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat parquet file %s: %w", filePath, err)
	}

	// Atomic write via temp file
	tmpFile := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	if err := parquet.WriteFile(tmpFile, mergedRows, parquet.Compression(&zstd.Codec{})); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("write parquet file: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename parquet temp file: %w", err)
	}

	return nil
}
