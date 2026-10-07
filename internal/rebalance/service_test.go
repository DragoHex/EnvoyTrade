package rebalance_test

import (
	"context"
	"errors"
	"testing"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/rebalance"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeRebalanceStore struct {
	roles       map[uuid.UUID]string
	groupDetail map[uuid.UUID]domain.GroupDetail
	lotSizes    map[string]int // "EXCHANGE:SYMBOL" -> lotSize
	accounts    map[uuid.UUID]domain.Account
}

func (s *fakeRebalanceStore) AccountRole(ctx context.Context, id uuid.UUID) (string, error) {
	if r, ok := s.roles[id]; ok {
		return r, nil
	}
	return "", domain.ErrNotFound
}

func (s *fakeRebalanceStore) GroupDetail(ctx context.Context, groupID uuid.UUID) (domain.GroupDetail, error) {
	if d, ok := s.groupDetail[groupID]; ok {
		return d, nil
	}
	return domain.GroupDetail{}, domain.ErrNotFound
}

func (s *fakeRebalanceStore) InstrumentLotSize(ctx context.Context, exchange, symbol string) (int, error) {
	key := exchange + ":" + symbol
	if ls, ok := s.lotSizes[key]; ok {
		return ls, nil
	}
	return 0, domain.ErrNotFound
}

func (s *fakeRebalanceStore) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	return nil
}

func (s *fakeRebalanceStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	var res []domain.Account
	for _, id := range ids {
		if a, ok := s.accounts[id]; ok {
			res = append(res, a)
		}
	}
	return res, nil
}

type failingOrderBroker struct {
	*fake.Broker
	orderErr error
}

func (f *failingOrderBroker) PlaceOrder(ctx context.Context, variety string, params broker.OrderParams) (broker.OrderResponse, error) {
	if f.orderErr != nil {
		return broker.OrderResponse{}, f.orderErr
	}
	return f.Broker.PlaceOrder(ctx, variety, params)
}

type fakeResolver struct {
	brokers map[uuid.UUID]broker.Broker
}

func (r *fakeResolver) ResolveBroker(ctx context.Context, accountID uuid.UUID) (broker.Broker, error) {
	if b, ok := r.brokers[accountID]; ok {
		return b, nil
	}
	return nil, errors.New("broker not found")
}



func TestComputeGroupDiff_SingleDriftingFollower(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{
						AccountID:       f1ID,
						Name:            "Follower 1",
						BrokerAccountID: "BRK_F1",
						Enabled:         true,
					},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:          f1ID,
				Name:        "Follower 1",
				CloneFactor: &cloneFactor,
				Enabled:     true,
				MasterID:    &masterID,
			},
		},
		lotSizes: map[string]int{
			"NFO:NIFTY26OCTFUT": 75,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}
	bF1 := &fake.Broker{
		Positions: []broker.Position{}, // 0 qty -> missed signal
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	diff, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err != nil {
		t.Fatalf("ComputeGroupDiff failed: %v", err)
	}

	if diff.FollowersEvaluated != 1 {
		t.Errorf("expected 1 follower evaluated, got %d", diff.FollowersEvaluated)
	}
	if diff.FollowersWithDrift != 1 {
		t.Errorf("expected 1 follower with drift, got %d", diff.FollowersWithDrift)
	}
	if len(diff.Drifts) != 1 {
		t.Fatalf("expected 1 drift record, got %d", len(diff.Drifts))
	}

	fDrift := diff.Drifts[0]
	if fDrift.AccountID != f1ID {
		t.Errorf("drift account ID = %s, want %s", fDrift.AccountID, f1ID)
	}
	if len(fDrift.Symbols) != 1 {
		t.Fatalf("expected 1 symbol drift, got %d", len(fDrift.Symbols))
	}

	sDrift := fDrift.Symbols[0]
	if sDrift.Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("tradingsymbol = %s, want NIFTY26OCTFUT", sDrift.Tradingsymbol)
	}
	if sDrift.MasterQty != 75 || sDrift.TargetQty != 75 || sDrift.FollowerQty != 0 || sDrift.DriftQty != 75 {
		t.Errorf("unexpected drift calculation: %+v", sDrift)
	}
	if sDrift.Action != "BUY" {
		t.Errorf("action = %s, want BUY", sDrift.Action)
	}
}

func TestComputeGroupDiff_RoguePositionTargetZero(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:          f1ID,
				Name:        "Follower 1",
				CloneFactor: &cloneFactor,
				Enabled:     true,
				MasterID:    &masterID,
			},
		},
		lotSizes: map[string]int{
			"NSE:RELIANCE": 1,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{}, // Master has 0 positions
	}
	bF1 := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 50},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	diff, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err != nil {
		t.Fatalf("ComputeGroupDiff failed: %v", err)
	}

	if diff.FollowersWithDrift != 1 || len(diff.Drifts[0].Symbols) != 1 {
		t.Fatalf("expected 1 drift with 1 symbol, got %+v", diff)
	}

	sym := diff.Drifts[0].Symbols[0]
	if sym.MasterQty != 0 || sym.TargetQty != 0 || sym.FollowerQty != 50 || sym.DriftQty != -50 {
		t.Errorf("unexpected rogue drift: %+v", sym)
	}
	if sym.Action != "SELL" {
		t.Errorf("action = %s, want SELL", sym.Action)
	}
}

func TestRebalanceGroup_PlacesOrdersAndCancelsPending(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:          f1ID,
				Role:        "follower",
				Name:        "Follower 1",
				CloneFactor: &cloneFactor,
				Enabled:     true,
				MasterID:    &masterID,
			},
		},
		lotSizes: map[string]int{
			"NFO:NIFTY26OCTFUT": 75,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}
	bF1 := &fake.Broker{
		OrderID: "ORD_REBAL_1",
		OpenOrders: []broker.Order{
			{OrderID: "PENDING_LIMIT_1", Tradingsymbol: "NIFTY26OCTFUT", Status: "OPEN"},
		},
		Positions: []broker.Position{}, // 0 qty -> needs +75 BUY
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	res, err := svc.RebalanceGroup(context.Background(), gID, []uuid.UUID{f1ID})
	if err != nil {
		t.Fatalf("RebalanceGroup failed: %v", err)
	}

	if res.Status != "completed" {
		t.Errorf("status = %s, want completed", res.Status)
	}
	if res.OrdersPlaced != 1 {
		t.Errorf("ordersPlaced = %d, want 1", res.OrdersPlaced)
	}
	if res.CancelledOrders != 1 {
		t.Errorf("cancelledOrders = %d, want 1", res.CancelledOrders)
	}
	if len(bF1.Calls) != 1 {
		t.Fatalf("expected 1 call on follower, got %d", len(bF1.Calls))
	}
	call := bF1.Calls[0]
	if call.TransactionType != "BUY" || call.Quantity != 75 || call.Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("unexpected rebalance order: %+v", call)
	}
	if len(bMaster.Calls) != 0 {
		t.Errorf("master account should never place orders during rebalance, got %d calls", len(bMaster.Calls))
	}
}

func TestRebalanceAccount_SingleFollower(t *testing.T) {
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(0.5)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			f1ID: "follower",
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:          f1ID,
				Role:        "follower",
				Name:        "Follower 1",
				CloneFactor: &cloneFactor,
				Enabled:     true,
				MasterID:    &masterID,
			},
		},
		lotSizes: map[string]int{
			"NFO:NIFTY26OCTFUT": 50,
		},
	}

	// Master has 100 qty. Follower clone factor = 0.5 -> Target = 50.
	// Follower current qty = 0 -> Needs BUY 50.
	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 100},
		},
	}
	bF1 := &fake.Broker{
		OrderID:   "ORD_REBAL_F1",
		Positions: []broker.Position{},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	diff, err := svc.ComputeAccountDiff(context.Background(), f1ID)
	if err != nil {
		t.Fatalf("ComputeAccountDiff failed: %v", err)
	}
	if len(diff.Symbols) != 1 || diff.Symbols[0].TargetQty != 50 || diff.Symbols[0].DriftQty != 50 {
		t.Errorf("unexpected diff: %+v", diff)
	}

	res, err := svc.RebalanceAccount(context.Background(), f1ID)
	if err != nil {
		t.Fatalf("RebalanceAccount failed: %v", err)
	}
	if res.Status != "completed" || res.OrdersPlaced != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	if len(bF1.Calls) != 1 || bF1.Calls[0].Quantity != 50 || bF1.Calls[0].TransactionType != "BUY" {
		t.Errorf("unexpected order placed: %+v", bF1.Calls)
	}
}

func TestRebalanceGroup_CleanEquilibrium(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:          f1ID,
				Role:        "follower",
				Name:        "Follower 1",
				CloneFactor: &cloneFactor,
				Enabled:     true,
				MasterID:    &masterID,
			},
		},
		lotSizes: map[string]int{
			"NFO:NIFTY26OCTFUT": 75,
		},
	}

	// Both master and follower have identical 75 positions
	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}
	bF1 := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	diff, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err != nil {
		t.Fatalf("ComputeGroupDiff failed: %v", err)
	}
	if diff.FollowersWithDrift != 0 || len(diff.Drifts) != 0 {
		t.Errorf("expected 0 drift in equilibrium, got %+v", diff)
	}

	res, err := svc.RebalanceGroup(context.Background(), gID, nil)
	if err != nil {
		t.Fatalf("RebalanceGroup failed: %v", err)
	}
	if res.Status != "completed" || res.OrdersPlaced != 0 {
		t.Errorf("unexpected result for clean equilibrium: %+v", res)
	}
	if len(bF1.Calls) != 0 {
		t.Errorf("expected 0 orders placed, got %d", len(bF1.Calls))
	}
}

func TestRebalanceGroup_PartialBrokerFailure(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:          f1ID,
				Role:        "follower",
				Name:        "Follower 1",
				CloneFactor: &cloneFactor,
				Enabled:     true,
				MasterID:    &masterID,
			},
		},
		lotSizes: map[string]int{
			"NFO:NIFTY26OCTFUT": 75,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}
	bF1 := &failingOrderBroker{
		Broker: &fake.Broker{
			Positions: []broker.Position{}, // 0 qty -> needs BUY 75
		},
		orderErr: errors.New("insufficient funds / margin error"),
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	res, err := svc.RebalanceGroup(context.Background(), gID, nil)
	if err != nil {
		t.Fatalf("RebalanceGroup returned error: %v", err)
	}
	if res.Status != "partial" {
		t.Errorf("status = %s, want partial", res.Status)
	}
	if len(res.Errors) != 1 {
		t.Errorf("expected 1 error recorded, got %d: %+v", len(res.Errors), res.Errors)
	}
}

type authFailingBroker struct {
	*fake.Broker
	failFirst bool
	hasFailed bool
}

type fakeSyncer struct {
	syncFn func(ctx context.Context, id uuid.UUID) error
}

func (s *fakeSyncer) SyncAccountPortfolio(ctx context.Context, accountID uuid.UUID) error {
	if s.syncFn != nil {
		return s.syncFn(ctx, accountID)
	}
	return nil
}

func (b *authFailingBroker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	if b.failFirst && !b.hasFailed {
		b.hasFailed = true
		return nil, errors.New("kite get positions: Incorrect `api_key` or `access_token`.")
	}
	return b.Broker.GetPositions(ctx)
}

func TestComputeGroupDiff_MasterAuthError_SyncsAndRecovers(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{
						AccountID:       f1ID,
						Name:            "Follower 1",
						BrokerAccountID: "BRK_F1",
						Enabled:         true,
					},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:              f1ID,
				Name:            "Follower 1",
				BrokerAccountID: "BRK_F1",
				Role:            "follower",
				MasterID:        &masterID,
				Enabled:         true,
				CloneFactor:     &cloneFactor,
			},
		},
		lotSizes: map[string]int{
			"NFO:NIFTY26OCTFUT": 75,
		},
	}

	bMaster := &authFailingBroker{
		Broker: &fake.Broker{
			Positions: []broker.Position{
				{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
			},
		},
		failFirst: true,
	}
	bF1 := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
			f1ID:     bF1,
		},
	}

	syncedMaster := false
	syncer := &fakeSyncer{
		syncFn: func(ctx context.Context, id uuid.UUID) error {
			if id == masterID {
				syncedMaster = true
			}
			return nil
		},
	}

	svc := rebalance.NewService(store, resolver, syncer, nil)

	diff, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err != nil {
		t.Fatalf("ComputeGroupDiff should recover after auth error, got: %v", err)
	}
	if !syncedMaster {
		t.Fatalf("expected syncer to be called for master")
	}
	if diff.FollowersEvaluated != 1 {
		t.Errorf("FollowersEvaluated = %d, want 1", diff.FollowersEvaluated)
	}
}

func TestComputeGroupDiff_MasterBrokerUnreachable(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	store := &fakeRebalanceStore{
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
	}

	bMaster := &fake.Broker{
		Err: errors.New("dial tcp: connection refused"),
	}
	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)
	_, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrBrokerUnreachable) {
		t.Fatalf("expected domain.ErrBrokerUnreachable, got: %v", err)
	}
}

func TestComputeGroupDiff_MasterAuthExpired(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	store := &fakeRebalanceStore{
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
	}

	bMaster := &fake.Broker{
		Err: errors.New("TokenException: Token is invalid"),
	}
	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)
	_, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrAuthExpired) {
		t.Fatalf("expected domain.ErrAuthExpired, got: %v", err)
	}
}

func TestComputeGroupDiff_FollowerBrokerUnreachable(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	store := &fakeRebalanceStore{
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:       f1ID,
				Role:     "follower",
				Name:     "Follower 1",
				Enabled:  true,
				MasterID: &masterID,
			},
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{},
	}
	bFollower := &fake.Broker{
		Err: errors.New("context deadline exceeded: timeout"),
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID:  bMaster,
			f1ID:      bFollower,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)
	_, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrBrokerUnreachable) {
		t.Fatalf("expected domain.ErrBrokerUnreachable, got: %v", err)
	}
}

func TestRebalanceGroup_DisallowsRebalanceWhenBrokerUnreachable(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	store := &fakeRebalanceStore{
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Follower 1", Enabled: true},
				},
			},
		},
	}

	bMaster := &fake.Broker{
		Err: errors.New("503 Service Unavailable"),
	}
	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID: bMaster,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)
	res, err := svc.RebalanceGroup(context.Background(), gID, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrBrokerUnreachable) {
		t.Fatalf("expected domain.ErrBrokerUnreachable, got: %v", err)
	}
	if res.OrdersPlaced > 0 || len(res.Orders) > 0 {
		t.Fatalf("expected zero orders placed on failed live check, got %d", res.OrdersPlaced)
	}
}

func TestComputeGroupDiff_IncludesDisabledFollower(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Name: "Disabled Follower", BrokerAccountID: "FOL_DIS", Enabled: false},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			f1ID: {
				ID:              f1ID,
				Name:            "Disabled Follower",
				BrokerAccountID: "FOL_DIS",
				Role:            "follower",
				MasterID:        &masterID,
				Enabled:         false,
				CloneFactor:     &cloneFactor,
			},
		},
		lotSizes: map[string]int{
			"NSE:INFY": 1,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "INFY", Product: "CNC", Quantity: 100},
		},
	}
	bFollower := &fake.Broker{
		Positions: []broker.Position{}, // 0 quantity -> drift = +100
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID:  bMaster,
			f1ID:      bFollower,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)
	diff, err := svc.ComputeGroupDiff(context.Background(), gID)
	if err != nil {
		t.Fatalf("ComputeGroupDiff unexpected err: %v", err)
	}

	if diff.FollowersWithDrift != 1 {
		t.Fatalf("expected 1 follower with drift, got %d", diff.FollowersWithDrift)
	}
	if len(diff.Drifts) != 1 {
		t.Fatalf("expected 1 drift row, got %d", len(diff.Drifts))
	}
	if diff.Drifts[0].Enabled != false {
		t.Errorf("expected drift row to preserve Enabled = false, got %v", diff.Drifts[0].Enabled)
	}
}

func TestRebalanceGroup_SkipsDisabledFollowers(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	fEnabledID := uuid.New()
	fDisabledID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID:    "master",
			fEnabledID:  "follower",
			fDisabledID: "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: fEnabledID, Name: "Enabled Follower", BrokerAccountID: "FOL_EN", Enabled: true},
					{AccountID: fDisabledID, Name: "Disabled Follower", BrokerAccountID: "FOL_DIS", Enabled: false},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			fEnabledID: {
				ID:              fEnabledID,
				Name:            "Enabled Follower",
				BrokerAccountID: "FOL_EN",
				Role:            "follower",
				MasterID:        &masterID,
				Enabled:         true,
				CloneFactor:     &cloneFactor,
			},
			fDisabledID: {
				ID:              fDisabledID,
				Name:            "Disabled Follower",
				BrokerAccountID: "FOL_DIS",
				Role:            "follower",
				MasterID:        &masterID,
				Enabled:         false,
				CloneFactor:     &cloneFactor,
			},
		},
		lotSizes: map[string]int{
			"NSE:RELIANCE": 1,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 50},
		},
	}
	bEnabled := &fake.Broker{
		Positions: []broker.Position{}, // drift = +50
	}
	bDisabled := &fake.Broker{
		Positions: []broker.Position{}, // drift = +50
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID:    bMaster,
			fEnabledID:  bEnabled,
			fDisabledID: bDisabled,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	// Case 1: Rebalance all (follower_ids is nil)
	res, err := svc.RebalanceGroup(context.Background(), gID, nil)
	if err != nil {
		t.Fatalf("RebalanceGroup unexpected err: %v", err)
	}

	if res.FollowersAffected != 1 {
		t.Errorf("FollowersAffected = %d, want 1", res.FollowersAffected)
	}
	if len(bEnabled.Calls) != 1 {
		t.Errorf("expected 1 order on enabled follower, got %d", len(bEnabled.Calls))
	}
	if len(bDisabled.Calls) != 0 {
		t.Errorf("expected 0 orders on disabled follower, got %d", len(bDisabled.Calls))
	}

	// Reset calls
	bEnabled.Calls = nil
	bDisabled.Calls = nil

	// Case 2: Explicitly requested disabled follower ID
	res2, err := svc.RebalanceGroup(context.Background(), gID, []uuid.UUID{fDisabledID})
	if err != nil {
		t.Fatalf("RebalanceGroup with disabled follower unexpected err: %v", err)
	}
	if res2.FollowersAffected != 0 {
		t.Errorf("FollowersAffected = %d, want 0 when only disabled follower was requested", res2.FollowersAffected)
	}
	if len(bDisabled.Calls) != 0 {
		t.Errorf("expected 0 orders on disabled follower even when explicitly requested, got %d", len(bDisabled.Calls))
	}
}

func TestRebalanceAccount_DisabledFollowerRejected(t *testing.T) {
	masterID := uuid.New()
	fDisabledID := uuid.New()

	cloneFactor := decimal.NewFromFloat(1.0)
	store := &fakeRebalanceStore{
		roles: map[uuid.UUID]string{
			masterID:    "master",
			fDisabledID: "follower",
		},
		accounts: map[uuid.UUID]domain.Account{
			fDisabledID: {
				ID:              fDisabledID,
				Name:            "Disabled Follower",
				BrokerAccountID: "FOL_DIS",
				Role:            "follower",
				MasterID:        &masterID,
				Enabled:         false,
				CloneFactor:     &cloneFactor,
			},
		},
		lotSizes: map[string]int{
			"NSE:RELIANCE": 1,
		},
	}

	bMaster := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 50},
		},
	}
	bDisabled := &fake.Broker{
		Positions: []broker.Position{},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]broker.Broker{
			masterID:    bMaster,
			fDisabledID: bDisabled,
		},
	}

	svc := rebalance.NewService(store, resolver, nil, nil)

	_, err := svc.RebalanceAccount(context.Background(), fDisabledID)
	if err == nil {
		t.Fatal("expected error for disabled follower rebalance, got nil")
	}
	if !errors.Is(err, domain.ErrAccountDisabled) {
		t.Errorf("expected ErrAccountDisabled, got: %v", err)
	}
	if len(bDisabled.Calls) != 0 {
		t.Errorf("expected 0 orders on disabled follower, got %d", len(bDisabled.Calls))
	}
}



