package squareoff_test

import (
	"context"
	"errors"
	"testing"

	"envoytrade/internal/broker"
	"envoytrade/internal/domain"
	"envoytrade/internal/kite/fake"
	"envoytrade/internal/squareoff"

	"github.com/google/uuid"
)

type fakeSquareOffStore struct {
	roles       map[uuid.UUID]string
	groupDetail map[uuid.UUID]domain.GroupDetail
	accounts    map[uuid.UUID]domain.Account
	insertedOrders []domain.FollowerOrder
	placedOrders   map[int64]string
	failedOrders   map[int64]string
}

func (s *fakeSquareOffStore) InsertFollowerOrder(ctx context.Context, o domain.FollowerOrder) (int64, error) {
	s.insertedOrders = append(s.insertedOrders, o)
	return int64(len(s.insertedOrders)), nil
}

func (s *fakeSquareOffStore) UpdateFollowerOrderPlaced(ctx context.Context, id int64, brokerOrderID string, placedQty int) error {
	if s.placedOrders == nil {
		s.placedOrders = make(map[int64]string)
	}
	s.placedOrders[id] = brokerOrderID
	return nil
}

func (s *fakeSquareOffStore) UpdateFollowerOrderFailed(ctx context.Context, id int64, terminalStatus string, errMsg string) error {
	if s.failedOrders == nil {
		s.failedOrders = make(map[int64]string)
	}
	s.failedOrders[id] = errMsg
	return nil
}

func (s *fakeSquareOffStore) AccountRole(ctx context.Context, id uuid.UUID) (string, error) {
	if r, ok := s.roles[id]; ok {
		return r, nil
	}
	return "", domain.ErrNotFound
}

func (s *fakeSquareOffStore) GroupDetail(ctx context.Context, groupID uuid.UUID) (domain.GroupDetail, error) {
	if d, ok := s.groupDetail[groupID]; ok {
		return d, nil
	}
	return domain.GroupDetail{}, domain.ErrNotFound
}

func (s *fakeSquareOffStore) AppendOrderEvent(ctx context.Context, ev domain.OrderEvent) error {
	return nil
}

func (s *fakeSquareOffStore) Accounts(ctx context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	var res []domain.Account
	for _, id := range ids {
		if a, ok := s.accounts[id]; ok {
			res = append(res, a)
		} else if r, ok := s.roles[id]; ok {
			res = append(res, domain.Account{ID: id, Role: r, Enabled: true})
		}
	}
	return res, nil
}



type fakeResolver struct {
	brokers map[uuid.UUID]*fake.Broker
}

func (r *fakeResolver) ResolveBroker(ctx context.Context, accountID uuid.UUID) (broker.Broker, error) {
	if b, ok := r.brokers[accountID]; ok {
		return b, nil
	}
	return nil, errors.New("broker not found")
}

func TestSquareOffAccount_FollowerLongPosition(t *testing.T) {
	fID := uuid.New()
	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{fID: "follower"},
	}

	b := &fake.Broker{
		OrderID: "ORD_EXIT_1",
		OpenOrders: []broker.Order{
			{OrderID: "LIMIT_PENDING", Tradingsymbol: "NIFTY26OCTFUT", Status: "OPEN"},
		},
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{fID: b},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	res, err := svc.SquareOffAccount(context.Background(), fID, nil)
	if err != nil {
		t.Fatalf("SquareOffAccount failed: %v", err)
	}

	if res.Status != "completed" {
		t.Errorf("status = %s, want completed", res.Status)
	}
	if res.PositionsSquaredOff != 1 {
		t.Errorf("positionsSquaredOff = %d, want 1", res.PositionsSquaredOff)
	}
	if res.CancelledOrders != 1 {
		t.Errorf("cancelledOrders = %d, want 1", res.CancelledOrders)
	}
	if len(b.Calls) != 1 {
		t.Fatalf("expected 1 order call, got %d", len(b.Calls))
	}

	call := b.Calls[0]
	if call.TransactionType != "SELL" || call.Quantity != 75 || call.Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("unexpected counter order: %+v", call)
	}

	if len(store.insertedOrders) != 1 {
		t.Fatalf("expected 1 inserted follower order, got %d", len(store.insertedOrders))
	}
	insOrder := store.insertedOrders[0]
	if insOrder.Origin != "square_off" {
		t.Errorf("origin = %s, want square_off", insOrder.Origin)
	}
	if insOrder.Tradingsymbol != "NIFTY26OCTFUT" || insOrder.IntendedQty != 75 || insOrder.TransactionType != "SELL" {
		t.Errorf("unexpected inserted order: %+v", insOrder)
	}
	if store.placedOrders[1] != "ORD_EXIT_1" {
		t.Errorf("placedOrders[1] = %s, want ORD_EXIT_1", store.placedOrders[1])
	}
}

func TestSquareOffAccount_FollowerShortPosition(t *testing.T) {
	fID := uuid.New()
	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{fID: "follower"},
	}

	b := &fake.Broker{
		OrderID: "ORD_EXIT_2",
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{fID: b},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	res, err := svc.SquareOffAccount(context.Background(), fID, nil)
	if err != nil {
		t.Fatalf("SquareOffAccount failed: %v", err)
	}

	if res.PositionsSquaredOff != 1 {
		t.Errorf("positionsSquaredOff = %d, want 1", res.PositionsSquaredOff)
	}
	if len(b.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(b.Calls))
	}
	if b.Calls[0].TransactionType != "BUY" || b.Calls[0].Quantity != 100 {
		t.Errorf("unexpected counter order: %+v", b.Calls[0])
	}
}

func TestSquareOffAccount_SelectiveSymbol(t *testing.T) {
	fID := uuid.New()
	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{fID: "follower"},
	}

	b := &fake.Broker{
		OrderID: "ORD_EXIT_3",
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Product: "NRML", Quantity: 75},
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26P8500", Product: "CNC", Quantity: -100},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{fID: b},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	// Filter strictly for NIFTY
	res, err := svc.SquareOffAccount(context.Background(), fID, []string{"NIFTY26OCTFUT"})
	if err != nil {
		t.Fatalf("SquareOffAccount failed: %v", err)
	}

	if res.PositionsSquaredOff != 1 {
		t.Errorf("positionsSquaredOff = %d, want 1", res.PositionsSquaredOff)
	}
	if len(b.Calls) != 1 || b.Calls[0].Tradingsymbol != "NIFTY26OCTFUT" {
		t.Errorf("expected only NIFTY to be squared off, got %+v", b.Calls)
	}
}

func TestSquareOffAccount_AlreadyZero(t *testing.T) {
	fID := uuid.New()
	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{fID: "follower"},
	}

	b := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NFO", Tradingsymbol: "NIFTY26OCTFUT", Quantity: 0},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{fID: b},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	res, err := svc.SquareOffAccount(context.Background(), fID, nil)
	if err != nil {
		t.Fatalf("SquareOffAccount failed: %v", err)
	}

	if res.Status != "completed" || res.PositionsSquaredOff != 0 {
		t.Errorf("unexpected zero-position result: %+v", res)
	}
	if len(b.Calls) != 0 {
		t.Errorf("expected 0 calls for already flat position, got %d", len(b.Calls))
	}
}

func TestSquareOffGroup_MasterAndFollowersCascaded(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()
	f2ID := uuid.New()

	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
			f2ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Enabled: true},
					{AccountID: f2ID, Enabled: true},
				},

			},
		},
	}

	bMaster := &fake.Broker{
		OrderID: "M_EXIT",
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}
	bF1 := &fake.Broker{
		OrderID: "F1_EXIT",
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}
	bF2 := &fake.Broker{
		OrderID: "F2_EXIT",
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{
			masterID: bMaster,
			f1ID:     bF1,
			f2ID:     bF2,
		},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	res, err := svc.SquareOffGroup(context.Background(), gID, nil)
	if err != nil {
		t.Fatalf("SquareOffGroup failed: %v", err)
	}

	if res.Status != "completed" {
		t.Errorf("status = %s, want completed", res.Status)
	}
	if res.FollowersAffected != 2 {
		t.Errorf("followersAffected = %d, want 2", res.FollowersAffected)
	}
	if res.PositionsSquaredOff != 3 {
		t.Errorf("positionsSquaredOff = %d (1 master + 2 followers), want 3", res.PositionsSquaredOff)
	}

	if len(bMaster.Calls) != 1 {
		t.Errorf("expected 1 master call, got %d", len(bMaster.Calls))
	}
	if len(bF1.Calls) != 1 {
		t.Errorf("expected 1 f1 call, got %d", len(bF1.Calls))
	}
	if len(bF2.Calls) != 1 {
		t.Errorf("expected 1 f2 call, got %d", len(bF2.Calls))
	}
}

func TestSquareOffGroup_PartialFailureResilience(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	f1ID := uuid.New()
	f2ID := uuid.New()

	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{
			masterID: "master",
			f1ID:     "follower",
			f2ID:     "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: f1ID, Enabled: true},
					{AccountID: f2ID, Enabled: true},
				},

			},
		},
	}

	bMaster := &fake.Broker{
		OrderID: "M_EXIT",
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}
	// F1 fails due to network error
	bF1 := &fake.Broker{
		Err: errors.New("network timeout to broker"),
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}
	// F2 succeeds
	bF2 := &fake.Broker{
		OrderID: "F2_EXIT",
		Positions: []broker.Position{
			{Exchange: "MCX", Tradingsymbol: "CRUDEOIL17SEP26C10600", Product: "CNC", Quantity: -100},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{
			masterID: bMaster,
			f1ID:     bF1,
			f2ID:     bF2,
		},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	res, err := svc.SquareOffGroup(context.Background(), gID, nil)
	if err != nil {
		t.Fatalf("SquareOffGroup unexpected err: %v", err)
	}

	if res.Status != "partial" {
		t.Errorf("status = %s, want partial", res.Status)
	}
	if len(res.Errors) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(res.Errors), res.Errors)
	}
	// Master and F2 succeeded
	if res.PositionsSquaredOff != 2 {
		t.Errorf("positionsSquaredOff = %d, want 2", res.PositionsSquaredOff)
	}
	if len(bF2.Calls) != 1 {
		t.Errorf("expected F2 to succeed despite F1 failure")
	}
}

func TestSquareOffGroup_SkipsDisabledFollowers(t *testing.T) {
	gID := uuid.New()
	masterID := uuid.New()
	enabledFID := uuid.New()
	disabledFID := uuid.New()

	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{
			masterID:    "master",
			enabledFID:  "follower",
			disabledFID: "follower",
		},
		groupDetail: map[uuid.UUID]domain.GroupDetail{
			gID: {
				GroupID:  gID,
				MasterID: masterID,
				Followers: []domain.GroupFollower{
					{AccountID: enabledFID, Enabled: true, Name: "Enabled Follower"},
					{AccountID: disabledFID, Enabled: false, Name: "Disabled Follower"},
				},
			},
		},
		accounts: map[uuid.UUID]domain.Account{
			masterID:    {ID: masterID, Role: "master", Active: true},
			enabledFID:  {ID: enabledFID, Role: "follower", Enabled: true},
			disabledFID: {ID: disabledFID, Role: "follower", Enabled: false},
		},
	}

	bMaster := &fake.Broker{
		OrderID: "M_EXIT",
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 10},
		},
	}
	bEnabled := &fake.Broker{
		OrderID: "F_EN_EXIT",
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 10},
		},
	}
	bDisabled := &fake.Broker{
		OrderID: "F_DIS_EXIT",
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 10},
		},
	}

	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{
			masterID:    bMaster,
			enabledFID:  bEnabled,
			disabledFID: bDisabled,
		},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	res, err := svc.SquareOffGroup(context.Background(), gID, nil)
	if err != nil {
		t.Fatalf("SquareOffGroup failed: %v", err)
	}

	if res.Status != "completed" {
		t.Errorf("status = %s, want completed", res.Status)
	}
	// FollowersAffected should only count enabled followers
	if res.FollowersAffected != 1 {
		t.Errorf("FollowersAffected = %d, want 1", res.FollowersAffected)
	}
	// Master and enabled follower squared off
	if res.PositionsSquaredOff != 2 {
		t.Errorf("positionsSquaredOff = %d, want 2", res.PositionsSquaredOff)
	}
	// Enabled follower had 1 order
	if len(bEnabled.Calls) != 1 {
		t.Errorf("expected 1 order on enabled follower, got %d", len(bEnabled.Calls))
	}
	// Disabled follower had ZERO orders
	if len(bDisabled.Calls) != 0 {
		t.Errorf("expected 0 orders on disabled follower, got %d", len(bDisabled.Calls))
	}
}

func TestSquareOffAccount_DisabledFollowerRejected(t *testing.T) {
	disabledFID := uuid.New()

	store := &fakeSquareOffStore{
		roles: map[uuid.UUID]string{
			disabledFID: "follower",
		},
		accounts: map[uuid.UUID]domain.Account{
			disabledFID: {ID: disabledFID, Role: "follower", Enabled: false},
		},
	}

	b := &fake.Broker{
		Positions: []broker.Position{
			{Exchange: "NSE", Tradingsymbol: "RELIANCE", Product: "CNC", Quantity: 10},
		},
	}
	resolver := &fakeResolver{
		brokers: map[uuid.UUID]*fake.Broker{disabledFID: b},
	}

	svc := squareoff.NewService(store, resolver, nil, nil)

	_, err := svc.SquareOffAccount(context.Background(), disabledFID, nil)
	if err == nil {
		t.Fatal("expected error for disabled follower square off, got nil")
	}
	if !errors.Is(err, domain.ErrAccountDisabled) {
		t.Errorf("expected ErrAccountDisabled, got: %v", err)
	}
	if len(b.Calls) != 0 {
		t.Errorf("expected 0 orders placed, got %d", len(b.Calls))
	}
}

