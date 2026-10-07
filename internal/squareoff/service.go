package squareoff

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// Store declares persistence dependencies needed by SquareOff service (PLAN.md §1 seam rule).
type Store interface {
	AccountRole(ctx context.Context, id uuid.UUID) (string, error)
	GroupDetail(ctx context.Context, groupID uuid.UUID) (domain.GroupDetail, error)
	Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error)
	AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error
}


// BrokerResolver resolves the broker.Broker instance for a given account.
type BrokerResolver interface {
	ResolveBroker(ctx context.Context, accountID uuid.UUID) (broker.Broker, error)
}

// PortfolioSyncer updates local database portfolio records after execution.
type PortfolioSyncer interface {
	SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error
}

// Service coordinates position square-off across groups and individual accounts.
type Service struct {
	store    Store
	resolver BrokerResolver
	syncer   PortfolioSyncer
	logger   *slog.Logger
}

// NewService constructs a new SquareOff Service.
func NewService(store Store, resolver BrokerResolver, syncer PortfolioSyncer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    store,
		resolver: resolver,
		syncer:   syncer,
		logger:   logger.With("component", "squareoff"),
	}
}

func (s *Service) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

func matchSymbol(symbol string, selectedSymbols []string) bool {
	if len(selectedSymbols) == 0 {
		return true
	}
	for _, s := range selectedSymbols {
		if strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(symbol)) {
			return true
		}
	}
	return false
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

// SquareOffAccount closes open positions strictly for one designated account.
func (s *Service) SquareOffAccount(ctx context.Context, accountID uuid.UUID, symbols []string) (domain.SquareOffResult, error) {
	s.log().Info("squareoff: starting account square off", "account_id", accountID, "symbols", symbols)

	role, err := s.store.AccountRole(ctx, accountID)
	if err != nil {
		return domain.SquareOffResult{}, fmt.Errorf("check account role: %w", err)
	}

	if role == "follower" {
		accs, err := s.store.Accounts(ctx, []uuid.UUID{accountID})
		if err != nil {
			return domain.SquareOffResult{}, fmt.Errorf("load account: %w", err)
		}
		if len(accs) == 0 {
			return domain.SquareOffResult{}, domain.ErrNotFound
		}
		if !accs[0].Enabled {
			return domain.SquareOffResult{}, fmt.Errorf("follower %s: %w", accountID, domain.ErrAccountDisabled)
		}
	}

	b, err := s.resolver.ResolveBroker(ctx, accountID)

	if err != nil {
		return domain.SquareOffResult{}, fmt.Errorf("resolve broker: %w", err)
	}

	result := domain.SquareOffResult{
		Action:            "square_off",
		Status:            "completed",
		AccountID:         &accountID,
		Role:              role,
		FollowersAffected: 1,
		Orders:            make([]domain.SquareOffOrder, 0),
		Errors:            make([]string, 0),
	}

	// 1. Cancel open orders to prevent re-entry fills
	openOrders, err := b.GetOpenOrders(ctx)
	if err == nil && len(openOrders) > 0 {
		for _, o := range openOrders {
			if matchSymbol(o.Tradingsymbol, symbols) {
				if _, err := b.CancelOrder(ctx, "regular", o.OrderID); err == nil {
					result.CancelledOrders++
				} else {
					s.log().Warn("squareoff: cancel order error", "account_id", accountID, "order_id", o.OrderID, "error", err)
				}
			}
		}
	}

	// 2. Query positions and place opposite market counter-orders
	positions, err := b.GetPositions(ctx)
	if err != nil && s.syncer != nil && isAuthError(err) {
		s.log().Info("squareoff: token expired or rejected, syncing portfolio", "account_id", accountID)
		if syncErr := s.syncer.SyncAccountPortfolio(ctx, accountID); syncErr == nil {
			if refreshedBroker, rErr := s.resolver.ResolveBroker(ctx, accountID); rErr == nil {
				b = refreshedBroker
				positions, err = b.GetPositions(ctx)
			}
		}
	}
	if err != nil {
		return domain.SquareOffResult{}, fmt.Errorf("fetch positions: %w", err)
	}

	for _, p := range positions {
		if p.Quantity == 0 || !matchSymbol(p.Tradingsymbol, symbols) {
			continue
		}

		side := "SELL"
		if p.Quantity < 0 {
			side = "BUY"
		}
		qty := abs(p.Quantity)
		rolePrefix := "a"
		if len(role) > 0 {
			rolePrefix = strings.ToLower(role[:1])
		}
		shortID := strings.ReplaceAll(accountID.String(), "-", "")
		if len(shortID) > 4 {
			shortID = shortID[:4]
		}
		ts := time.Now().Unix() % 10000000
		tag := fmt.Sprintf("sqoff-%s-%s-%07d", rolePrefix, shortID, ts)
		if len(tag) > 20 {
			tag = tag[:20]
		}

		orderResp, err := b.PlaceOrder(ctx, "regular", broker.OrderParams{
			Exchange:        p.Exchange,
			Tradingsymbol:   p.Tradingsymbol,
			TransactionType: side,
			Product:         p.Product,
			OrderType:       "MARKET",
			Quantity:        qty,
			Tag:             tag,
		})
		if err != nil {
			errStr := fmt.Sprintf("account %s square off %s: %v", accountID, p.Tradingsymbol, err)
			s.log().Error("squareoff: place order failed", "account_id", accountID, "symbol", p.Tradingsymbol, "error", err)
			result.Errors = append(result.Errors, errStr)
			result.Status = "partial"
			continue
		}

		result.PositionsSquaredOff++
		result.Orders = append(result.Orders, domain.SquareOffOrder{
			AccountID:     accountID,
			Role:          role,
			BrokerOrderID: orderResp.OrderID,
			Exchange:      p.Exchange,
			Tradingsymbol: p.Tradingsymbol,
			Product:       p.Product,
			Side:          side,
			Quantity:      qty,
			Status:        "placed",
		})
	}

	if result.PositionsSquaredOff == 0 && len(result.Errors) == 0 {
		result.Status = "completed"
	}

	// 3. Trigger local portfolio sync
	if s.syncer != nil {
		_ = s.syncer.SyncAccountPortfolio(ctx, accountID)
	}

	return result, nil
}

// SquareOffGroup flattens open positions across Master and all linked Followers.
func (s *Service) SquareOffGroup(ctx context.Context, groupID uuid.UUID, symbols []string) (domain.SquareOffResult, error) {
	s.log().Info("squareoff: starting group square off", "group_id", groupID, "symbols", symbols)

	groupDetail, err := s.store.GroupDetail(ctx, groupID)
	if err != nil {
		return domain.SquareOffResult{}, fmt.Errorf("load group detail: %w", err)
	}

	masterID := groupDetail.MasterID

	enabledFollowersCount := 0
	for _, f := range groupDetail.Followers {
		if f.Enabled {
			enabledFollowersCount++
		}
	}

	result := domain.SquareOffResult{
		Action:            "square_off",
		Status:            "completed",
		GroupID:           &groupID,
		AccountID:         &masterID,
		Role:              "master",
		FollowersAffected: enabledFollowersCount,
		Orders:            make([]domain.SquareOffOrder, 0),
		Errors:            make([]string, 0),
	}

	var mu sync.Mutex

	// Step 1 & 2 on Master: Cancel open orders and place counter market orders
	masterResult, err := s.SquareOffAccount(ctx, masterID, symbols)
	if err != nil {
		s.log().Error("squareoff: master square off error", "master_id", masterID, "error", err)
		result.Errors = append(result.Errors, fmt.Sprintf("master %s: %v", masterID, err))
		result.Status = "partial"
	} else {
		result.CancelledOrders += masterResult.CancelledOrders
		result.PositionsSquaredOff += masterResult.PositionsSquaredOff
		result.Orders = append(result.Orders, masterResult.Orders...)
		if len(masterResult.Errors) > 0 {
			result.Errors = append(result.Errors, masterResult.Errors...)
			result.Status = "partial"
		}
	}

	// Step 2: Concurrently square off all enabled followers in this group
	if len(groupDetail.Followers) > 0 {
		var wg sync.WaitGroup
		for _, f := range groupDetail.Followers {
			if !f.Enabled {
				s.log().Info("squareoff: skipping disabled follower", "follower_id", f.AccountID)
				continue
			}
			followerID := f.AccountID
			wg.Add(1)
			go func(fID uuid.UUID) {

				defer wg.Done()
				fResult, fErr := s.SquareOffAccount(ctx, fID, symbols)
				mu.Lock()
				defer mu.Unlock()
				if fErr != nil {
					s.log().Error("squareoff: follower square off error", "follower_id", fID, "error", fErr)
					result.Errors = append(result.Errors, fmt.Sprintf("follower %s: %v", fID, fErr))
					result.Status = "partial"
				} else {
					result.CancelledOrders += fResult.CancelledOrders
					result.PositionsSquaredOff += fResult.PositionsSquaredOff
					result.Orders = append(result.Orders, fResult.Orders...)
					if len(fResult.Errors) > 0 {
						result.Errors = append(result.Errors, fResult.Errors...)
						result.Status = "partial"
					}
				}
			}(followerID)
		}
		wg.Wait()
	}

	s.log().Info("squareoff: group square off finished",
		"group_id", groupID,
		"status", result.Status,
		"cancelled_orders", result.CancelledOrders,
		"positions_closed", result.PositionsSquaredOff,
		"errors_count", len(result.Errors),
	)

	return result, nil
}
