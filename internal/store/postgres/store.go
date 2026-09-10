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
	"envoytrade/internal/store/postgres/sqlcgen"

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

//go:embed migrations/0003_account_api_secret.sql
var accountAPISecretSchema string

//go:embed migrations/0004_account_active.sql
var accountActiveSchema string

//go:embed migrations/0005_groups_and_names.sql
var groupsAndNamesSchema string

const uniqueViolation = "23505"
const foreignKeyViolation = "23503"

// Store wraps a pgx connection pool with the sqlc-generated queries the
// fan-out mechanism needs.
type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// New wraps an already-connected pool. The pool must have been created
// with NewPool (or otherwise have the shopspring/decimal codec
// registered) — plain pgxpool.New leaves numeric columns unscannable
// into domain's decimal.Decimal fields.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, queries: sqlcgen.New(pool)}
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

// Migrate applies the fan-out schema. Every statement is guarded
// (IF NOT EXISTS / duplicate_object) so it's safe to call on every process
// startup (cmd/server) as well as against a fresh database (tests).
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, fanoutSchema); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, instrumentsSchema); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, accountAPISecretSchema); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, accountActiveSchema); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, groupsAndNamesSchema)
	return err
}

// CreateAccount is a fixture helper: the auth/onboarding flow that
// normally populates accounts is a separate milestone, but follow_links
// and master_fills both carry FK references to accounts.id. apiSecret is
// that account's own Kite Connect app secret — every account (master and
// each follower) has its own app, needed to verify that account's
// postback checksums.
func (s *Store) CreateAccount(ctx context.Context, id uuid.UUID, name string, role string, broker string, brokerUserID string, apiSecret string) error {
	err := s.queries.CreateAccount(ctx, sqlcgen.CreateAccountParams{
		ID:           id,
		Name:         name,
		Role:         role,
		Broker:       broker,
		BrokerUserID: brokerUserID,
		ApiSecret:    apiSecret,
	})
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	if err != nil {
		return err
	}
	if role == "master" {
		groupName := name
		if groupName == "" {
			groupName = brokerUserID
		}
		_ = s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
			ID:       id,
			Name:     groupName,
			MasterID: id,
		})
	}
	return nil
}

// AccountByBrokerUserID resolves the account a postback's user_id
// belongs to. Returns domain.ErrNotFound if no account has this
// broker_user_id.
func (s *Store) AccountByBrokerUserID(ctx context.Context, brokerUserID string) (uuid.UUID, string, string, error) {
	row, err := s.queries.AccountByBrokerUserID(ctx, brokerUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, "", "", domain.ErrNotFound
	}
	return row.ID, row.Role, row.ApiSecret, err
}

// CreateFollowLink is a fixture helper for tests and, later, onboarding
// — and the Accounts page's "attach account to group" action. Returns
// domain.ErrDuplicate if the follower is already attached to a group
// (follower_id is the follow_links primary key).
func (s *Store) CreateFollowLink(ctx context.Context, link domain.FollowLink) error {
	groupID := link.GroupID
	if groupID == uuid.Nil {
		if link.MasterID != uuid.Nil {
			g, err := s.queries.GroupByMasterID(ctx, link.MasterID)
			if err == nil {
				groupID = g.ID
			} else {
				groupID = link.MasterID
				_ = s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
					ID:       groupID,
					Name:     link.MasterID.String(),
					MasterID: link.MasterID,
				})
			}
		}
	}
	err := s.queries.CreateFollowLink(ctx, sqlcgen.CreateFollowLinkParams{
		FollowerID:     link.FollowerID,
		GroupID:        groupID,
		CapitalRatio:   link.CapitalRatio,
		MaxQtyPerOrder: nullableMaxQty(link.MaxQtyPerOrder),
		Enabled:        link.Enabled,
	})
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	if isForeignKeyViolation(err) {
		return domain.ErrConflict
	}
	return err
}

func nullableMaxQty(v int) *int32 {
	if v <= 0 {
		return nil
	}
	v32 := int32(v)
	return &v32
}

// EnabledFollowLinks returns every enabled follow_link for the given
// master, the set the engine fans a master fill out to.
func (s *Store) EnabledFollowLinks(ctx context.Context, masterID uuid.UUID) ([]domain.FollowLink, error) {
	rows, err := s.queries.EnabledFollowLinks(ctx, masterID)
	if err != nil {
		return nil, err
	}
	links := make([]domain.FollowLink, 0, len(rows))
	for _, r := range rows {
		links = append(links, domain.FollowLink{
			FollowerID:     r.FollowerID,
			GroupID:        r.GroupID,
			MasterID:       r.MasterID,
			CapitalRatio:   r.CapitalRatio,
			MaxQtyPerOrder: int(r.MaxQtyPerOrder),
			Enabled:        r.Enabled,
			EffectiveFrom:  r.EffectiveFrom.Time,
		})
	}
	return links, nil
}

// InsertMasterFill persists a master fill. A duplicate WS redelivery
// collides on the unique (master_id, broker_order_id, filled_quantity,
// status) index and returns domain.ErrDuplicate rather than a new row.
func (s *Store) InsertMasterFill(ctx context.Context, f domain.MasterFill) (int64, error) {
	id, err := s.queries.InsertMasterFill(ctx, sqlcgen.InsertMasterFillParams{
		MasterID:        f.MasterID,
		BrokerOrderID:   f.BrokerOrderID,
		Exchange:        f.Exchange,
		Tradingsymbol:   f.Tradingsymbol,
		InstrumentToken: f.InstrumentToken,
		TransactionType: f.TransactionType,
		Product:         f.Product,
		OrderType:       f.OrderType,
		FilledQuantity:  int32(f.FilledQuantity),
		AveragePrice:    f.AveragePrice,
		Status:          f.Status,
		OrderTimestamp:  pgtimestamptz(f.OrderTimestamp),
		RawPayload:      f.RawPayload,
		DispatchState:   string(domain.DispatchPending),
	})
	if isUniqueViolation(err) {
		return 0, domain.ErrDuplicate
	}
	return id, err
}

// SetMasterFillDispatchState transitions a master fill through the
// outbox states (PLAN.md §4.1).
func (s *Store) SetMasterFillDispatchState(ctx context.Context, id int64, state domain.DispatchState) error {
	return s.queries.SetMasterFillDispatchState(ctx, sqlcgen.SetMasterFillDispatchStateParams{
		ID:            id,
		DispatchState: string(state),
	})
}

// InsertFollowerOrder persists one follower_order row, created before
// any broker call is attempted. A duplicate idempotency_tag or a
// duplicate (master_fill_id, follower_id) pair returns domain.ErrDuplicate.
func (s *Store) InsertFollowerOrder(ctx context.Context, o domain.FollowerOrder) (int64, error) {
	id, err := s.queries.InsertFollowerOrder(ctx, sqlcgen.InsertFollowerOrderParams{
		MasterFillID:   o.MasterFillID,
		FollowerID:     o.FollowerID,
		IdempotencyTag: o.IdempotencyTag,
		IntendedQty:    int32(o.IntendedQty),
		LotSize:        int32(o.LotSize),
		SizingReason:   int32(o.SizingReason),
	})
	if isUniqueViolation(err) {
		return 0, domain.ErrDuplicate
	}
	return id, err
}

// GetFollowerOrder fetches one follower_order by id.
func (s *Store) GetFollowerOrder(ctx context.Context, id int64) (domain.FollowerOrder, error) {
	row, err := s.queries.GetFollowerOrder(ctx, id)
	if err != nil {
		return domain.FollowerOrder{}, err
	}
	o := domain.FollowerOrder{
		ID:             row.ID,
		MasterFillID:   row.MasterFillID,
		FollowerID:     row.FollowerID,
		IdempotencyTag: row.IdempotencyTag,
		IntendedQty:    int(row.IntendedQty),
		LotSize:        int(row.LotSize),
		SizingReason:   domain.SizingReason(row.SizingReason),
		FilledQty:      int(row.FilledQty),
		AttemptCount:   int(row.AttemptCount),
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
	if row.PlacedQty != nil {
		placedQty := int(*row.PlacedQty)
		o.PlacedQty = &placedQty
	}
	if row.BrokerOrderID != nil {
		o.BrokerOrderID = *row.BrokerOrderID
	}
	if row.TerminalStatus != nil {
		o.TerminalStatus = *row.TerminalStatus
	}
	if row.LastError != nil {
		o.LastError = *row.LastError
	}
	if row.AveragePrice.Valid {
		o.AveragePrice = row.AveragePrice.Decimal
	}
	return o, nil
}

// FollowerOrdersByMasterFill returns every follower_order fanned out from
// one master fill — used to assert redelivery doesn't create duplicates.
func (s *Store) FollowerOrdersByMasterFill(ctx context.Context, masterFillID int64) ([]domain.FollowerOrder, error) {
	rows, err := s.queries.FollowerOrdersByMasterFill(ctx, masterFillID)
	if err != nil {
		return nil, err
	}
	orders := make([]domain.FollowerOrder, 0, len(rows))
	for _, r := range rows {
		orders = append(orders, domain.FollowerOrder{ID: r.ID, FollowerID: r.FollowerID})
	}
	return orders, nil
}

// UpdateFollowerOrderPlaced records a successful broker placement.
func (s *Store) UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error {
	placedQty32 := int32(placedQty)
	return s.queries.UpdateFollowerOrderPlaced(ctx, sqlcgen.UpdateFollowerOrderPlacedParams{
		ID:            id,
		BrokerOrderID: &brokerOrderID,
		PlacedQty:     &placedQty32,
	})
}

// UpdateFollowerOrderFailed records a terminal failure — no retry in
// this slice (PLAN.md §4.4's retry policy is a separate milestone).
func (s *Store) UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error {
	return s.queries.UpdateFollowerOrderFailed(ctx, sqlcgen.UpdateFollowerOrderFailedParams{
		ID:             id,
		TerminalStatus: &terminalStatus,
		LastError:      &errMsg,
	})
}

// UpdateFollowerOrderStatus records a status update delivered by
// postback/WS for an order already placed, located by broker_order_id
// (never by id — the caller doesn't know it, only the broker's order
// id). Returns the updated row's id and domain.ErrNotFound if no
// follower_order has this broker_order_id yet (the postback can race the
// worker's own UpdateFollowerOrderPlaced write).
func (s *Store) UpdateFollowerOrderStatus(ctx context.Context, brokerOrderID string, status string, filledQty int, averagePrice decimal.Decimal) (int64, error) {
	id, err := s.queries.UpdateFollowerOrderStatus(ctx, sqlcgen.UpdateFollowerOrderStatusParams{
		BrokerOrderID:  &brokerOrderID,
		TerminalStatus: &status,
		FilledQty:      int32(filledQty),
		AveragePrice:   decimal.NullDecimal{Decimal: averagePrice, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return id, err
}

// AppendOrderEvent writes one immutable transition-log row — the SEBI
// audit trail (PLAN.md §2).
func (s *Store) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	return s.queries.AppendOrderEvent(ctx, sqlcgen.AppendOrderEventParams{
		FollowerOrderID: ev.FollowerOrderID,
		MasterFillID:    ev.MasterFillID,
		AccountID:       ev.AccountID,
		EventType:       ev.EventType,
		Payload:         ev.Payload,
	})
}

// OrderEventsByFollowerOrder returns every event for a follower_order, in
// insertion order — the sequence a SEBI/exchange audit query would ask
// for.
func (s *Store) OrderEventsByFollowerOrder(ctx context.Context, followerOrderID int64) ([]domain.OrderEvent, error) {
	rows, err := s.queries.OrderEventsByFollowerOrder(ctx, &followerOrderID)
	if err != nil {
		return nil, err
	}
	events := make([]domain.OrderEvent, 0, len(rows))
	for _, r := range rows {
		events = append(events, domain.OrderEvent{
			ID:              r.ID,
			FollowerOrderID: r.FollowerOrderID,
			MasterFillID:    r.MasterFillID,
			AccountID:       r.AccountID,
			EventType:       r.EventType,
			Payload:         r.Payload,
			OccurredAt:      r.OccurredAt.Time,
		})
	}
	return events, nil
}

// UpsertInstrument inserts or refreshes one row of the instrument master.
// Real deployments call this from the daily sync job (PLAN.md §3.1);
// tests use it to seed the lot sizes fan-out depends on.
func (s *Store) UpsertInstrument(ctx context.Context, ins domain.Instrument) error {
	segment := ins.Segment
	return s.queries.UpsertInstrument(ctx, sqlcgen.UpsertInstrumentParams{
		InstrumentToken: ins.InstrumentToken,
		Exchange:        ins.Exchange,
		Tradingsymbol:   ins.Tradingsymbol,
		LotSize:         int32(ins.LotSize),
		TickSize:        ins.TickSize,
		Segment:         &segment,
		Expiry:          pgdate(ins.Expiry),
	})
}

// InstrumentLotSize resolves the canonical lot size for an
// (exchange, tradingsymbol) pair. Returns domain.ErrNotFound if the
// instrument master has no matching row — e.g. the daily sync job hasn't
// picked it up yet, or the symbol doesn't exist.
func (s *Store) InstrumentLotSize(ctx context.Context, exchange, tradingsymbol string) (int, error) {
	lotSize, err := s.queries.InstrumentLotSize(ctx, sqlcgen.InstrumentLotSizeParams{
		Exchange:      exchange,
		Tradingsymbol: tradingsymbol,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return int(lotSize), err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

// AccountRole reports whether id is a master or a follower. Returns
// domain.ErrNotFound if id has no accounts row.
func (s *Store) AccountRole(ctx context.Context, id uuid.UUID) (string, error) {
	role, err := s.queries.AccountRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return role, err
}

// Accounts lists every account (master and follower), joined with its
// follow_link (if any) — the flat, ungrouped view the Accounts page's
// list/detail needs. ids filters to just those accounts; nil returns all.
func (s *Store) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	rows, err := s.queries.Accounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	accounts := make([]domain.Account, 0, len(rows))
	for _, r := range rows {
		a := domain.Account{
			ID:              r.ID,
			Name:            r.Name,
			Role:            r.Role,
			Broker:          r.Broker,
			BrokerAccountID: r.BrokerUserID,
			Active:          r.Active,
			Status:          r.Status,
			Enabled:         r.Enabled,
			GroupName:       r.GroupName,
		}
		if r.GroupID.Valid {
			groupID := uuid.UUID(r.GroupID.Bytes)
			a.GroupID = &groupID
		}
		if r.MasterID.Valid {
			masterID := uuid.UUID(r.MasterID.Bytes)
			a.MasterID = &masterID
		}
		if r.CapitalRatio.Valid {
			a.CapitalRatio = &r.CapitalRatio.Decimal
		}
		if r.MaxQtyPerOrder != nil {
			maxQty := int(*r.MaxQtyPerOrder)
			a.MaxQtyPerOrder = &maxQty
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// UpdateFollowLinkTerms updates a follower's capital ratio and max
// quantity per order — the Accounts page's edit form. Returns
// domain.ErrNotFound if followerID has no follow_link row.
func (s *Store) UpdateFollowLinkTerms(ctx context.Context, followerID uuid.UUID, capitalRatio decimal.Decimal, maxQtyPerOrder *int) error {
	rowsAffected, err := s.queries.UpdateFollowLinkTerms(ctx, sqlcgen.UpdateFollowLinkTermsParams{
		FollowerID:     followerID,
		CapitalRatio:   capitalRatio,
		MaxQtyPerOrder: nullableMaxQtyPtr(maxQtyPerOrder),
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func nullableMaxQtyPtr(v *int) *int32 {
	if v == nil {
		return nil
	}
	v32 := int32(*v)
	return &v32
}

// SetAccountStatus updates an account's free-text status column — the
// Accounts page's edit form. Returns domain.ErrNotFound if id has no
// accounts row.
func (s *Store) SetAccountStatus(ctx context.Context, id uuid.UUID, status string) error {
	rowsAffected, err := s.queries.SetAccountStatus(ctx, sqlcgen.SetAccountStatusParams{
		ID:     id,
		Status: status,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteAccount removes an account row. Returns domain.ErrConflict if the
// account is still referenced (a follow_link, master_fill, or
// follower_order FK) — Postgres's own constraints are the source of
// truth here rather than re-implementing referential checks in Go.
// Returns domain.ErrNotFound if id has no accounts row.
func (s *Store) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	rowsAffected, err := s.queries.DeleteAccount(ctx, id)
	if isForeignKeyViolation(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteFollowLink detaches a follower from its group without deleting
// the account itself — the Accounts page's "remove from group" action.
// Returns domain.ErrNotFound if followerID has no follow_link row.
func (s *Store) DeleteFollowLink(ctx context.Context, followerID uuid.UUID) error {
	rowsAffected, err := s.queries.DeleteFollowLink(ctx, followerID)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetAccountName updates an account's name.
func (s *Store) SetAccountName(ctx context.Context, id uuid.UUID, name string) error {
	rowsAffected, err := s.queries.SetAccountName(ctx, sqlcgen.SetAccountNameParams{
		ID:   id,
		Name: name,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
