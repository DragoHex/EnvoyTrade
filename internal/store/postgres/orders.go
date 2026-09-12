package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"envoytrade/internal/domain"
	"envoytrade/internal/store/postgres/sqlcgen"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountOrders returns paginated order, position, holding, and summary detail for an account
// executing pagination at the DB level with LIMIT and OFFSET.
func (s *Store) AccountOrders(ctx context.Context, accountID uuid.UUID, tab string, page int, limit int) (domain.AccountOrdersDetail, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	if tab == "" {
		tab = "open_positions"
	}

	accs, err := s.Accounts(ctx, []uuid.UUID{accountID})
	if err != nil {
		return domain.AccountOrdersDetail{}, err
	}
	if len(accs) == 0 {
		return domain.AccountOrdersDetail{}, domain.ErrNotFound
	}
	acc := accs[0]

	detail := domain.AccountOrdersDetail{
		AccountID:       acc.ID,
		Role:            acc.Role,
		BrokerAccountID: acc.BrokerAccountID,
		OpenPositions:   make([]domain.PositionItem, 0),
		ClosedPositions: make([]domain.PositionItem, 0),
		Holdings:        make([]domain.HoldingItem, 0),
		OpenOrders:      make([]domain.OrderDetailItem, 0),
		ClosedOrders:    make([]domain.OrderDetailItem, 0),
		RejectedOrders:  make([]domain.OrderDetailItem, 0),
	}

	// 1. Fetch total counts across all tabs from DB
	openPosCount, _ := s.queries.CountOpenPositionsByAccount(ctx, accountID)
	closedPosCount, _ := s.queries.CountClosedPositionsByAccount(ctx, accountID)
	holdingsCount, _ := s.queries.CountAccountHoldings(ctx, accountID)

	var openOrdersCount, closedOrdersCount, rejectedOrdersCount int64
	if acc.Role == "master" {
		openOrdersCount, _ = s.queries.CountOpenMasterOrders(ctx, accountID)
		closedOrdersCount, _ = s.queries.CountClosedMasterOrders(ctx, accountID)
		rejectedOrdersCount, _ = s.queries.CountRejectedMasterOrders(ctx, accountID)
	} else {
		openOrdersCount, _ = s.queries.CountOpenFollowerOrders(ctx, accountID)
		closedOrdersCount, _ = s.queries.CountClosedFollowerOrders(ctx, accountID)
		rejectedOrdersCount, _ = s.queries.CountRejectedFollowerOrders(ctx, accountID)
	}

	// Calculate pagination state for requested tab
	var totalCount int
	switch tab {
	case "open_positions":
		totalCount = int(openPosCount)
	case "closed_positions":
		totalCount = int(closedPosCount)
	case "holdings":
		totalCount = int(holdingsCount)
	case "open_orders":
		totalCount = int(openOrdersCount)
	case "closed_orders":
		totalCount = int(closedOrdersCount)
	case "rejected_orders":
		totalCount = int(rejectedOrdersCount)
	default:
		tab = "open_positions"
		totalCount = int(openPosCount)
	}

	totalPages := 0
	if totalCount > 0 {
		totalPages = (totalCount + limit - 1) / limit
	}

	detail.Counts = domain.TabCounts{
		OpenPositions:   int(openPosCount),
		ClosedPositions: int(closedPosCount),
		Holdings:        int(holdingsCount),
		OpenOrders:      int(openOrdersCount),
		ClosedOrders:    int(closedOrdersCount),
		RejectedOrders:  int(rejectedOrdersCount),
	}

	detail.Pagination = domain.PaginationInfo{
		Tab:        tab,
		Page:       page,
		Limit:      limit,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	// 2. Query DB ONLY for the requested tab with LIMIT and OFFSET
	switch tab {
	case "open_positions":
		rows, err := s.queries.ListOpenPositionsByAccount(ctx, sqlcgen.ListOpenPositionsByAccountParams{
			AccountID: accountID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
		if err == nil && len(rows) > 0 {
			for _, r := range rows {
				detail.OpenPositions = append(detail.OpenPositions, domain.PositionItem{
					Product:    r.Product,
					Instrument: r.Instrument,
					Qty:        int(r.Quantity),
					AvgPrice:   fmt.Sprintf("%.2f/%.2f", r.BuyPrice.InexactFloat64(), r.SellPrice.InexactFloat64()),
					Ltp:        r.Ltp,
					Mtm:        r.Mtm,
					Action:     r.Action,
				})
			}
		}

	case "closed_positions":
		rows, err := s.queries.ListClosedPositionsByAccount(ctx, sqlcgen.ListClosedPositionsByAccountParams{
			AccountID: accountID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
		if err == nil && len(rows) > 0 {
			for _, r := range rows {
				detail.ClosedPositions = append(detail.ClosedPositions, domain.PositionItem{
					Product:    r.Product,
					Instrument: r.Instrument,
					Qty:        int(r.Quantity),
					AvgPrice:   fmt.Sprintf("%.2f/%.2f", r.BuyPrice.InexactFloat64(), r.SellPrice.InexactFloat64()),
					Ltp:        r.Ltp,
					Mtm:        r.Mtm,
					Action:     r.Action,
				})
			}
		}

	case "holdings":
		rows, err := s.queries.ListAccountHoldingsPaginated(ctx, sqlcgen.ListAccountHoldingsPaginatedParams{
			AccountID: accountID,
			Limit:     int32(limit),
			Offset:    int32(offset),
		})
		if err == nil && len(rows) > 0 {
			for _, r := range rows {
				detail.Holdings = append(detail.Holdings, domain.HoldingItem{
					Instrument:       r.Instrument,
					SellableQuantity: int(r.SellableQuantity),
					BuyAveragePrice:  r.BuyAveragePrice,
					Ltp:              r.Ltp,
					Pnl:              r.Pnl,
					Action:           r.Action,
				})
			}
		}

	case "open_orders":
		if acc.Role == "master" {
			fills, err := s.queries.ListOpenMasterOrdersPaginated(ctx, sqlcgen.ListOpenMasterOrdersPaginatedParams{
				MasterID: accountID,
				Limit:    int32(limit),
				Offset:   int32(offset),
			})
			if err == nil {
				for _, f := range fills {
					detail.OpenOrders = append(detail.OpenOrders, masterFillToOrderItem(f))
				}
			}
		} else {
			orders, err := s.queries.ListOpenFollowerOrdersPaginated(ctx, sqlcgen.ListOpenFollowerOrdersPaginatedParams{
				FollowerID: accountID,
				Limit:      int32(limit),
				Offset:     int32(offset),
			})
			if err == nil {
				for _, fo := range orders {
					detail.OpenOrders = append(detail.OpenOrders, followerOrderRowToOrderItem(
						fo.ID, fo.Product, fo.CreatedAt.Time, fo.Tradingsymbol, fo.IntendedQty,
						fo.PlacedQty, fo.TransactionType, fo.AveragePrice, fo.TerminalStatus, fo.LastError, fo.SizingReason,
					))
				}
			}
		}

	case "closed_orders":
		if acc.Role == "master" {
			fills, err := s.queries.ListClosedMasterOrdersPaginated(ctx, sqlcgen.ListClosedMasterOrdersPaginatedParams{
				MasterID: accountID,
				Limit:    int32(limit),
				Offset:   int32(offset),
			})
			if err == nil && len(fills) > 0 {
				for _, f := range fills {
					detail.ClosedOrders = append(detail.ClosedOrders, masterFillToOrderItem(f))
				}
			}
		} else {
			orders, err := s.queries.ListClosedFollowerOrdersPaginated(ctx, sqlcgen.ListClosedFollowerOrdersPaginatedParams{
				FollowerID: accountID,
				Limit:      int32(limit),
				Offset:     int32(offset),
			})
			if err == nil && len(orders) > 0 {
				for _, fo := range orders {
					detail.ClosedOrders = append(detail.ClosedOrders, followerOrderRowToOrderItem(
						fo.ID, fo.Product, fo.CreatedAt.Time, fo.Tradingsymbol, fo.IntendedQty,
						fo.PlacedQty, fo.TransactionType, fo.AveragePrice, fo.TerminalStatus, fo.LastError, fo.SizingReason,
					))
				}
			}
		}

	case "rejected_orders":
		if acc.Role == "master" {
			fills, err := s.queries.ListRejectedMasterOrdersPaginated(ctx, sqlcgen.ListRejectedMasterOrdersPaginatedParams{
				MasterID: accountID,
				Limit:    int32(limit),
				Offset:   int32(offset),
			})
			if err == nil {
				for _, f := range fills {
					detail.RejectedOrders = append(detail.RejectedOrders, masterFillToOrderItem(f))
				}
			}
		} else {
			orders, err := s.queries.ListRejectedFollowerOrdersPaginated(ctx, sqlcgen.ListRejectedFollowerOrdersPaginatedParams{
				FollowerID: accountID,
				Limit:      int32(limit),
				Offset:     int32(offset),
			})
			if err == nil {
				for _, fo := range orders {
					detail.RejectedOrders = append(detail.RejectedOrders, followerOrderRowToOrderItem(
						fo.ID, fo.Product, fo.CreatedAt.Time, fo.Tradingsymbol, fo.IntendedQty,
						fo.PlacedQty, fo.TransactionType, fo.AveragePrice, fo.TerminalStatus, fo.LastError, fo.SizingReason,
					))
				}
			}
		}
	}

	// 3. Margins & summary metrics
	margins, err := s.queries.GetAccountMargins(ctx, accountID)
	if err == nil {
		detail.Summary = domain.AccountSummaryMetrics{
			NetQty:               int(margins.NetQty),
			OpenPositionsCount:   int(openPosCount),
			ClosedPositionsCount: int(closedPosCount),
			PendingOrdersCount:   int(openOrdersCount),
			TotalMtm:             margins.TotalMtm,
			RealizedPnl:          margins.RealizedPnl,
			AccountValue:         margins.AccountValue,
			Status:               margins.Status,
		}
	} else {
		detail.Summary = domain.AccountSummaryMetrics{
			NetQty:               0,
			OpenPositionsCount:   int(openPosCount),
			ClosedPositionsCount: int(closedPosCount),
			PendingOrdersCount:   int(openOrdersCount),
			TotalMtm:             decimal.Zero,
			RealizedPnl:          decimal.Zero,
			AccountValue:         decimal.Zero,
			Status:               "offline",
		}
	}

	return detail, nil
}

func masterFillToOrderItem(f sqlcgen.MasterFill) domain.OrderDetailItem {
	item := domain.OrderDetailItem{
		ID:         fmt.Sprintf("%d", f.ID),
		Product:    f.Product,
		Time:       f.OrderTimestamp.Time.Format("2006-01-02 15:04:05"),
		Instrument: f.Tradingsymbol,
		Quantity:   int(f.FilledQuantity),
		Type:       transactionTypeLetter(f.TransactionType),
		Status:     f.Status,
	}
	price := f.AveragePrice
	item.Price = &price

	var rawMap map[string]any
	if len(f.RawPayload) > 0 && json.Unmarshal(f.RawPayload, &rawMap) == nil {
		if tp, ok := rawMap["trigger_price"].(float64); ok && tp > 0 {
			d := decimal.NewFromFloat(tp)
			item.TriggerPrice = &d
		}
		if lp, ok := rawMap["price"].(float64); ok && lp > 0 {
			d := decimal.NewFromFloat(lp)
			item.LimitPrice = &d
		}
		if sm, ok := rawMap["status_message"].(string); ok && sm != "" {
			item.Reason = sm
		}
	}
	if item.Reason == "" && (f.Status == domain.TerminalRejected || f.Status == domain.TerminalCancelled) {
		item.Reason = f.Status
	}
	return item
}

func followerOrderRowToOrderItem(
	id int64,
	product string,
	createdAt time.Time,
	tradingsymbol string,
	intendedQty int32,
	placedQty *int32,
	txType string,
	avgPrice decimal.NullDecimal,
	termStatus *string,
	lastErr *string,
	sizingReason int32,
) domain.OrderDetailItem {
	item := domain.OrderDetailItem{
		ID:         fmt.Sprintf("%d", id),
		Product:    product,
		Time:       createdAt.Format("2006-01-02 15:04:05"),
		Instrument: tradingsymbol,
		Quantity:   int(intendedQty),
		Type:       transactionTypeLetter(txType),
	}
	if placedQty != nil && *placedQty > 0 {
		item.Quantity = int(*placedQty)
	}
	if avgPrice.Valid {
		price := avgPrice.Decimal
		item.Price = &price
	}
	status := ""
	if termStatus != nil {
		status = *termStatus
	}
	item.Status = status

	if lastErr != nil && *lastErr != "" {
		item.Reason = *lastErr
	} else if intendedQty == 0 {
		item.Reason = fmt.Sprintf("sizing: %s", sizingReasonString(domain.SizingReason(sizingReason)))
	} else if status == domain.TerminalRejected || status == domain.TerminalCancelled || status == domain.TerminalDeadLettered {
		item.Reason = status
	}
	return item
}

func sizingReasonString(r domain.SizingReason) string {
	switch r {
	case domain.ReasonBelowOneLot:
		return "below one lot"
	case domain.ReasonCapped:
		return "capped"
	case domain.ReasonBadInstrument:
		return "bad instrument"
	case domain.ReasonInvalidRatio:
		return "invalid ratio"
	default:
		return "ok"
	}
}

func transactionTypeLetter(tx string) string {
	if tx == "BUY" || tx == "B" {
		return "B"
	}
	return "S"
}
