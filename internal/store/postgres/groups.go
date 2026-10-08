package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// statusFromAccountStatus maps the accounts.status column to the "ok"|"error" rollup.
func statusFromAccountStatus(accountStatus string) string {
	status, err := domain.ParseAccountStatus(accountStatus)
	if err != nil {
		return "error"
	}
	return status.ToAPI()
}

// Groups lists every group with follower-count and status rollup,
// for the Dashboard's GroupList (docs/APIs/groups.md).
func (s *Store) Groups(ctx context.Context) ([]domain.GroupSummary, error) {
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	rows, err := s.queries.Groups(ctx, userID)
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
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	info, err := s.queries.GroupInfo(ctx, sqlcgen.GroupInfoParams{ID: id, UserID: userID})
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

	accountIDs := make([]uuid.UUID, 0, 1+len(rows))
	accountIDs = append(accountIDs, info.MasterID)
	for _, r := range rows {
		accountIDs = append(accountIDs, r.ID)
	}

	type accountMetricsRow struct {
		netQty               int
		totalMtm             decimal.Decimal
		availableCash        *decimal.Decimal
		availableMargin      *decimal.Decimal
		openPositionsCount   int
		closedPositionsCount int
		openOrdersCount      int
		mtmBreakdown         map[string]decimal.Decimal
	}

	metricsMap := make(map[uuid.UUID]accountMetricsRow, len(accountIDs))
	mRows, err := s.pool.Query(ctx, `
		SELECT 
			a.id,
			COALESCE(m.net_qty, 0),
			COALESCE(m.total_mtm, 0),
			m.available_cash,
			m.available_margin,
			CASE 
				WHEN m.product_mtm IS NOT NULL AND m.product_mtm <> '{}'::jsonb THEN m.product_mtm
				ELSE COALESCE(
					(
						SELECT jsonb_object_agg(sub.product, sub.sum_mtm)
						FROM (
							SELECT product, COALESCE(SUM(mtm), 0) AS sum_mtm
							FROM account_positions
							WHERE account_id = a.id AND product != 'CNC'
							GROUP BY product
						) sub
					),
					'{}'::jsonb
				)
			END,
			(SELECT COUNT(*) FROM account_positions ap WHERE ap.account_id = a.id AND ap.quantity != 0 AND ap.product != 'CNC'),
			(SELECT COUNT(*) FROM account_positions ap WHERE ap.account_id = a.id AND ap.quantity = 0 AND ap.product != 'CNC'),
			CASE 
				WHEN a.role = 'master' THEN 
					(SELECT COUNT(*) FROM master_fills mf WHERE mf.master_id = a.id AND mf.status NOT IN ('COMPLETE', 'REJECTED', 'CANCELLED'))
				ELSE 
					(SELECT COUNT(*) FROM follower_orders fo WHERE fo.follower_id = a.id AND fo.terminal_status IS NULL AND fo.intended_qty > 0)
			END
		FROM accounts a
		LEFT JOIN account_margins m ON m.account_id = a.id
		WHERE a.id = ANY($1::uuid[])
	`, accountIDs)
	if err != nil {
		return domain.GroupDetail{}, err
	}
	defer mRows.Close()

	for mRows.Next() {
		var accID uuid.UUID
		var m accountMetricsRow
		var netQty int32
		var totalMtm decimal.Decimal
		var availCash, availMargin *decimal.Decimal
		var rawProductMtm []byte
		var openPos, closedPos, openOrders int64
		if err := mRows.Scan(
			&accID,
			&netQty,
			&totalMtm,
			&availCash,
			&availMargin,
			&rawProductMtm,
			&openPos,
			&closedPos,
			&openOrders,
		); err != nil {
			return domain.GroupDetail{}, err
		}
		m.netQty = int(netQty)
		m.totalMtm = totalMtm
		m.availableCash = availCash
		m.availableMargin = availMargin
		m.openPositionsCount = int(openPos)
		m.closedPositionsCount = int(closedPos)
		m.openOrdersCount = int(openOrders)
		if len(rawProductMtm) > 0 {
			var breakdown map[string]decimal.Decimal
			if err := json.Unmarshal(rawProductMtm, &breakdown); err == nil {
				m.mtmBreakdown = breakdown
			}
		}
		metricsMap[accID] = m
	}
	if err := mRows.Err(); err != nil {
		return domain.GroupDetail{}, err
	}

	masterM := metricsMap[info.MasterID]
	detail.MasterNetQty = masterM.netQty
	detail.MasterOpenPositionsCount = masterM.openPositionsCount
	detail.MasterClosedPositionsCount = masterM.closedPositionsCount
	detail.MasterOpenOrdersCount = masterM.openOrdersCount
	detail.MasterTotalMtm = masterM.totalMtm
	detail.MasterAvailableCash = masterM.availableCash
	detail.MasterAvailableMargin = masterM.availableMargin
	detail.MasterMtmBreakdown = masterM.mtmBreakdown

	for _, r := range rows {
		fMetrics := metricsMap[r.ID]
		detail.Followers = append(detail.Followers, domain.GroupFollower{
			AccountID:            r.ID,
			Name:                 r.Name,
			BrokerAccountID:      r.BrokerUserID,
			Enabled:              r.Enabled,
			Status:               statusFromAccountStatus(r.Status),
			NetQty:               fMetrics.netQty,
			OpenPositionsCount:   fMetrics.openPositionsCount,
			ClosedPositionsCount: fMetrics.closedPositionsCount,
			OpenOrdersCount:      fMetrics.openOrdersCount,
			TotalMtm:             fMetrics.totalMtm,
			AvailableCash:        fMetrics.availableCash,
			AvailableMargin:      fMetrics.availableMargin,
			MtmBreakdown:         fMetrics.mtmBreakdown,
		})
	}
	return detail, nil
}

// CreateGroup creates a new group.
func (s *Store) CreateGroup(ctx context.Context, id uuid.UUID, name string, masterID uuid.UUID) error {
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	err := s.queries.CreateGroup(ctx, sqlcgen.CreateGroupParams{
		ID:       id,
		Name:     name,
		MasterID: masterID,
		UserID:   userID,
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
	var userID *uuid.UUID
	if u, ok := domain.UserFromContext(ctx); ok {
		userID = &u.ID
	}
	info, err := s.queries.GroupInfo(ctx, sqlcgen.GroupInfoParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	groupID := info.ID
	currentMasterID := info.MasterID

	if name != nil {
		affected, err := s.queries.UpdateGroupName(ctx, sqlcgen.UpdateGroupNameParams{
			ID:   groupID,
			Name: *name,
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			return domain.ErrNotFound
		}
	}
	if masterID != nil && *masterID != currentMasterID {
		openCount, err := s.queries.CountOpenPositionsByAccount(ctx, currentMasterID)
		if err != nil {
			return err
		}
		if openCount > 0 {
			return domain.ErrMasterHasOpenPositions
		}

		targetOpenCount, err := s.queries.CountOpenPositionsByAccount(ctx, *masterID)
		if err != nil {
			return err
		}
		if targetOpenCount > 0 {
			return domain.ErrMasterHasOpenPositions
		}

		otherGroup, err := s.queries.GroupByMasterID(ctx, *masterID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		if err == nil {
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)

			qtx := s.queries.WithTx(tx)
			affected1, err := qtx.UpdateGroupMaster(ctx, sqlcgen.UpdateGroupMasterParams{
				ID:       groupID,
				MasterID: *masterID,
			})
			if err != nil {
				return err
			}
			if affected1 == 0 {
				return domain.ErrNotFound
			}

			affected2, err := qtx.UpdateGroupMaster(ctx, sqlcgen.UpdateGroupMasterParams{
				ID:       otherGroup.ID,
				MasterID: currentMasterID,
			})
			if err != nil {
				return err
			}
			if affected2 == 0 {
				return domain.ErrNotFound
			}

			if err := tx.Commit(ctx); err != nil {
				return err
			}
		} else {
			affected, err := s.queries.UpdateGroupMaster(ctx, sqlcgen.UpdateGroupMasterParams{
				ID:       groupID,
				MasterID: *masterID,
			})
			if err != nil {
				if isUniqueViolation(err) {
					return domain.ErrDuplicate
				}
				if isForeignKeyViolation(err) {
					return domain.ErrNotFound
				}
				return err
			}
			if affected == 0 {
				return domain.ErrNotFound
			}
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
