// Package postgres is the fan-out mechanism's persistence layer: a
// trimmed slice of PLAN.md §2 (accounts, follow_links, master_fills,
// follower_orders, order_events). account_sessions, kill_switch, and
// instruments belong to milestones not built yet.
package postgres

import (
	"context"
	"fmt"
	_ "embed"
	"errors"
	"strings"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	decimalpgx "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

//go:embed migrations/0001_init.sql
var initSchema string

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
// columns directly into shopspring/decimal.Decimal — clone_factor and
// every price column in this schema depend on it (PLAN.md §4.2 mandates
// decimal, never float64, for money and ratio math).
func NewPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		decimalpgx.Register(conn.TypeMap())
		if _, err := conn.Exec(ctx, "SET TIME ZONE 'Asia/Kolkata';"); err != nil {
			return fmt.Errorf("set timezone to Asia/Kolkata: %w", err)
		}
		return nil
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Migrate applies the database schema. Every statement is guarded
// (IF NOT EXISTS / duplicate_object) so it's safe to call on every process
// startup (cmd/server) as well as against a fresh database (tests).
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, initSchema); err != nil {
		return fmt.Errorf("0001_init: %w", err)
	}
	return nil
}

// CreateAccount is a fixture helper: the auth/onboarding flow that
// normally populates accounts is a separate milestone, but follow_links
// and master_fills both carry FK references to accounts.id. apiSecret is
// that account's own Kite Connect app secret — every account (master and
// each follower) has its own app, needed to verify that account's
// postback checksums.
func (s *Store) CreateAccount(ctx context.Context, id uuid.UUID, name string, role string, broker string, brokerUserID string, apiKey string, apiSecret string, ipAddress string) error {
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	err := s.queries.CreateAccount(ctx, sqlcgen.CreateAccountParams{
		ID:           id,
		Name:         name,
		Role:         role,
		Broker:       broker,
		BrokerUserID: brokerUserID,
		ApiKey:       apiKey,
		ApiSecret:    apiSecret,
		IpAddress:    ipAddress,
		UserID:       userID,
	})
	if isIPUniqueViolation(err) {
		return domain.ErrIPAlreadyAssigned
	}
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	return err
}

// CreateAccountWithCredentials creates an account with encrypted credentials.
func (s *Store) CreateAccountWithCredentials(ctx context.Context, id uuid.UUID, name string, role string, broker string, brokerUserID string, apiKey string, apiSecret string, ipAddress string, encPassword, encTotpSecret string) error {
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	err := s.queries.CreateAccountWithCredentials(ctx, sqlcgen.CreateAccountWithCredentialsParams{
		ID:                  id,
		Name:                name,
		Role:                role,
		Broker:              broker,
		BrokerUserID:        brokerUserID,
		ApiKey:              apiKey,
		ApiSecret:           apiSecret,
		IpAddress:           ipAddress,
		EncryptedPassword:   encPassword,
		EncryptedTotpSecret: encTotpSecret,
		UserID:              userID,
	})
	if isIPUniqueViolation(err) {
		return domain.ErrIPAlreadyAssigned
	}
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	return err
}

// SetAccountEncryptedCredentials updates encrypted password and TOTP secret.
func (s *Store) SetAccountEncryptedCredentials(ctx context.Context, id uuid.UUID, encPassword, encTotpSecret string) error {
	n, err := s.queries.SetAccountEncryptedCredentials(ctx, sqlcgen.SetAccountEncryptedCredentialsParams{
		ID:                  id,
		EncryptedPassword:   encPassword,
		EncryptedTotpSecret: encTotpSecret,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetAccountAccessToken stores the active access token and token status.
func (s *Store) SetAccountAccessToken(ctx context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	var exp pgtype.Timestamptz
	if expiresAt != nil {
		exp = pgtype.Timestamptz{Time: *expiresAt, Valid: true}
	}
	n, err := s.queries.SetAccountAccessToken(ctx, sqlcgen.SetAccountAccessTokenParams{
		ID:             id,
		AccessToken:    token,
		TokenExpiresAt: exp,
		AuthStatus:     authStatus,
		AuthError:      authError,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// AccountAuthInfo retrieves an account's authentication details and credentials.
func (s *Store) AccountAuthInfo(ctx context.Context, id uuid.UUID) (domain.AccountAuthInfo, error) {
	row, err := s.queries.AccountAuthInfo(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AccountAuthInfo{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AccountAuthInfo{}, err
	}
	var expiresAt *time.Time
	if row.TokenExpiresAt.Valid {
		t := row.TokenExpiresAt.Time.In(domain.IST)
		expiresAt = &t
	}
	return domain.AccountAuthInfo{
		ID:                  row.ID,
		Role:                row.Role,
		Broker:              row.Broker,
		BrokerAccountID:     row.BrokerUserID,
		ApiKey:              row.ApiKey,
		ApiSecret:           row.ApiSecret,
		IPAddress:           row.IpAddress,
		EncryptedPassword:   row.EncryptedPassword,
		EncryptedTotpSecret: row.EncryptedTotpSecret,
		AccessToken:         row.AccessToken,
		TokenExpiresAt:      expiresAt,
		AuthStatus:          row.AuthStatus,
		AuthError:           row.AuthError,
	}, nil
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
				var userID *uuid.UUID
				if u, ok := domain.UserFromContext(ctx); ok {
					userID = &u.ID
				}
				_ = s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
					ID:       groupID,
					Name:     link.MasterID.String(),
					MasterID: link.MasterID,
					UserID:   userID,
				})
			}
		}
	}
	err := s.queries.CreateFollowLink(ctx, sqlcgen.CreateFollowLinkParams{
		FollowerID:     link.FollowerID,
		GroupID:        groupID,
		CloneFactor:    link.CloneFactor,
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
			CloneFactor:    r.CloneFactor,
			MaxQtyPerOrder: int(r.MaxQtyPerOrder),
			Enabled:        r.Enabled,
			EffectiveFrom:  r.EffectiveFrom.Time.In(domain.IST),
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

// MasterFillExists checks if a master fill already exists in the database.
func (s *Store) MasterFillExists(ctx context.Context, masterID uuid.UUID, brokerOrderID string, filledQty int, status string) (bool, error) {
	return s.queries.MasterFillExists(ctx, sqlcgen.MasterFillExistsParams{
		MasterID:       masterID,
		BrokerOrderID:  brokerOrderID,
		FilledQuantity: int32(filledQty),
		Status:         status,
	})
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
	var masterFillID *int64
	if o.MasterFillID != 0 {
		masterFillID = &o.MasterFillID
	}
	origin := o.Origin
	if origin == "" {
		origin = "copy_trade"
	}
	id, err := s.queries.InsertFollowerOrder(ctx, sqlcgen.InsertFollowerOrderParams{
		MasterFillID:    masterFillID,
		FollowerID:      o.FollowerID,
		IdempotencyTag:  o.IdempotencyTag,
		IntendedQty:     int32(o.IntendedQty),
		LotSize:         int32(o.LotSize),
		SizingReason:    int32(o.SizingReason),
		Origin:          origin,
		Tradingsymbol:   o.Tradingsymbol,
		Exchange:        o.Exchange,
		Product:         o.Product,
		TransactionType: o.TransactionType,
		OrderType:       o.OrderType,
	})
	if isUniqueViolation(err) {
		return 0, domain.ErrDuplicate
	}
	return id, err
}

// InsertDirectFollowerOrder persists an external/manual or dashboard-initiated follower order,
// using a deterministic tag or unique constraint on (follower_id, broker_order_id) to ensure idempotency.
func (s *Store) InsertDirectFollowerOrder(ctx context.Context, upd domain.OrderUpdate) (int64, error) {
	tag := upd.Tag
	if tag == "" {
		tag = "ext-" + upd.BrokerOrderID
	}
	origin := "manual"
	if strings.HasPrefix(tag, "sqoff-") {
		origin = "square_off"
	} else if strings.HasPrefix(tag, "rebal-") {
		origin = "rebalance"
	}
	var brokerOrderID *string
	if upd.BrokerOrderID != "" {
		brokerOrderID = &upd.BrokerOrderID
	}
	var terminalStatus *string
	if upd.Status != "" {
		terminalStatus = &upd.Status
	}
	qty := upd.Quantity
	if qty <= 0 {
		qty = upd.FilledQuantity
	}
	placedQty := int32(qty)

	id, err := s.queries.InsertDirectFollowerOrder(ctx, sqlcgen.InsertDirectFollowerOrderParams{
		FollowerID:      upd.FollowerID,
		BrokerOrderID:   brokerOrderID,
		IdempotencyTag:  tag,
		IntendedQty:     int32(qty),
		PlacedQty:       &placedQty,
		FilledQty:       int32(upd.FilledQuantity),
		LotSize:         1,
		SizingReason:    int32(domain.ReasonOK),
		TerminalStatus:  terminalStatus,
		AveragePrice:    decimal.NullDecimal{Decimal: upd.AveragePrice, Valid: !upd.AveragePrice.IsZero()},
		Origin:          origin,
		Tradingsymbol:   upd.Tradingsymbol,
		Exchange:        upd.Exchange,
		Product:         upd.Product,
		TransactionType: upd.TransactionType,
		OrderType:       upd.OrderType,
	})
	if isUniqueViolation(err) {
		return 0, domain.ErrDuplicate
	}
	return id, err
}

// UpdateFollowerOrderByTag resolves an in-flight worker order whose postback arrived before
// the worker placement write.
func (s *Store) UpdateFollowerOrderByTag(ctx context.Context, followerID uuid.UUID, tag, brokerOrderID, status string, filledQty int, avgPrice decimal.Decimal) (int64, error) {
	var bID *string
	if brokerOrderID != "" {
		bID = &brokerOrderID
	}
	var termStatus *string
	if status != "" {
		termStatus = &status
	}
	id, err := s.queries.UpdateFollowerOrderByTag(ctx, sqlcgen.UpdateFollowerOrderByTagParams{
		FollowerID:     followerID,
		IdempotencyTag: tag,
		BrokerOrderID:  bID,
		TerminalStatus: termStatus,
		FilledQty:      int32(filledQty),
		AveragePrice:   decimal.NullDecimal{Decimal: avgPrice, Valid: !avgPrice.IsZero()},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return id, err
}

// FollowerOrderTerminalExists checks if a follower order already exists in terminal state.
func (s *Store) FollowerOrderTerminalExists(ctx context.Context, followerID uuid.UUID, brokerOrderID string) (bool, error) {
	bID := &brokerOrderID
	return s.queries.FollowerOrderTerminalExists(ctx, sqlcgen.FollowerOrderTerminalExistsParams{
		FollowerID:    followerID,
		BrokerOrderID: bID,
	})
}

// GetFollowerOrder fetches one follower_order by id.
func (s *Store) GetFollowerOrder(ctx context.Context, id int64) (domain.FollowerOrder, error) {
	row, err := s.queries.GetFollowerOrder(ctx, id)
	if err != nil {
		return domain.FollowerOrder{}, err
	}
	var masterFillID int64
	if row.MasterFillID != nil {
		masterFillID = *row.MasterFillID
	}
	o := domain.FollowerOrder{
		ID:              row.ID,
		MasterFillID:    masterFillID,
		FollowerID:      row.FollowerID,
		IdempotencyTag:  row.IdempotencyTag,
		IntendedQty:     int(row.IntendedQty),
		LotSize:         int(row.LotSize),
		SizingReason:    domain.SizingReason(row.SizingReason),
		FilledQty:       int(row.FilledQty),
		AttemptCount:    int(row.AttemptCount),
		Origin:          row.Origin,
		Tradingsymbol:   row.Tradingsymbol,
		Exchange:        row.Exchange,
		Product:         row.Product,
		TransactionType: row.TransactionType,
		OrderType:       row.OrderType,
		CreatedAt:       row.CreatedAt.Time.In(domain.IST),
		UpdatedAt:       row.UpdatedAt.Time.In(domain.IST),
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
	mfID := masterFillID
	rows, err := s.queries.FollowerOrdersByMasterFill(ctx, &mfID)
	if err != nil {
		return nil, err
	}
	orders := make([]domain.FollowerOrder, 0, len(rows))
	for _, r := range rows {
		orders = append(orders, domain.FollowerOrder{ID: r.ID, FollowerID: r.FollowerID})
	}
	return orders, nil
}

// StashPendingOrderUpdate records an early postback that arrived before
// the follower_orders row received its broker_order_id.
func (s *Store) StashPendingOrderUpdate(ctx context.Context, brokerOrderID, status string, filledQty int, avgPrice decimal.Decimal, rawPayload []byte) error {
	query := `
		INSERT INTO pending_order_updates (broker_order_id, status, filled_quantity, average_price, raw_payload, received_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (broker_order_id) DO UPDATE SET
			status = EXCLUDED.status,
			filled_quantity = EXCLUDED.filled_quantity,
			average_price = EXCLUDED.average_price,
			raw_payload = EXCLUDED.raw_payload,
			received_at = now()`
	_, err := s.pool.Exec(ctx, query, brokerOrderID, status, filledQty, avgPrice, rawPayload)
	return err
}

// UpdateFollowerOrderPlaced records a successful broker placement.
// If an early postback was stashed in pending_order_updates, it is consumed in the
// exact same transaction, applying terminal status and appending the audit event in 0ms.
func (s *Store) UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	placedQty32 := int32(placedQty)
	updateQuery := `
		UPDATE follower_orders
		SET broker_order_id = $2, placed_qty = $3, attempt_count = attempt_count + 1, updated_at = now()
		WHERE id = $1`
	if _, err := tx.Exec(ctx, updateQuery, id, brokerOrderID, placedQty32); err != nil {
		return fmt.Errorf("update follower_orders placed: %w", err)
	}

	// Check if postback arrived early and was stashed
	consumeQuery := `
		DELETE FROM pending_order_updates
		WHERE broker_order_id = $1
		RETURNING status, filled_quantity, average_price, raw_payload`
	var (
		status     string
		filledQty  int32
		avgPrice   *decimal.Decimal
		rawPayload []byte
	)
	row := tx.QueryRow(ctx, consumeQuery, brokerOrderID)
	err = row.Scan(&status, &filledQty, &avgPrice, &rawPayload)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check pending_order_updates: %w", err)
	}

	if err == nil {
		applyQuery := `
			UPDATE follower_orders
			SET terminal_status = $2, filled_qty = $3, average_price = $4, updated_at = now()
			WHERE id = $1`
		if _, err := tx.Exec(ctx, applyQuery, id, status, filledQty, avgPrice); err != nil {
			return fmt.Errorf("apply stashed follower_orders status: %w", err)
		}

		var followerID uuid.UUID
		var masterFillID int64
		if err := tx.QueryRow(ctx, `SELECT follower_id, master_fill_id FROM follower_orders WHERE id = $1`, id).Scan(&followerID, &masterFillID); err == nil {
			eventQuery := `
				INSERT INTO order_events (follower_order_id, master_fill_id, account_id, event_type, payload)
				VALUES ($1, $2, $3, 'status_update', $4)`
			_, _ = tx.Exec(ctx, eventQuery, id, masterFillID, followerID, rawPayload)
		}
	}

	return tx.Commit(ctx)
}

// SweepPendingOrderUpdates deletes pending order updates older than cutoff, returning
// the deleted broker order IDs for alerting.
func (s *Store) SweepPendingOrderUpdates(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := s.pool.Query(ctx, `DELETE FROM pending_order_updates WHERE received_at < $1 RETURNING broker_order_id`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deleted []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		deleted = append(deleted, id)
	}
	return deleted, rows.Err()
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
			OccurredAt:      r.OccurredAt.Time.In(domain.IST),
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

func isIPUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "idx_accounts_ip_address_unique"
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
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	rows, err := s.queries.Accounts(ctx, sqlcgen.AccountsParams{
		Ids:    ids,
		UserID: userID,
	})
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
			ApiKey:          r.ApiKey,
			ApiSecret:       r.ApiSecret,
			Active:          r.Active,
			Status:          statusFromAccountStatus(r.Status),
			IPAddress:       r.IpAddress,
			AuthStatus:      r.AuthStatus,
			AuthError:       r.AuthError,
			Enabled:         r.Enabled,
			GroupName:       r.GroupName,
			GroupID:         r.GroupID,
		}
		if r.Role == "follower" {
			a.MasterID = r.MasterID
		}
		if r.CloneFactor.Valid {
			a.CloneFactor = &r.CloneFactor.Decimal
		}
		if r.MaxQtyPerOrder != nil {
			maxQty := int(*r.MaxQtyPerOrder)
			a.MaxQtyPerOrder = &maxQty
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// UpdateFollowLinkTerms updates a follower's clone factor and max
// quantity per order — the Accounts page's edit form. Returns
// domain.ErrNotFound if followerID has no follow_link row.
func (s *Store) UpdateFollowLinkTerms(ctx context.Context, followerID uuid.UUID, cloneFactor decimal.Decimal, maxQtyPerOrder *int) error {
	rowsAffected, err := s.queries.UpdateFollowLinkTerms(ctx, sqlcgen.UpdateFollowLinkTermsParams{
		FollowerID:     followerID,
		CloneFactor:    cloneFactor,
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

// SetAccountStatus updates an account's status column.
// Returns domain.ErrNotFound if id has no accounts row, or domain.ErrInvalidAccountStatus if status is invalid.
func (s *Store) SetAccountStatus(ctx context.Context, id uuid.UUID, status domain.AccountStatus) error {
	if !status.IsValid() {
		return domain.ErrInvalidAccountStatus
	}
	rowsAffected, err := s.queries.SetAccountStatus(ctx, sqlcgen.SetAccountStatusParams{
		ID:     id,
		Status: string(status),
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

// SetAccountIPAddress updates an account's IP address (IPv4 or IPv6).
func (s *Store) SetAccountIPAddress(ctx context.Context, id uuid.UUID, ip string) error {
	rowsAffected, err := s.queries.SetAccountIPAddress(ctx, sqlcgen.SetAccountIPAddressParams{
		ID:        id,
		IpAddress: ip,
	})
	if isIPUniqueViolation(err) {
		return domain.ErrIPAlreadyAssigned
	}
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetAccountAPIKey updates an account's Kite Connect API key.
func (s *Store) SetAccountAPIKey(ctx context.Context, id uuid.UUID, apiKey string) error {
	rowsAffected, err := s.queries.SetAccountAPIKey(ctx, sqlcgen.SetAccountAPIKeyParams{
		ID:     id,
		ApiKey: apiKey,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetAccountAPISecret updates an account's Kite Connect API secret.
func (s *Store) SetAccountAPISecret(ctx context.Context, id uuid.UUID, apiSecret string) error {
	rowsAffected, err := s.queries.SetAccountAPISecret(ctx, sqlcgen.SetAccountAPISecretParams{
		ID:        id,
		ApiSecret: apiSecret,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// PendingFollowerOrders returns follower orders still awaiting terminal status created before cutoff.
func (s *Store) PendingFollowerOrders(ctx context.Context, cutoff time.Time) ([]domain.FollowerOrder, error) {
	rows, err := s.queries.PendingFollowerOrders(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		return nil, err
	}
	orders := make([]domain.FollowerOrder, 0, len(rows))
	for _, r := range rows {
		orders = append(orders, toFollowerOrder(r))
	}
	return orders, nil
}
