// Package postgres is the fan-out mechanism's persistence layer: a
// trimmed slice of PLAN.md §2 (accounts, follow_links, master_fills,
// follower_orders, order_events). account_sessions, kill_switch, and
// instruments belong to milestones not built yet.
package postgres

import (
	"context"
	_ "embed"
	"errors"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	decimalpgx "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

//go:embed migrations/0001_fanout.sql
var fanoutSchema string

//go:embed migrations/0002_instruments.sql
var instrumentsSchema string

const uniqueViolation = "23505"

// Store wraps a pgx connection pool with the queries the fan-out
// mechanism needs.
type Store struct {
	pool *pgxpool.Pool
}

// New wraps an already-connected pool. The pool must have been created
// with NewPool (or otherwise have the shopspring/decimal codec
// registered) — plain pgxpool.New leaves numeric columns unscannable
// into domain's decimal.Decimal fields.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// NewPool creates a pgx pool configured to scan Postgres `numeric`
// columns directly into shopspring/decimal.Decimal — capital_ratio and
// every price column in this schema depend on it (PLAN.md §4.2 mandates
// decimal, never float64, for money and ratio math).
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		decimalpgx.Register(conn.TypeMap())
		return nil
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Migrate applies the fan-out schema to an empty database. It is not
// idempotent — callers run it once against a fresh database (tests,
// initial provisioning).
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, fanoutSchema); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, instrumentsSchema)
	return err
}

// CreateAccount is a fixture helper: the auth/onboarding flow that
// normally populates accounts is a separate milestone, but follow_links
// and master_fills both carry FK references to accounts.id.
func (s *Store) CreateAccount(ctx context.Context, id uuid.UUID, role string, brokerUserID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO accounts (id, role, broker_user_id) VALUES ($1, $2, $3)`,
		id, role, brokerUserID)
	return err
}

// CreateFollowLink is a fixture helper for tests and, later, onboarding.
func (s *Store) CreateFollowLink(ctx context.Context, link domain.FollowLink) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO follow_links (follower_id, master_id, capital_ratio, max_qty_per_order, enabled)
		 VALUES ($1, $2, $3, $4, $5)`,
		link.FollowerID, link.MasterID, link.CapitalRatio, nullableMaxQty(link.MaxQtyPerOrder), link.Enabled)
	return err
}

func nullableMaxQty(v int) *int {
	if v <= 0 {
		return nil
	}
	return &v
}

// EnabledFollowLinks returns every enabled follow_link for the given
// master, the set the engine fans a master fill out to.
func (s *Store) EnabledFollowLinks(ctx context.Context, masterID uuid.UUID) ([]domain.FollowLink, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT follower_id, master_id, capital_ratio, COALESCE(max_qty_per_order, 0), enabled, effective_from
		 FROM follow_links WHERE master_id = $1 AND enabled = true`,
		masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []domain.FollowLink
	for rows.Next() {
		var l domain.FollowLink
		if err := rows.Scan(&l.FollowerID, &l.MasterID, &l.CapitalRatio, &l.MaxQtyPerOrder, &l.Enabled, &l.EffectiveFrom); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// InsertMasterFill persists a master fill. A duplicate WS redelivery
// collides on the unique (master_id, broker_order_id, filled_quantity,
// status) index and returns domain.ErrDuplicate rather than a new row.
func (s *Store) InsertMasterFill(ctx context.Context, f domain.MasterFill) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO master_fills
		   (master_id, broker_order_id, exchange, tradingsymbol, instrument_token,
		    transaction_type, product, order_type, filled_quantity, average_price,
		    status, order_timestamp, raw_payload, dispatch_state)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING id`,
		f.MasterID, f.BrokerOrderID, f.Exchange, f.Tradingsymbol, f.InstrumentToken,
		f.TransactionType, f.Product, f.OrderType, f.FilledQuantity, f.AveragePrice,
		f.Status, f.OrderTimestamp, f.RawPayload, string(domain.DispatchPending),
	).Scan(&id)
	if isUniqueViolation(err) {
		return 0, domain.ErrDuplicate
	}
	return id, err
}

// SetMasterFillDispatchState transitions a master fill through the
// outbox states (PLAN.md §4.1).
func (s *Store) SetMasterFillDispatchState(ctx context.Context, id int64, state domain.DispatchState) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE master_fills SET dispatch_state = $2, dispatched_at = CASE WHEN $2 = 'dispatched' THEN now() ELSE dispatched_at END
		 WHERE id = $1`,
		id, string(state))
	return err
}

// InsertFollowerOrder persists one follower_order row, created before
// any broker call is attempted. A duplicate idempotency_tag or a
// duplicate (master_fill_id, follower_id) pair returns domain.ErrDuplicate.
func (s *Store) InsertFollowerOrder(ctx context.Context, o domain.FollowerOrder) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO follower_orders
		   (master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size, sizing_reason)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING id`,
		o.MasterFillID, o.FollowerID, o.IdempotencyTag, o.IntendedQty, o.LotSize, int(o.SizingReason),
	).Scan(&id)
	if isUniqueViolation(err) {
		return 0, domain.ErrDuplicate
	}
	return id, err
}

// GetFollowerOrder fetches one follower_order by id.
func (s *Store) GetFollowerOrder(ctx context.Context, id int64) (domain.FollowerOrder, error) {
	var o domain.FollowerOrder
	var brokerOrderID, terminalStatus, lastError *string
	var placedQty *int
	var averagePrice *decimal.Decimal
	err := s.pool.QueryRow(ctx,
		`SELECT id, master_fill_id, follower_id, idempotency_tag, intended_qty, lot_size, sizing_reason,
		        placed_qty, broker_order_id, terminal_status, filled_qty, average_price, attempt_count,
		        last_error, created_at, updated_at
		 FROM follower_orders WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.MasterFillID, &o.FollowerID, &o.IdempotencyTag, &o.IntendedQty, &o.LotSize, &o.SizingReason,
		&placedQty, &brokerOrderID, &terminalStatus, &o.FilledQty, &averagePrice, &o.AttemptCount,
		&lastError, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return domain.FollowerOrder{}, err
	}
	o.PlacedQty = placedQty
	if brokerOrderID != nil {
		o.BrokerOrderID = *brokerOrderID
	}
	if terminalStatus != nil {
		o.TerminalStatus = *terminalStatus
	}
	if lastError != nil {
		o.LastError = *lastError
	}
	if averagePrice != nil {
		o.AveragePrice = *averagePrice
	}
	return o, nil
}

// FollowerOrdersByMasterFill returns every follower_order fanned out from
// one master fill — used to assert redelivery doesn't create duplicates.
func (s *Store) FollowerOrdersByMasterFill(ctx context.Context, masterFillID int64) ([]domain.FollowerOrder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, follower_id FROM follower_orders WHERE master_fill_id = $1 ORDER BY id ASC`,
		masterFillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []domain.FollowerOrder
	for rows.Next() {
		var o domain.FollowerOrder
		if err := rows.Scan(&o.ID, &o.FollowerID); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// UpdateFollowerOrderPlaced records a successful broker placement.
func (s *Store) UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE follower_orders SET broker_order_id = $2, placed_qty = $3, attempt_count = attempt_count + 1, updated_at = now()
		 WHERE id = $1`,
		id, brokerOrderID, placedQty)
	return err
}

// UpdateFollowerOrderFailed records a terminal failure — no retry in
// this slice (PLAN.md §4.4's retry policy is a separate milestone).
func (s *Store) UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE follower_orders SET terminal_status = $2, last_error = $3, attempt_count = attempt_count + 1, updated_at = now()
		 WHERE id = $1`,
		id, terminalStatus, errMsg)
	return err
}

// AppendOrderEvent writes one immutable transition-log row — the SEBI
// audit trail (PLAN.md §2).
func (s *Store) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO order_events (follower_order_id, master_fill_id, account_id, event_type, payload)
		 VALUES ($1,$2,$3,$4,$5)`,
		ev.FollowerOrderID, ev.MasterFillID, ev.AccountID, ev.EventType, ev.Payload)
	return err
}

// OrderEventsByFollowerOrder returns every event for a follower_order, in
// insertion order — the sequence a SEBI/exchange audit query would ask
// for.
func (s *Store) OrderEventsByFollowerOrder(ctx context.Context, followerOrderID int64) ([]domain.OrderEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, follower_order_id, master_fill_id, account_id, event_type, payload, occurred_at
		 FROM order_events WHERE follower_order_id = $1 ORDER BY id ASC`,
		followerOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.OrderEvent
	for rows.Next() {
		var ev domain.OrderEvent
		if err := rows.Scan(&ev.ID, &ev.FollowerOrderID, &ev.MasterFillID, &ev.AccountID, &ev.EventType, &ev.Payload, &ev.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// UpsertInstrument inserts or refreshes one row of the instrument master.
// Real deployments call this from the daily sync job (PLAN.md §3.1);
// tests use it to seed the lot sizes fan-out depends on.
func (s *Store) UpsertInstrument(ctx context.Context, ins domain.Instrument) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO instruments (instrument_token, exchange, tradingsymbol, lot_size, tick_size, segment, expiry, refreshed_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7, now())
		 ON CONFLICT (instrument_token) DO UPDATE SET
		   exchange = EXCLUDED.exchange, tradingsymbol = EXCLUDED.tradingsymbol,
		   lot_size = EXCLUDED.lot_size, tick_size = EXCLUDED.tick_size,
		   segment = EXCLUDED.segment, expiry = EXCLUDED.expiry, refreshed_at = now()`,
		ins.InstrumentToken, ins.Exchange, ins.Tradingsymbol, ins.LotSize, ins.TickSize, ins.Segment, ins.Expiry)
	return err
}

// InstrumentLotSize resolves the canonical lot size for an
// (exchange, tradingsymbol) pair. Returns domain.ErrNotFound if the
// instrument master has no matching row — e.g. the daily sync job hasn't
// picked it up yet, or the symbol doesn't exist.
func (s *Store) InstrumentLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error) {
	var lotSize int
	err := s.pool.QueryRow(ctx,
		`SELECT lot_size FROM instruments WHERE exchange = $1 AND tradingsymbol = $2`,
		exchange, tradingsymbol,
	).Scan(&lotSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return lotSize, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
