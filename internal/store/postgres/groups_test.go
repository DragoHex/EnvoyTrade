//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestGroups_ListsMastersWithFollowerCountAndStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	otherMaster := seedAccount(t, s, "master")
	f1 := seedAccount(t, s, "follower")
	f2 := seedAccount(t, s, "follower")

	for _, l := range []domain.FollowLink{
		{FollowerID: f1, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true},
		{FollowerID: f2, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: false},
	} {
		if err := s.CreateFollowLink(ctx, l); err != nil {
			t.Fatalf("CreateFollowLink: %v", err)
		}
	}

	groups, err := s.Groups(ctx)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	var got *domain.GroupSummary
	for i := range groups {
		if groups[i].MasterID == master {
			got = &groups[i]
		}
	}
	if got == nil {
		t.Fatalf("master %v not present in Groups() result", master)
	}
	if got.FollowerCount != 2 {
		t.Errorf("FollowerCount = %d, want 2", got.FollowerCount)
	}
	if got.Status != "ok" {
		t.Errorf("Status = %q, want ok", got.Status)
	}
	if got.Broker != "zerodha" {
		t.Errorf("Broker = %q, want zerodha (schema default)", got.Broker)
	}
	_ = otherMaster
}

func TestGroups_EmptyWhenNoMasters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	groups, err := s.Groups(ctx)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0", len(groups))
	}
}

func TestGroupDetail_ReturnsMasterAndFollowers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")

	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	detail, err := s.GroupDetail(ctx, master)
	if err != nil {
		t.Fatalf("GroupDetail: %v", err)
	}
	if detail.MasterID != master {
		t.Errorf("MasterID = %v, want %v", detail.MasterID, master)
	}
	if len(detail.Followers) != 1 {
		t.Fatalf("got %d followers, want 1", len(detail.Followers))
	}
	if detail.Followers[0].AccountID != follower {
		t.Errorf("follower id = %v, want %v", detail.Followers[0].AccountID, follower)
	}
	if !detail.Followers[0].Enabled {
		t.Errorf("follower Enabled = false, want true")
	}
}

func TestGroupDetail_UnknownMasterReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GroupDetail(ctx, uuid.New())
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestGroupDetail_NonMasterAccountReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	follower := seedAccount(t, s, "follower")

	_, err := s.GroupDetail(ctx, follower)
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestSetFollowLinkEnabled_TogglesEnabledFlag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	if err := s.SetFollowLinkEnabled(ctx, follower, false); err != nil {
		t.Fatalf("SetFollowLinkEnabled: %v", err)
	}

	detail, err := s.GroupDetail(ctx, master)
	if err != nil {
		t.Fatalf("GroupDetail: %v", err)
	}
	if detail.Followers[0].Enabled {
		t.Errorf("Enabled = true, want false after SetFollowLinkEnabled(false)")
	}
}

func TestSetFollowLinkEnabled_UnknownFollowerReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.SetFollowLinkEnabled(ctx, uuid.New(), false)
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestGroups_NewMasterIsActiveByDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	groups, err := s.Groups(ctx)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) != 1 || groups[0].MasterID != master {
		t.Fatalf("Groups = %+v, want one entry for %v", groups, master)
	}
	if !groups[0].Active {
		t.Errorf("Active = false, want true (schema default)")
	}
}

func TestGroupDetail_MasterActiveReflectsAccountsActive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	detail, err := s.GroupDetail(ctx, master)
	if err != nil {
		t.Fatalf("GroupDetail: %v", err)
	}
	if !detail.MasterActive {
		t.Errorf("MasterActive = false, want true (schema default)")
	}

	if err := s.SetAccountActive(ctx, master, false); err != nil {
		t.Fatalf("SetAccountActive: %v", err)
	}
	detail, err = s.GroupDetail(ctx, master)
	if err != nil {
		t.Fatalf("GroupDetail: %v", err)
	}
	if detail.MasterActive {
		t.Errorf("MasterActive = true, want false after SetAccountActive(false)")
	}
}

func TestSetAccountActive_UnknownAccountReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.SetAccountActive(ctx, uuid.New(), false)
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestMasterActive_ReturnsAccountsActiveColumn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	active, err := s.MasterActive(ctx, master)
	if err != nil {
		t.Fatalf("MasterActive: %v", err)
	}
	if !active {
		t.Errorf("MasterActive = false, want true (schema default)")
	}

	if err := s.SetAccountActive(ctx, master, false); err != nil {
		t.Fatalf("SetAccountActive: %v", err)
	}
	active, err = s.MasterActive(ctx, master)
	if err != nil {
		t.Fatalf("MasterActive: %v", err)
	}
	if active {
		t.Errorf("MasterActive = true, want false after SetAccountActive(false)")
	}
}

func TestMasterActive_NonMasterReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	follower := seedAccount(t, s, "follower")

	_, err := s.MasterActive(ctx, follower)
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestLatestMasterFill_ReturnsMostRecent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	older := domain.MasterFill{
		MasterID: master, BrokerOrderID: "1", Exchange: "NSE", Tradingsymbol: "INFY",
		InstrumentToken: 1, TransactionType: "BUY", Product: "MIS", OrderType: "MARKET",
		FilledQuantity: 10, AveragePrice: decimal.NewFromInt(100), Status: "COMPLETE",
		OrderTimestamp: time.Now().Add(-time.Hour), RawPayload: []byte(`{}`),
	}
	newer := older
	newer.BrokerOrderID = "2"
	newer.OrderTimestamp = time.Now()

	if _, err := s.InsertMasterFill(ctx, older); err != nil {
		t.Fatalf("InsertMasterFill(older): %v", err)
	}
	if _, err := s.InsertMasterFill(ctx, newer); err != nil {
		t.Fatalf("InsertMasterFill(newer): %v", err)
	}

	got, err := s.LatestMasterFill(ctx, master)
	if err != nil {
		t.Fatalf("LatestMasterFill: %v", err)
	}
	if got.BrokerOrderID != "2" {
		t.Errorf("BrokerOrderID = %q, want 2 (most recent by order_timestamp)", got.BrokerOrderID)
	}
}

func TestResolveMasterID_ReturnsMasterID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")
	follower := seedAccount(t, s, "follower")
	if err := s.CreateFollowLink(ctx, domain.FollowLink{
		FollowerID: follower, MasterID: master, CapitalRatio: decimal.NewFromFloat(0.5), Enabled: true,
	}); err != nil {
		t.Fatalf("CreateFollowLink: %v", err)
	}

	got, err := s.ResolveMasterID(ctx, follower)
	if err != nil {
		t.Fatalf("ResolveMasterID: %v", err)
	}
	if got != master {
		t.Errorf("ResolveMasterID = %v, want %v", got, master)
	}
}

func TestResolveMasterID_MasterIDResolvesToItself(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	got, err := s.ResolveMasterID(ctx, master)
	if err != nil {
		t.Fatalf("ResolveMasterID: %v", err)
	}
	if got != master {
		t.Errorf("ResolveMasterID(master) = %v, want %v (itself)", got, master)
	}
}

func TestResolveMasterID_UnknownFollowerReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.ResolveMasterID(ctx, uuid.New())
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestLatestMasterFill_NoneReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	master := seedAccount(t, s, "master")

	_, err := s.LatestMasterFill(ctx, master)
	if err != domain.ErrNotFound {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}
