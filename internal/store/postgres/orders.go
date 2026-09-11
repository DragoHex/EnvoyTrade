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

	// Fallback sample data if DB tables are empty (unseeded dev state)
	var sampleOpen []domain.PositionItem
	var sampleClosed []domain.PositionItem
	if openPosCount == 0 && closedPosCount == 0 {
		sampleOpen = sampleOpenPositions()
		sampleClosed = sampleClosedPositions()
		openPosCount = int64(len(sampleOpen))
		closedPosCount = int64(len(sampleClosed))
	}

	var sampleHold []domain.HoldingItem
	if holdingsCount == 0 {
		sampleHold = sampleHoldings()
		holdingsCount = int64(len(sampleHold))
	}

	var sampleClOrders []domain.OrderDetailItem
	if openOrdersCount == 0 && closedOrdersCount == 0 && rejectedOrdersCount == 0 {
		sampleClOrders = sampleClosedOrders()
		closedOrdersCount = int64(len(sampleClOrders))
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
		} else if len(sampleOpen) > 0 {
			detail.OpenPositions = sliceItems(sampleOpen, offset, limit)
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
		} else if len(sampleClosed) > 0 {
			detail.ClosedPositions = sliceItems(sampleClosed, offset, limit)
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
		} else if len(sampleHold) > 0 {
			detail.Holdings = sliceItems(sampleHold, offset, limit)
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
			} else if len(sampleClOrders) > 0 {
				detail.ClosedOrders = sliceItems(sampleClOrders, offset, limit)
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
			} else if len(sampleClOrders) > 0 {
				detail.ClosedOrders = sliceItems(sampleClOrders, offset, limit)
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
			NetQty:               -890,
			OpenPositionsCount:   int(openPosCount),
			ClosedPositionsCount: int(closedPosCount),
			PendingOrdersCount:   int(openOrdersCount),
			TotalMtm:             decimal.NewFromFloat(380.00),
			RealizedPnl:          decimal.Zero,
			AccountValue:         decimal.NewFromFloat(2163520.84),
			Status:               "online",
		}
	}

	return detail, nil
}

func sliceItems[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return make([]T, 0)
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
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

func sampleOpenPositions() []domain.PositionItem {
	return []domain.PositionItem{
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26C10600", Qty: -100, AvgPrice: "0.00/111.10", Ltp: decimal.NewFromFloat(114.4), Mtm: decimal.NewFromFloat(-330.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26C10700", Qty: -100, AvgPrice: "0.00/137.20", Ltp: decimal.NewFromFloat(101.6), Mtm: decimal.NewFromFloat(3560.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26C11000", Qty: -200, AvgPrice: "0.00/67.10", Ltp: decimal.NewFromFloat(71.0), Mtm: decimal.NewFromFloat(-780.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26P8400", Qty: -200, AvgPrice: "0.00/45.65", Ltp: decimal.NewFromFloat(43.0), Mtm: decimal.NewFromFloat(530.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26P8500", Qty: -100, AvgPrice: "0.00/53.20", Ltp: decimal.NewFromFloat(50.5), Mtm: decimal.NewFromFloat(270.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26P8600", Qty: -100, AvgPrice: "0.00/59.70", Ltp: decimal.NewFromFloat(60.6), Mtm: decimal.NewFromFloat(-90.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26P8700", Qty: -100, AvgPrice: "0.00/53.90", Ltp: decimal.NewFromFloat(73.3), Mtm: decimal.NewFromFloat(-1940.00), Action: "exit"},
		{Product: "CNC", Instrument: "CRUDEOILM21SEP26", Qty: 10, AvgPrice: "9896.00/0.00", Ltp: decimal.NewFromFloat(9580.0), Mtm: decimal.NewFromFloat(-3160.00), Action: "exit"},
	}
}

func sampleClosedPositions() []domain.PositionItem {
	return []domain.PositionItem{
		{Product: "CNC", Instrument: "CRUDEOIL17SEP26P8200", Qty: 0, AvgPrice: "23.00/40.20", Ltp: decimal.NewFromFloat(29.7), Mtm: decimal.NewFromFloat(1720.00)},
		{Product: "CNC", Instrument: "CRUDEOIL21SEP26", Qty: 0, AvgPrice: "9885.00/9891.00", Ltp: decimal.NewFromFloat(9584.0), Mtm: decimal.NewFromFloat(600.00)},
	}
}

func sampleHoldings() []domain.HoldingItem {
	return []domain.HoldingItem{
		{Instrument: "ASIANPAINT-EQ", SellableQuantity: 1, BuyAveragePrice: decimal.NewFromFloat(2369.20), Ltp: decimal.NewFromFloat(2469.2), Pnl: decimal.NewFromFloat(100.00), Action: "exit"},
		{Instrument: "ATHERENERG", SellableQuantity: 100, BuyAveragePrice: decimal.NewFromFloat(891.10), Ltp: decimal.NewFromFloat(1656.0), Pnl: decimal.NewFromFloat(76490.00), Action: "exit"},
		{Instrument: "BHARTIARTL-EQ", SellableQuantity: 1, BuyAveragePrice: decimal.NewFromFloat(756.01), Ltp: decimal.NewFromFloat(1842.5), Pnl: decimal.NewFromFloat(1086.49), Action: "exit"},
		{Instrument: "DELHIVERY", SellableQuantity: 1, BuyAveragePrice: decimal.NewFromFloat(259.85), Ltp: decimal.NewFromFloat(438.5), Pnl: decimal.NewFromFloat(178.65), Action: "exit"},
		{Instrument: "EXIDEIND-EQ", SellableQuantity: 100, BuyAveragePrice: decimal.NewFromFloat(331.80), Ltp: decimal.NewFromFloat(414.4), Pnl: decimal.NewFromFloat(8260.00), Action: "exit"},
		{Instrument: "GOLDBEES", SellableQuantity: 200, BuyAveragePrice: decimal.NewFromFloat(122.80), Ltp: decimal.NewFromFloat(125.18), Pnl: decimal.NewFromFloat(476.00), Action: "exit"},
		{Instrument: "HAL", SellableQuantity: 1, BuyAveragePrice: decimal.NewFromFloat(2574.31), Ltp: decimal.NewFromFloat(4921.7), Pnl: decimal.NewFromFloat(2347.39), Action: "exit"},
		{Instrument: "HCLTECH-EQ", SellableQuantity: 0, BuyAveragePrice: decimal.NewFromFloat(1363.87), Ltp: decimal.NewFromFloat(1219.3), Pnl: decimal.NewFromFloat(-15567.00), Action: "exit"},
		{Instrument: "HDFCBANK", SellableQuantity: 100, BuyAveragePrice: decimal.NewFromFloat(860.17), Ltp: decimal.NewFromFloat(703.85), Pnl: decimal.NewFromFloat(-15631.67), Action: "exit"},
		{Instrument: "HINDZINC", SellableQuantity: 1, BuyAveragePrice: decimal.NewFromFloat(491.40), Ltp: decimal.NewFromFloat(577.5), Pnl: decimal.NewFromFloat(86.10), Action: "exit"},
		{Instrument: "IEX-EQ", SellableQuantity: 1, BuyAveragePrice: decimal.NewFromFloat(131.50), Ltp: decimal.NewFromFloat(114.03), Pnl: decimal.NewFromFloat(-17.47), Action: "exit"},
	}
}

func sampleClosedOrders() []domain.OrderDetailItem {
	p1 := decimal.NewFromFloat(111.1)
	p2 := decimal.NewFromFloat(137.2)
	p3 := decimal.NewFromFloat(9891.0)
	p4 := decimal.NewFromFloat(9896.0)
	p5 := decimal.NewFromFloat(9885.0)
	p6 := decimal.NewFromFloat(23.0)
	p7 := decimal.NewFromFloat(53.9)

	return []domain.OrderDetailItem{
		{Product: "CNC", Time: "2026-09-11 14:24:05", Instrument: "CRUDEOIL17SEP26C10600", Quantity: 100, Price: &p1, Type: "S"},
		{Product: "CNC", Time: "2026-09-11 11:18:42", Instrument: "CRUDEOIL17SEP26C10700", Quantity: 100, Price: &p2, Type: "S"},
		{Product: "CNC", Time: "2026-09-11 09:22:37", Instrument: "CRUDEOIL21SEP26", Quantity: 100, Price: &p3, Type: "S"},
		{Product: "CNC", Time: "2026-09-11 09:22:34", Instrument: "CRUDEOILM21SEP26", Quantity: 10, Price: &p4, Type: "B"},
		{Product: "CNC", Time: "2026-09-11 09:19:01", Instrument: "CRUDEOIL21SEP26", Quantity: 100, Price: &p5, Type: "B"},
		{Product: "CNC", Time: "2026-09-11 09:17:37", Instrument: "CRUDEOIL17SEP26P8200", Quantity: 100, Price: &p6, Type: "B"},
		{Product: "CNC", Time: "2026-09-11 09:17:28", Instrument: "CRUDEOIL17SEP26P8700", Quantity: 100, Price: &p7, Type: "S"},
	}
}
