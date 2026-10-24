//go:build integration

package archive_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"envoytrade/internal/archive"
	"envoytrade/internal/store/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("envoytrade_archive_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := postgres.NewPool(ctx, connStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	store := postgres.New(pool)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func setupBaseAccounts(t *testing.T, pool *pgxpool.Pool) (masterID, followerID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, username, password_hash)
		VALUES ($1, 'trader@envoytrade.com', 'trader', 'hash')
	`, userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	masterID = uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO accounts (id, user_id, role, broker, broker_user_id, name, active)
		VALUES ($1, $2, 'master', 'zerodha', 'M123', 'Master 1', true)
	`, masterID, userID)
	if err != nil {
		t.Fatalf("insert master account: %v", err)
	}

	followerID = uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO accounts (id, user_id, role, broker, broker_user_id, name, active)
		VALUES ($1, $2, 'follower', 'zerodha', 'F123', 'Follower 1', true)
	`, followerID, userID)
	if err != nil {
		t.Fatalf("insert follower account: %v", err)
	}

	groupID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO groups (id, user_id, name, master_id)
		VALUES ($1, $2, 'Main Group', $3)
	`, groupID, userID, masterID)
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO follow_links (follower_id, group_id, clone_factor, enabled)
		VALUES ($1, $2, 1.0, true)
	`, followerID, groupID)
	if err != nil {
		t.Fatalf("insert follow link: %v", err)
	}

	return masterID, followerID
}

func TestArchiveAndRestore_EndToEnd(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	archiveDir := t.TempDir()

	masterID, followerID := setupBaseAccounts(t, pool)

	// Create test dates
	loc, _ := time.LoadLocation("Asia/Kolkata")
	archive.SetTradingLocation(loc)

	d1 := time.Date(2026, 10, 21, 11, 0, 0, 0, loc)
	d2 := time.Date(2026, 10, 22, 14, 0, 0, 0, loc)
	dOpen := time.Date(2026, 10, 22, 15, 0, 0, 0, loc)

	// 1. Seed Master Fills
	// mf1: terminal on d1
	var mf1ID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO master_fills (
			master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
			transaction_type, product, order_type, filled_quantity, average_price,
			status, order_timestamp, raw_payload, received_at, dispatch_state
		) VALUES (
			$1, 'M_ORD_1', 'NFO', 'NIFTY26OCT25000CE', 1001,
			'BUY', 'NRML', 'MARKET', 50, 100.0,
			'COMPLETE', $2, '{}', $2, 'dispatched'
		) RETURNING id
	`, masterID, d1).Scan(&mf1ID)
	if err != nil {
		t.Fatalf("insert mf1: %v", err)
	}

	// mf2: terminal on d2
	var mf2ID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO master_fills (
			master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
			transaction_type, product, order_type, filled_quantity, average_price,
			status, order_timestamp, raw_payload, received_at, dispatch_state
		) VALUES (
			$1, 'M_ORD_2', 'NFO', 'BANKNIFTY26OCT50000CE', 1002,
			'BUY', 'NRML', 'MARKET', 30, 250.0,
			'COMPLETE', $2, '{}', $2, 'dispatched'
		) RETURNING id
	`, masterID, d2).Scan(&mf2ID)
	if err != nil {
		t.Fatalf("insert mf2: %v", err)
	}

	// mfOpen: associated with an open order (must NOT be archived or deleted!)
	var mfOpenID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO master_fills (
			master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
			transaction_type, product, order_type, filled_quantity, average_price,
			status, order_timestamp, raw_payload, received_at, dispatch_state
		) VALUES (
			$1, 'M_ORD_OPEN', 'NSE', 'TCS', 1003,
			'BUY', 'CNC', 'LIMIT', 10, 3500.0,
			'COMPLETE', $2, '{}', $2, 'dispatched'
		) RETURNING id
	`, masterID, dOpen).Scan(&mfOpenID)
	if err != nil {
		t.Fatalf("insert mfOpen: %v", err)
	}

	// 2. Seed Follower Orders
	// fo1: terminal on d1
	var fo1ID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO follower_orders (
			master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size,
			sizing_reason, placed_qty, broker_order_id, terminal_status, filled_qty,
			average_price, origin, tradingsymbol, exchange, product, transaction_type,
			order_type, created_at, updated_at
		) VALUES (
			$1, $2, 'tag-1', 50, 25, 0, 50, 'F_ORD_1', 'COMPLETE', 50, 100.25,
			'copy_trade', 'NIFTY26OCT25000CE', 'NFO', 'NRML', 'BUY', 'MARKET', $3, $3
		) RETURNING id
	`, mf1ID, followerID, d1).Scan(&fo1ID)
	if err != nil {
		t.Fatalf("insert fo1: %v", err)
	}

	// fo2: terminal on d2
	var fo2ID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO follower_orders (
			master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size,
			sizing_reason, placed_qty, broker_order_id, terminal_status, filled_qty,
			average_price, origin, tradingsymbol, exchange, product, transaction_type,
			order_type, created_at, updated_at
		) VALUES (
			$1, $2, 'tag-2', 30, 15, 0, 30, 'F_ORD_2', 'COMPLETE', 30, 250.50,
			'copy_trade', 'BANKNIFTY26OCT50000CE', 'NFO', 'NRML', 'BUY', 'MARKET', $3, $3
		) RETURNING id
	`, mf2ID, followerID, d2).Scan(&fo2ID)
	if err != nil {
		t.Fatalf("insert fo2: %v", err)
	}

	// foOpen: OPEN status (must NOT be touched)
	var foOpenID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO follower_orders (
			master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size,
			sizing_reason, placed_qty, broker_order_id, terminal_status, filled_qty,
			origin, tradingsymbol, exchange, product, transaction_type,
			order_type, created_at, updated_at
		) VALUES (
			$1, $2, 'tag-open', 10, 1, 0, 10, 'F_ORD_OPEN', NULL, 0,
			'copy_trade', 'TCS', 'NSE', 'CNC', 'BUY', 'LIMIT', $3, $3
		) RETURNING id
	`, mfOpenID, followerID, dOpen).Scan(&foOpenID)
	if err != nil {
		t.Fatalf("insert foOpen: %v", err)
	}

	// 3. Seed Order Events
	_, err = pool.Exec(ctx, `
		INSERT INTO order_events (follower_order_id, master_fill_id, account_id, event_type, payload, occurred_at)
		VALUES 
			($1, $2, $3, 'order_placed', '{}', $4),
			($5, $6, $3, 'order_placed', '{}', $7),
			($8, $9, $3, 'order_open', '{}', $10)
	`, fo1ID, mf1ID, followerID, d1, fo2ID, mf2ID, d2, foOpenID, mfOpenID, dOpen)
	if err != nil {
		t.Fatalf("insert order_events: %v", err)
	}

	// ---------------------------------------------------------
	// STEP 1: ARCHIVE TEST
	// ---------------------------------------------------------
	archiver := archive.NewArchiver(pool, nil)
	cutoff := time.Date(2026, 10, 22, 23, 59, 59, 0, loc)

	summary, err := archiver.Archive(ctx, cutoff, archiveDir)
	if err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	if summary.FollowerOrdersArchived != 2 {
		t.Errorf("expected 2 follower orders archived, got %d", summary.FollowerOrdersArchived)
	}
	if summary.MasterFillsArchived != 2 {
		t.Errorf("expected 2 master fills archived, got %d", summary.MasterFillsArchived)
	}
	if summary.OrderEventsArchived != 2 {
		t.Errorf("expected 2 order events archived, got %d", summary.OrderEventsArchived)
	}

	// Verify Parquet file existence
	p1 := filepath.Join(archiveDir, "follower_orders", "date=2026-10-21", "data.parquet")
	p2 := filepath.Join(archiveDir, "follower_orders", "date=2026-10-22", "data.parquet")
	if _, err := os.Stat(p1); err != nil {
		t.Errorf("expected parquet file at %s, got: %v", p1, err)
	}
	if _, err := os.Stat(p2); err != nil {
		t.Errorf("expected parquet file at %s, got: %v", p2, err)
	}

	// Verify PostgreSQL State: Terminal orders deleted, OPEN order preserved!
	var foCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM follower_orders").Scan(&foCount)
	if foCount != 1 {
		t.Errorf("expected exactly 1 follower order remaining (the OPEN one), got %d", foCount)
	}

	var remainingFoID int64
	var remStatus *string
	_ = pool.QueryRow(ctx, "SELECT id, terminal_status FROM follower_orders").Scan(&remainingFoID, &remStatus)
	if remainingFoID != foOpenID {
		t.Errorf("expected remaining order to be %d, got %d", foOpenID, remainingFoID)
	}
	if remStatus != nil {
		t.Errorf("expected remaining order status to be nil (OPEN), got %v", *remStatus)
	}

	var mfCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM master_fills").Scan(&mfCount)
	if mfCount != 1 {
		t.Errorf("expected exactly 1 master fill remaining (mfOpen), got %d", mfCount)
	}

	var oeCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM order_events").Scan(&oeCount)
	if oeCount != 1 {
		t.Errorf("expected exactly 1 order event remaining, got %d", oeCount)
	}

	// ---------------------------------------------------------
	// STEP 2: RESTORE TEST (Selective Date Range: 2026-10-21 only)
	// ---------------------------------------------------------
	restorer := archive.NewRestorer(pool, nil)
	fromD1 := time.Date(2026, 10, 21, 0, 0, 0, 0, loc)
	uptoD1 := time.Date(2026, 10, 21, 23, 59, 59, 0, loc)

	resSummary, err := restorer.Restore(ctx, archiveDir, &fromD1, &uptoD1)
	if err != nil {
		t.Fatalf("selective restore failed: %v", err)
	}

	if resSummary.FollowerOrdersRestored != 1 {
		t.Errorf("expected 1 follower order restored for 2026-10-21, got %d", resSummary.FollowerOrdersRestored)
	}

	// Verify DB has 2 follower orders now (d1 restored + dOpen)
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM follower_orders").Scan(&foCount)
	if foCount != 2 {
		t.Errorf("expected 2 follower orders after selective restore, got %d", foCount)
	}

	// ---------------------------------------------------------
	// STEP 3: IDEMPOTENT RESTORE TEST (Re-run same restore)
	// ---------------------------------------------------------
	resSummary2, err := restorer.Restore(ctx, archiveDir, &fromD1, &uptoD1)
	if err != nil {
		t.Fatalf("second restore should not fail: %v", err)
	}
	// ON CONFLICT DO NOTHING: 0 rows affected
	if resSummary2.FollowerOrdersRestored != 0 {
		t.Errorf("expected 0 new follower orders on re-run, got %d", resSummary2.FollowerOrdersRestored)
	}

	// ---------------------------------------------------------
	// STEP 4: RESTORE REMAINING (2026-10-22)
	// ---------------------------------------------------------
	uptoD2 := time.Date(2026, 10, 22, 23, 59, 59, 0, loc)
	_, err = restorer.Restore(ctx, archiveDir, nil, &uptoD2)
	if err != nil {
		t.Fatalf("full restore failed: %v", err)
	}

	_ = pool.QueryRow(ctx, "SELECT count(*) FROM follower_orders").Scan(&foCount)
	if foCount != 3 {
		t.Errorf("expected 3 follower orders after full restore, got %d", foCount)
	}

	// ---------------------------------------------------------
	// STEP 5: SEQUENCE SYNCHRONIZATION TEST
	// ---------------------------------------------------------
	// Insert a new order without specifying id — must succeed without collision!
	var newOrderID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO follower_orders (
			follower_id, idempotency_tag, intended_qty, lot_size, sizing_reason, origin
		) VALUES ($1, 'tag-new-after-restore', 10, 1, 0, 'manual')
		RETURNING id
	`, followerID).Scan(&newOrderID)
	if err != nil {
		t.Fatalf("insert new follower order after restore failed (sequence desync): %v", err)
	}
	if newOrderID <= fo2ID && newOrderID <= foOpenID {
		t.Errorf("expected new order ID (%d) to be greater than all restored IDs (%d, %d)", newOrderID, fo2ID, foOpenID)
	}
}

func TestArchive_NoOrdersFound(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	archiveDir := t.TempDir()

	archiver := archive.NewArchiver(pool, nil)
	summary, err := archiver.Archive(ctx, time.Now(), archiveDir)
	if err != nil {
		t.Fatalf("expected clean exit when no orders to archive, got: %v", err)
	}
	if summary.FollowerOrdersArchived != 0 || summary.MasterFillsArchived != 0 {
		t.Errorf("expected 0 archived, got %+v", summary)
	}
}
