package postgres

import (
	"context"
	"errors"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// statusFromAccountStatus maps the free-text accounts.status column to the
// "ok"|"error" rollup docs/APIs/groups.md documents — "active" is the only
// healthy value the schema defines today (0001_fanout.sql's DEFAULT).
func statusFromAccountStatus(accountStatus string) string {
	if accountStatus == "active" {
		return "ok"
	}
	return "error"
}

// Groups lists every master account with a follower-count and status
// rollup, for the Dashboard's GroupList (docs/APIs/groups.md).
func (s *Store) Groups(ctx context.Context) ([]domain.GroupSummary, error) {
	rows, err := s.queries.Groups(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.GroupSummary, 0, len(rows))
	for _, r := range rows {
		groups = append(groups, domain.GroupSummary{
			MasterID:        r.ID,
			MasterAccountID: r.BrokerUserID,
			Broker:          r.Broker,
			FollowerCount:   int(r.FollowerCount),
			Status:          statusFromAccountStatus(r.Status),
			Active:          r.Active,
		})
	}
	return groups, nil
}

// GroupDetail returns one master and its followers, for the Dashboard's
// GroupCard (docs/APIs/groups.md). Returns domain.ErrNotFound if masterID
// doesn't exist or isn't role=master.
func (s *Store) GroupDetail(ctx context.Context, masterID uuid.UUID) (domain.GroupDetail, error) {
	master, err := s.queries.GroupMasterInfo(ctx, masterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GroupDetail{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.GroupDetail{}, err
	}

	detail := domain.GroupDetail{
		MasterID:        masterID,
		MasterAccountID: master.BrokerUserID,
		MasterActive:    master.Active,
	}

	rows, err := s.queries.GroupFollowerRows(ctx, masterID)
	if err != nil {
		return domain.GroupDetail{}, err
	}
	for _, r := range rows {
		detail.Followers = append(detail.Followers, domain.GroupFollower{
			AccountID:       r.ID,
			BrokerAccountID: r.BrokerUserID,
			Enabled:         r.Enabled,
			Status:          statusFromAccountStatus(r.Status),
		})
	}
	return detail, nil
}

// SetFollowLinkEnabled toggles a follower's copy-trading state — the
// Dashboard's CopyToggle/Stop Copy button (docs/APIs/accounts.md's PATCH
// /accounts/{id}). Returns domain.ErrNotFound if followerID has no
// follow_link row.
func (s *Store) SetFollowLinkEnabled(ctx context.Context, followerID uuid.UUID, enabled bool) error {
	rowsAffected, err := s.queries.SetFollowLinkEnabled(ctx, sqlcgen.SetFollowLinkEnabledParams{
		FollowerID: followerID,
		Enabled:    enabled,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ResolveMasterID resolves the master an action-target account should
// fan out from: if id is itself a master, it's returned unchanged; else
// it's looked up as a follower via the 1-master-per-follower follow_link
// (follower_id is the follow_links primary key). This lets Rebalance
// work identically from a follower row or the master row. Returns
// domain.ErrNotFound if id is neither a master nor a linked follower.
func (s *Store) ResolveMasterID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	role, err := s.queries.AccountRole(ctx, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, err
	}
	if role == "master" {
		return id, nil
	}

	masterID, err := s.queries.MasterIDForFollower(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, domain.ErrNotFound
	}
	return masterID, err
}

// SetAccountActive toggles whether an account (master or follower) is
// active — the Dashboard master row's Stop/Start button (docs/APIs/
// accounts.md's PATCH /accounts/{id} "active" field). When false on a
// master, the engine skips fan-out entirely (see engine.Store.MasterActive).
// Returns domain.ErrNotFound if id has no accounts row.
func (s *Store) SetAccountActive(ctx context.Context, id uuid.UUID, active bool) error {
	rowsAffected, err := s.queries.SetAccountActive(ctx, sqlcgen.SetAccountActiveParams{
		ID:     id,
		Active: active,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// MasterActive reports whether a master account is active — the engine's
// fan-out gate. Returns domain.ErrNotFound if masterID isn't a master.
func (s *Store) MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error) {
	active, err := s.queries.MasterActive(ctx, masterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return active, err
}

// LatestMasterFill returns the most recent (by order_timestamp) master
// fill for a master account — what the Rebalance action re-dispatches
// (docs/APIs/actions.md). Returns domain.ErrNotFound if the master has no
// fills yet.
func (s *Store) LatestMasterFill(ctx context.Context, masterID uuid.UUID) (domain.MasterFill, error) {
	row, err := s.queries.LatestMasterFill(ctx, masterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MasterFill{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.MasterFill{}, err
	}
	return domain.MasterFill{
		ID:              row.ID,
		MasterID:        row.MasterID,
		BrokerOrderID:   row.BrokerOrderID,
		Exchange:        row.Exchange,
		Tradingsymbol:   row.Tradingsymbol,
		InstrumentToken: row.InstrumentToken,
		TransactionType: row.TransactionType,
		Product:         row.Product,
		OrderType:       row.OrderType,
		FilledQuantity:  int(row.FilledQuantity),
		AveragePrice:    row.AveragePrice,
		Status:          row.Status,
		OrderTimestamp:  row.OrderTimestamp.Time,
		RawPayload:      row.RawPayload,
	}, nil
}
