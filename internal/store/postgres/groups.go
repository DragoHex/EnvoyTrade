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

// Groups lists every group with follower-count and status rollup,
// for the Dashboard's GroupList (docs/APIs/groups.md).
func (s *Store) Groups(ctx context.Context) ([]domain.GroupSummary, error) {
	rows, err := s.queries.Groups(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.GroupSummary, 0, len(rows))
	for _, r := range rows {
		name := r.Name
		if name == "" {
			name = r.MasterBrokerUserID
		}
		groups = append(groups, domain.GroupSummary{
			ID:              r.ID,
			Name:            name,
			MasterID:        r.MasterID,
			MasterAccountID: r.MasterBrokerUserID,
			MasterName:      r.MasterName,
			Broker:          r.Broker,
			FollowerCount:   int(r.FollowerCount),
			Status:          statusFromAccountStatus(r.Status),
			Active:          r.Active,
		})
	}
	return groups, nil
}

// GroupDetail returns one group's master info plus its member follower rows.
// Accepts either group ID or master account ID.
func (s *Store) GroupDetail(ctx context.Context, id uuid.UUID) (domain.GroupDetail, error) {
	info, err := s.queries.GroupInfo(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GroupDetail{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.GroupDetail{}, err
	}

	name := info.Name
	if name == "" {
		name = info.MasterBrokerUserID
	}

	detail := domain.GroupDetail{
		GroupID:         info.ID,
		GroupName:       name,
		MasterID:        info.MasterID,
		MasterAccountID: info.MasterBrokerUserID,
		MasterName:      info.MasterName,
		MasterActive:    info.MasterActive,
	}

	rows, err := s.queries.GroupFollowerRows(ctx, info.ID)
	if err != nil {
		return domain.GroupDetail{}, err
	}
	for _, r := range rows {
		detail.Followers = append(detail.Followers, domain.GroupFollower{
			AccountID:       r.ID,
			Name:            r.Name,
			BrokerAccountID: r.BrokerUserID,
			Enabled:         r.Enabled,
			Status:          statusFromAccountStatus(r.Status),
		})
	}
	return detail, nil
}

// CreateGroup creates a new group.
func (s *Store) CreateGroup(ctx context.Context, id uuid.UUID, name string, masterID uuid.UUID) error {
	err := s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
		ID:       id,
		Name:     name,
		MasterID: masterID,
	})
	if isUniqueViolation(err) {
		return domain.ErrDuplicate
	}
	if isForeignKeyViolation(err) {
		return domain.ErrNotFound
	}
	return err
}

// UpdateGroup updates group name and/or master ID.
func (s *Store) UpdateGroup(ctx context.Context, id uuid.UUID, name *string, masterID *uuid.UUID) error {
	if name != nil {
		affected, err := s.queries.UpdateGroupName(ctx, sqlcgen.UpdateGroupNameParams{
			ID:   id,
			Name: *name,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return domain.ErrNotFound
		}
	}
	if masterID != nil {
		affected, err := s.queries.UpdateGroupMaster(ctx, sqlcgen.UpdateGroupMasterParams{
			ID:       id,
			MasterID: *masterID,
		})
		if err != nil {
			if isForeignKeyViolation(err) {
				return domain.ErrNotFound
			}
			return err
		}
		if affected == 0 {
			return domain.ErrNotFound
		}
	}
	return nil
}

// DeleteGroup removes a group. Returns domain.ErrConflict if followers are attached.
func (s *Store) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	affected, err := s.queries.DeleteGroup(ctx, id)
	if isForeignKeyViolation(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// GroupIDForFollower resolves the group a follower is attached to.
func (s *Store) GroupIDForFollower(ctx context.Context, followerID uuid.UUID) (uuid.UUID, error) {
	groupID, err := s.queries.GroupIDForFollower(ctx, followerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, domain.ErrNotFound
	}
	return groupID, err
}

// GroupByMasterID resolves the group whose master is masterID.
func (s *Store) GroupByMasterID(ctx context.Context, masterID uuid.UUID) (domain.Group, error) {
	g, err := s.queries.GroupByMasterID(ctx, masterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Group{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Group{}, err
	}
	return domain.Group{
		ID:        g.ID,
		Name:      g.Name,
		MasterID:  g.MasterID,
		CreatedAt: g.CreatedAt.Time,
		UpdatedAt: g.UpdatedAt.Time,
	}, nil
}

// SetFollowLinkEnabled toggles a follower's enabled flag — the Dashboard's
// "Copy" toggle. Returns domain.ErrNotFound if followerID has no
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

// ResolveMasterID finds the master account associated with an account:
// returns the account's own ID if it is already a master, or looks up its
// master via follow_links if it is a follower. Returns domain.ErrNotFound
// if the account does not exist or is a follower not linked to any group.
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

// SetAccountActive toggles a master account's active flag — the
// Dashboard's "Stop Master" / "Start Master" toggle. Returns
// domain.ErrNotFound if id has no accounts row.
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

// MasterActive reports whether the given master account is currently
// active (copying enabled). Returns domain.ErrNotFound if masterID is not
// a master account.
func (s *Store) MasterActive(ctx context.Context, masterID uuid.UUID) (bool, error) {
	active, err := s.queries.MasterActive(ctx, masterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return active, err
}

// LatestMasterFill returns the most recent master fill recorded for
// masterID — used by POST /accounts/{id}/actions to find the current
// trade to square off or rebalance against. Returns domain.ErrNotFound if
// the master has no recorded fills.
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
