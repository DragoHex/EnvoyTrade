package rebalance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Store declares persistence dependencies needed by Rebalance service (PLAN.md §1 seam rule).
type Store interface {
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
	GroupDetail(ctx context.Context, groupID uuid.UUID) (domain.GroupDetail, error)
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	InstrumentLotSize(ctx context.Context, exchange, symbol string) (int, error)
	AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error
	InsertFollowerOrder(ctx context.Context, o domain.FollowerOrder) (int64, error)
	UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error
	UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error
}

// BrokerResolver resolves the broker.Broker instance for a given account.
type BrokerResolver interface {
	ResolveBroker(ctx context.Context, accountID uuid.UUID) (broker.Broker, error)
}

// PortfolioSyncer updates local database portfolio records after execution.
type PortfolioSyncer interface {
	SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error
}

// Service coordinates position rebalancing between master and followers.
type Service struct {
	store    Store
	resolver BrokerResolver
	syncer   PortfolioSyncer
	logger   *slog.Logger
}

// NewService constructs a new Rebalance Service.
func NewService(store Store, resolver BrokerResolver, syncer PortfolioSyncer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    store,
		resolver: resolver,
		syncer:   syncer,
		logger:   logger.With("component", "rebalance"),
	}
}

func (s *Service) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tokenexception") ||
		strings.Contains(msg, "incorrect `api_key` or `access_token`") ||
		strings.Contains(msg, "incorrect api_key or access_token") ||
		strings.Contains(msg, "invalid token")
}

func isBrokerUnreachableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") ||
		strings.Contains(msg, "network") ||
		strings.Contains(msg, "broker is not reachable")
}

func wrapBrokerError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	if isAuthError(err) {
		return fmt.Errorf("%s: %w (%v)", prefix, domain.ErrAuthExpired, err)
	}
	return fmt.Errorf("%s: %w (%v)", prefix, domain.ErrBrokerUnreachable, err)
}

func (s *Service) getPositionsWithRetry(ctx context.Context, b broker.Broker, accountID uuid.UUID) ([]broker.Position, broker.Broker, error) {
	positions, err := b.GetPositions(ctx)
	if err != nil && s.syncer != nil && isAuthError(err) {
		s.log().Info("rebalance: token expired or rejected, syncing portfolio", "account_id", accountID)
		if syncErr := s.syncer.SyncAccountPortfolio(ctx, accountID); syncErr == nil {
			if refreshedBroker, rErr := s.resolver.ResolveBroker(ctx, accountID); rErr == nil {
				b = refreshedBroker
				positions, err = b.GetPositions(ctx)
			}
		}
	}
	return positions, b, err
}

// ComputeAccountDiff calculates position drift for one follower relative to its master.
func (s *Service) ComputeAccountDiff(ctx context.Context, followerID uuid.UUID) (domain.FollowerDrift, error) {
	accs, err := s.store.Accounts(ctx, []uuid.UUID{followerID})
	if err != nil {
		return domain.FollowerDrift{}, fmt.Errorf("load follower account: %w", err)
	}
	if len(accs) == 0 {
		return domain.FollowerDrift{}, domain.ErrNotFound
	}
	followerAcc := accs[0]
	if followerAcc.Role != "follower" || followerAcc.MasterID == nil {
		return domain.FollowerDrift{}, fmt.Errorf("account %s is not a linked follower: %w", followerID, domain.ErrNotFound)
	}

	masterID := *followerAcc.MasterID
	bMaster, err := s.resolver.ResolveBroker(ctx, masterID)
	if err != nil {
		return domain.FollowerDrift{}, wrapBrokerError("resolve master broker", err)
	}
	bFollower, err := s.resolver.ResolveBroker(ctx, followerID)
	if err != nil {
		return domain.FollowerDrift{}, wrapBrokerError("resolve follower broker", err)
	}

	masterPositions, _, err := s.getPositionsWithRetry(ctx, bMaster, masterID)
	if err != nil {
		return domain.FollowerDrift{}, wrapBrokerError("fetch master positions", err)
	}
	followerPositions, _, err := s.getPositionsWithRetry(ctx, bFollower, followerID)
	if err != nil {
		return domain.FollowerDrift{}, wrapBrokerError("fetch follower positions", err)
	}

	return s.calculateFollowerDrift(ctx, followerAcc, masterPositions, followerPositions)
}

// ComputeGroupDiff computes position drifts for all followers in a group.
func (s *Service) ComputeGroupDiff(ctx context.Context, groupID uuid.UUID) (domain.GroupRebalanceDiff, error) {
	groupDetail, err := s.store.GroupDetail(ctx, groupID)
	if err != nil {
		return domain.GroupRebalanceDiff{}, fmt.Errorf("load group detail: %w", err)
	}

	masterID := groupDetail.MasterID
	bMaster, err := s.resolver.ResolveBroker(ctx, masterID)
	if err != nil {
		return domain.GroupRebalanceDiff{}, wrapBrokerError("resolve master broker", err)
	}

	masterPositions, _, err := s.getPositionsWithRetry(ctx, bMaster, masterID)
	if err != nil {
		return domain.GroupRebalanceDiff{}, wrapBrokerError("fetch master positions", err)
	}

	followerIDs := make([]uuid.UUID, 0, len(groupDetail.Followers))
	for _, f := range groupDetail.Followers {
		followerIDs = append(followerIDs, f.AccountID)
	}

	accs, err := s.store.Accounts(ctx, followerIDs)
	if err != nil {
		return domain.GroupRebalanceDiff{}, fmt.Errorf("load follower accounts: %w", err)
	}
	accMap := make(map[uuid.UUID]domain.Account, len(accs))
	for _, a := range accs {
		accMap[a.ID] = a
	}

	diffResult := domain.GroupRebalanceDiff{
		GroupID:            groupID,
		MasterID:           masterID,
		FollowersEvaluated: len(groupDetail.Followers),
		Drifts:             make([]domain.FollowerDrift, 0),
	}

	for _, gf := range groupDetail.Followers {
		fAcc, ok := accMap[gf.AccountID]
		if !ok {
			fAcc = domain.Account{
				ID:              gf.AccountID,
				Name:            gf.Name,
				BrokerAccountID: gf.BrokerAccountID,
				Enabled:         gf.Enabled,
			}
		}

		displayName := gf.Name
		if displayName == "" {
			displayName = gf.BrokerAccountID
		}

		bFollower, err := s.resolver.ResolveBroker(ctx, gf.AccountID)
		if err != nil {
			s.log().Warn("rebalance: resolve follower broker failed", "follower_id", gf.AccountID, "error", err)
			return domain.GroupRebalanceDiff{}, wrapBrokerError(fmt.Sprintf("resolve follower %s broker", displayName), err)
		}

		fPositions, _, err := s.getPositionsWithRetry(ctx, bFollower, gf.AccountID)
		if err != nil {
			s.log().Warn("rebalance: fetch follower positions failed", "follower_id", gf.AccountID, "error", err)
			return domain.GroupRebalanceDiff{}, wrapBrokerError(fmt.Sprintf("fetch follower %s positions", displayName), err)
		}

		fDrift, err := s.calculateFollowerDrift(ctx, fAcc, masterPositions, fPositions)
		if err != nil {
			s.log().Warn("rebalance: calculate follower drift failed", "follower_id", gf.AccountID, "error", err)
			return domain.GroupRebalanceDiff{}, fmt.Errorf("calculate follower %s drift: %w", displayName, err)
		}

		if len(fDrift.Symbols) > 0 {
			diffResult.FollowersWithDrift++
			diffResult.Drifts = append(diffResult.Drifts, fDrift)
		}
	}

	return diffResult, nil
}

func (s *Service) calculateFollowerDrift(
	ctx context.Context,
	followerAcc domain.Account,
	masterPositions []broker.Position,
	followerPositions []broker.Position,
) (domain.FollowerDrift, error) {
	cloneFactor := decimal.NewFromFloat(1.0)
	if followerAcc.CloneFactor != nil && followerAcc.CloneFactor.Sign() > 0 {
		cloneFactor = *followerAcc.CloneFactor
	}
	maxQty := 0
	if followerAcc.MaxQtyPerOrder != nil {
		maxQty = *followerAcc.MaxQtyPerOrder
	}

	// Index master positions by "EXCHANGE:SYMBOL"
	type posKey struct {
		exchange string
		symbol   string
	}
	masterMap := make(map[posKey]broker.Position)
	for _, mp := range masterPositions {
		if mp.Quantity != 0 {
			masterMap[posKey{exchange: mp.Exchange, symbol: mp.Tradingsymbol}] = mp
		}
	}

	// Index follower positions by "EXCHANGE:SYMBOL"
	followerMap := make(map[posKey]broker.Position)
	for _, fp := range followerPositions {
		if fp.Quantity != 0 {
			followerMap[posKey{exchange: fp.Exchange, symbol: fp.Tradingsymbol}] = fp
		}
	}

	// Union of all keys
	allKeys := make(map[posKey]bool)
	for k := range masterMap {
		allKeys[k] = true
	}
	for k := range followerMap {
		allKeys[k] = true
	}

	var symbolDrifts []domain.SymbolDrift

	for k := range allKeys {
		mPos := masterMap[k]
		fPos := followerMap[k]

		lotSize, err := s.store.InstrumentLotSize(ctx, k.exchange, k.symbol)
		if err != nil || lotSize <= 0 {
			lotSize = 1 // fallback if missing from instrument master
		}

		product := mPos.Product
		if product == "" {
			product = fPos.Product
		}
		if product == "" {
			product = "CNC"
		}

		masterQty := mPos.Quantity
		followerQty := fPos.Quantity

		targetQty := 0
		if masterQty != 0 {
			absTarget, reason := domain.SizeOrder(abs(masterQty), cloneFactor, lotSize, maxQty)
			if reason == domain.ReasonOK || reason == domain.ReasonCapped {
				if masterQty > 0 {
					targetQty = absTarget
				} else {
					targetQty = -absTarget
				}
			}
		}

		driftQty := targetQty - followerQty
		if driftQty != 0 {
			action := "BUY"
			if driftQty < 0 {
				action = "SELL"
			}
			symbolDrifts = append(symbolDrifts, domain.SymbolDrift{
				Exchange:      k.exchange,
				Tradingsymbol: k.symbol,
				Product:       product,
				LotSize:       lotSize,
				MasterQty:     masterQty,
				TargetQty:     targetQty,
				FollowerQty:   followerQty,
				DriftQty:      driftQty,
				Action:        action,
			})
		}
	}

	return domain.FollowerDrift{
		AccountID:       followerAcc.ID,
		AccountName:     followerAcc.Name,
		BrokerAccountID: followerAcc.BrokerAccountID,
		Enabled:         followerAcc.Enabled,
		CloneFactor:     cloneFactor,
		Symbols:         symbolDrifts,
	}, nil
}

// RebalanceAccount balances a single follower account against its master.
func (s *Service) RebalanceAccount(ctx context.Context, followerID uuid.UUID) (domain.RebalanceResult, error) {
	s.log().Info("rebalance: starting single account rebalance", "follower_id", followerID)

	drift, err := s.ComputeAccountDiff(ctx, followerID)
	if err != nil {
		return domain.RebalanceResult{}, err
	}
	if !drift.Enabled {
		return domain.RebalanceResult{}, fmt.Errorf("follower %s: %w", followerID, domain.ErrAccountDisabled)
	}


	result := domain.RebalanceResult{
		Action:            "rebalance",
		Status:            "completed",
		AccountID:         &followerID,
		FollowersAffected: 1,
		Orders:            make([]domain.RebalanceOrder, 0),
		Errors:            make([]string, 0),
	}

	if len(drift.Symbols) == 0 {
		return result, nil
	}

	bFollower, err := s.resolver.ResolveBroker(ctx, followerID)
	if err != nil {
		return domain.RebalanceResult{}, fmt.Errorf("resolve follower broker: %w", err)
	}

	// 1. Cancel open orders for drifting symbols to prevent duplicate fills
	driftingSymbols := make([]string, 0, len(drift.Symbols))
	for _, s := range drift.Symbols {
		driftingSymbols = append(driftingSymbols, s.Tradingsymbol)
	}
	openOrders, err := bFollower.GetOpenOrders(ctx)
	if err == nil && len(openOrders) > 0 {
		for _, o := range openOrders {
			for _, sym := range driftingSymbols {
				if strings.EqualFold(o.Tradingsymbol, sym) {
					if _, err := bFollower.CancelOrder(ctx, "regular", o.OrderID); err == nil {
						result.CancelledOrders++
					} else {
						s.log().Warn("rebalance: cancel order error", "follower_id", followerID, "order_id", o.OrderID, "error", err)
					}
					break
				}
			}
		}
	}

	// 2. Place market counter orders for each drift
	for _, symDrift := range drift.Symbols {
		orderQty := abs(symDrift.DriftQty)
		if orderQty == 0 {
			continue
		}

		shortID := strings.ReplaceAll(followerID.String(), "-", "")
		if len(shortID) > 4 {
			shortID = shortID[:4]
		}
		ts := time.Now().Unix() % 10000000
		tag := fmt.Sprintf("rebal-f-%s-%07d", shortID, ts)
		if len(tag) > 20 {
			tag = tag[:20]
		}

		followerOrderID, insErr := s.store.InsertFollowerOrder(ctx, domain.FollowerOrder{
			FollowerID:      followerID,
			IdempotencyTag:  tag,
			IntendedQty:     orderQty,
			Origin:          "rebalance",
			Tradingsymbol:   symDrift.Tradingsymbol,
			Exchange:        symDrift.Exchange,
			Product:         symDrift.Product,
			TransactionType: symDrift.Action,
			OrderType:       "MARKET",
		})
		if insErr != nil && !errors.Is(insErr, domain.ErrDuplicate) {
			s.log().Warn("rebalance: failed to pre-insert follower order", "follower_id", followerID, "error", insErr)
		}

		orderResp, err := bFollower.PlaceOrder(ctx, "regular", broker.OrderParams{
			Exchange:        symDrift.Exchange,
			Tradingsymbol:   symDrift.Tradingsymbol,
			TransactionType: symDrift.Action,
			Product:         symDrift.Product,
			OrderType:       "MARKET",
			Quantity:        orderQty,
			Tag:             tag,
		})
		if err != nil {
			if followerOrderID > 0 {
				_ = s.store.UpdateFollowerOrderFailed(ctx, followerOrderID, "REJECTED", err.Error())
			}
			errStr := fmt.Sprintf("follower %s rebalance %s %s: %v", followerID, symDrift.Action, symDrift.Tradingsymbol, err)
			s.log().Error("rebalance: place order failed", "follower_id", followerID, "symbol", symDrift.Tradingsymbol, "error", err)
			result.Errors = append(result.Errors, errStr)
			result.Status = "partial"
			continue
		}

		if followerOrderID > 0 {
			_ = s.store.UpdateFollowerOrderPlaced(ctx, followerOrderID, orderResp.OrderID, orderQty)
		}

		result.OrdersPlaced++
		result.Orders = append(result.Orders, domain.RebalanceOrder{
			AccountID:     followerID,
			Role:          "follower",
			BrokerOrderID: orderResp.OrderID,
			Exchange:      symDrift.Exchange,
			Tradingsymbol: symDrift.Tradingsymbol,
			Product:       symDrift.Product,
			Side:          symDrift.Action,
			Quantity:      orderQty,
			Status:        "placed",
		})
	}

	if s.syncer != nil {
		_ = s.syncer.SyncAccountPortfolio(ctx, followerID)
	}

	return result, nil
}

// RebalanceGroup executes rebalancing across selected followers (or all drifting followers) in a group.
func (s *Service) RebalanceGroup(ctx context.Context, groupID uuid.UUID, followerIDs []uuid.UUID) (domain.RebalanceResult, error) {
	s.log().Info("rebalance: starting group rebalance", "group_id", groupID, "follower_ids", followerIDs)

	diff, err := s.ComputeGroupDiff(ctx, groupID)
	if err != nil {
		return domain.RebalanceResult{}, err
	}

	filterMap := make(map[uuid.UUID]bool)
	for _, id := range followerIDs {
		filterMap[id] = true
	}

	var targetDrifts []domain.FollowerDrift
	for _, d := range diff.Drifts {
		if !d.Enabled {
			s.log().Info("rebalance: skipping disabled follower in group rebalance", "follower_id", d.AccountID)
			continue
		}
		if len(filterMap) == 0 || filterMap[d.AccountID] {
			targetDrifts = append(targetDrifts, d)
		}
	}


	result := domain.RebalanceResult{
		Action:            "rebalance",
		Status:            "completed",
		GroupID:           &groupID,
		FollowersAffected: len(targetDrifts),
		Orders:            make([]domain.RebalanceOrder, 0),
		Errors:            make([]string, 0),
	}

	if len(targetDrifts) == 0 {
		return result, nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, d := range targetDrifts {
		fID := d.AccountID
		wg.Add(1)
		go func(targetID uuid.UUID) {
			defer wg.Done()
			fRes, fErr := s.RebalanceAccount(ctx, targetID)
			mu.Lock()
			defer mu.Unlock()
			if fErr != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("follower %s: %v", targetID, fErr))
				result.Status = "partial"
			} else {
				result.CancelledOrders += fRes.CancelledOrders
				result.OrdersPlaced += fRes.OrdersPlaced
				result.Orders = append(result.Orders, fRes.Orders...)
				if len(fRes.Errors) > 0 {
					result.Errors = append(result.Errors, fRes.Errors...)
					result.Status = "partial"
				}
			}
		}(fID)
	}

	wg.Wait()

	s.log().Info("rebalance: group rebalance finished",
		"group_id", groupID,
		"status", result.Status,
		"cancelled_orders", result.CancelledOrders,
		"orders_placed", result.OrdersPlaced,
		"errors_count", len(result.Errors),
	)

	return result, nil
}
