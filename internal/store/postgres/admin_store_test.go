//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestAdminStore_ProxyIPBatchAndExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	ips := []domain.ProxyIP{
		{
			IPAddress:  "198.51.100.10",
			IPType:     "ipv4",
			Host:       "proxy.example.com",
			Port:       443,
			Username:   "user1",
			Password:   "pass1",
			ValidFrom:  now.Add(-24 * time.Hour),
			ValidUntil: now.Add(30 * 24 * time.Hour), // active (>15d)
			Plan:       "QUARTERLY",
		},
		{
			IPAddress:  "198.51.100.20",
			IPType:     "ipv4",
			Host:       "proxy.example.com",
			Port:       443,
			Username:   "user2",
			Password:   "pass2",
			ValidFrom:  now.Add(-24 * time.Hour),
			ValidUntil: now.Add(5 * 24 * time.Hour), // expiring soon (<=15d)
			Plan:       "QUARTERLY",
		},
		{
			IPAddress:  "2001:db8::1",
			IPType:     "ipv6",
			Host:       "proxy.example.com",
			Port:       443,
			Username:   "user3",
			Password:   "pass3",
			ValidFrom:  now.Add(-48 * time.Hour),
			ValidUntil: now.Add(-2 * time.Hour), // expired
			Plan:       "QUARTERLY",
		},
	}

	// 1. Initial Batch Upsert
	res, err := store.UpsertProxyIPsBatch(ctx, ips)
	if err != nil {
		t.Fatalf("UpsertProxyIPsBatch failed: %v", err)
	}
	if res.Inserted != 3 || res.Updated != 0 {
		t.Errorf("expected 3 inserted, 0 updated; got %d inserted, %d updated", res.Inserted, res.Updated)
	}

	// 2. Query stats
	stats, err := store.ProxyIPStats(ctx)
	if err != nil {
		t.Fatalf("ProxyIPStats failed: %v", err)
	}
	if stats.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3", stats.TotalCount)
	}
	if stats.IPv4Count != 2 {
		t.Errorf("IPv4Count = %d, want 2", stats.IPv4Count)
	}
	if stats.IPv6Count != 1 {
		t.Errorf("IPv6Count = %d, want 1", stats.IPv6Count)
	}
	if stats.ExpiringSoonCount != 1 {
		t.Errorf("ExpiringSoonCount = %d, want 1", stats.ExpiringSoonCount)
	}
	if stats.ExpiredCount != 1 {
		t.Errorf("ExpiredCount = %d, want 1", stats.ExpiredCount)
	}

	// 3. Second batch: update 1 IP, insert 1 new IP
	updateBatch := []domain.ProxyIP{
		{
			IPAddress:  "198.51.100.10",
			IPType:     "ipv4",
			Host:       "proxy.example.com",
			Port:       443,
			Username:   "user1_updated",
			Password:   "newpass1",
			ValidFrom:  now,
			ValidUntil: now.Add(90 * 24 * time.Hour),
			Plan:       "ANNUAL",
		},
		{
			IPAddress:  "198.51.100.30",
			IPType:     "ipv4",
			Host:       "proxy.example.com",
			Port:       443,
			Username:   "user4",
			Password:   "pass4",
			ValidFrom:  now,
			ValidUntil: now.Add(30 * 24 * time.Hour),
			Plan:       "QUARTERLY",
		},
	}
	res2, err := store.UpsertProxyIPsBatch(ctx, updateBatch)
	if err != nil {
		t.Fatalf("second UpsertProxyIPsBatch failed: %v", err)
	}
	if res2.Inserted != 1 || res2.Updated != 1 {
		t.Errorf("expected 1 inserted, 1 updated; got %d inserted, %d updated", res2.Inserted, res2.Updated)
	}

	// 4. UpdateProxyIP
	err = store.UpdateProxyIP(ctx, "198.51.100.30", now.Add(100*24*time.Hour), "pass4_mod", "MONTHLY")
	if err != nil {
		t.Fatalf("UpdateProxyIP failed: %v", err)
	}
	err = store.UpdateProxyIP(ctx, "non.existent.ip", now, "pass", "PLAN")
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent IP, got: %v", err)
	}

	// 4b. UpdateProxyIPFull (single DB call)
	fullIP := domain.ProxyIP{
		IPAddress:  "198.51.100.30",
		IPType:     "ipv4",
		Host:       "updated.proxy.com",
		Port:       8443,
		Username:   "user4_full",
		Password:   "", // blank password should preserve existing 'pass4_mod'
		ValidFrom:  now,
		ValidUntil: now.Add(120 * 24 * time.Hour),
		Plan:       "ANNUAL",
	}
	err = store.UpdateProxyIPFull(ctx, fullIP)
	if err != nil {
		t.Fatalf("UpdateProxyIPFull failed: %v", err)
	}
	fetched, err := store.ProxyIPByAddress(ctx, "198.51.100.30")
	if err != nil {
		t.Fatalf("ProxyIPByAddress failed: %v", err)
	}
	if fetched.Host != "updated.proxy.com" || fetched.Port != 8443 || fetched.Username != "user4_full" || fetched.Plan != "ANNUAL" {
		t.Errorf("UpdateProxyIPFull did not update attributes correctly: %+v", fetched)
	}
	if fetched.Password != "pass4_mod" {
		t.Errorf("expected empty password to preserve existing password 'pass4_mod', got %q", fetched.Password)
	}
	err = store.UpdateProxyIPFull(ctx, domain.ProxyIP{IPAddress: "non.existent.ip"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for non-existent IP, got: %v", err)
	}

	// 5. DeleteProxyIP (unassigned)
	err = store.DeleteProxyIP(ctx, "198.51.100.30")
	if err != nil {
		t.Fatalf("DeleteProxyIP failed: %v", err)
	}

	// 6. DeleteProxyIP assigned to account -> should fail with ErrConflict
	accID := uuid.New()
	err = store.CreateAccount(ctx, accID, "Test Account", "master", "zerodha", "USER123", "key", "secret", "198.51.100.10")
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	err = store.DeleteProxyIP(ctx, "198.51.100.10")
	if err != domain.ErrConflict {
		t.Errorf("expected ErrConflict deleting assigned IP, got: %v", err)
	}
}

func TestAdminStore_UserManagement(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	u1ID := uuid.New()
	err := store.CreateUser(ctx, u1ID, "trader1@example.com", "trader1", "Trader One", "hash", domain.RoleUser)
	if err != nil {
		t.Fatalf("CreateUser u1 failed: %v", err)
	}

	u2ID := uuid.New()
	err = store.CreateUser(ctx, u2ID, "admin1@example.com", "admin1", "Admin One", "hash", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("CreateUser u2 failed: %v", err)
	}

	// 1. ListUsers
	users, total, err := store.ListUsers(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if total < 2 {
		t.Errorf("total = %d, want >= 2", total)
	}
	if len(users) < 2 {
		t.Errorf("len(users) = %d, want >= 2", len(users))
	}

	// 2. UpdateUserRoleAndStatus
	activeFalse := false
	err = store.UpdateUserRoleAndStatus(ctx, u1ID, domain.RoleAdmin, &activeFalse)
	if err != nil {
		t.Fatalf("UpdateUserRoleAndStatus failed: %v", err)
	}

	// 3. Create sessions and list
	tokenHash1 := "hash_sess_1"
	err = store.CreateSession(ctx, tokenHash1, u1ID, "127.0.0.1", "Mozilla/5.0", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	tokenHash2 := "hash_sess_2"
	err = store.CreateSession(ctx, tokenHash2, u1ID, "127.0.0.2", "Mozilla/5.0", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("CreateSession 2 failed: %v", err)
	}

	sessions, err := store.ListUserSessions(ctx, u1ID)
	if err != nil {
		t.Fatalf("ListUserSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 active sessions, got %d", len(sessions))
	}

	// 4. RevokeAllUserSessions
	deleted, err := store.RevokeAllUserSessions(ctx, u1ID)
	if err != nil {
		t.Fatalf("RevokeAllUserSessions failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	sessionsAfter, err := store.ListUserSessions(ctx, u1ID)
	if err != nil {
		t.Fatalf("ListUserSessions after revoke failed: %v", err)
	}
	if len(sessionsAfter) != 0 {
		t.Errorf("expected 0 sessions after revoke, got %d", len(sessionsAfter))
	}

	// 5. AccountsByUserID
	acctID := uuid.New()
	err = store.CreateAccount(domain.ContextWithUser(ctx, &domain.User{ID: u1ID}), acctID, "Test Master", "master", "testbroker", "TM1", "k1", "s1", "")
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	u1Accounts, err := store.AccountsByUserID(ctx, u1ID)
	if err != nil {
		t.Fatalf("AccountsByUserID failed: %v", err)
	}
	if len(u1Accounts) != 1 || u1Accounts[0].ID != acctID {
		t.Errorf("expected 1 account for u1ID with ID %s, got %+v", acctID, u1Accounts)
	}

	u2Accounts, err := store.AccountsByUserID(ctx, u2ID)
	if err != nil {
		t.Fatalf("AccountsByUserID u2 failed: %v", err)
	}
	if len(u2Accounts) != 0 {
		t.Errorf("expected 0 accounts for u2ID, got %d", len(u2Accounts))
	}
}

func TestAdminStore_BrokerReadinessAndOrderIssues(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	validExpiry := now.Add(12 * time.Hour)
	expiredTime := now.Add(-2 * time.Hour)

	// 1. Setup Accounts
	masterID := uuid.New()
	err := store.CreateAccount(ctx, masterID, "Primary Master", "master", "zerodha", "MST001", "key1", "sec1", "")
	if err != nil {
		t.Fatalf("create master account: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, masterID, "tok_m1", &validExpiry, "authenticated", "")

	follower1 := uuid.New()
	err = store.CreateAccount(ctx, follower1, "Ready Follower", "follower", "zerodha", "FOL001", "key2", "sec2", "")
	if err != nil {
		t.Fatalf("create follower 1: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, follower1, "tok_f1", &validExpiry, "authenticated", "")

	follower2 := uuid.New()
	err = store.CreateAccount(ctx, follower2, "Expired Follower", "follower", "zerodha", "FOL002", "key3", "sec3", "")
	if err != nil {
		t.Fatalf("create follower 2: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, follower2, "tok_f2", &expiredTime, "authenticated", "")

	follower3 := uuid.New()
	err = store.CreateAccount(ctx, follower3, "Unauth Follower", "follower", "zerodha", "FOL003", "key4", "sec4", "")
	if err != nil {
		t.Fatalf("create follower 3: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, follower3, "", &expiredTime, "unauthenticated", "Token revoked")

	// 2. Test BrokerReadiness
	readiness, err := store.BrokerReadiness(ctx)
	if err != nil {
		t.Fatalf("BrokerReadiness failed: %v", err)
	}

	if !readiness.MasterReady {
		t.Errorf("expected MasterReady true, got false")
	}
	if readiness.TotalFollowers < 3 {
		t.Errorf("expected TotalFollowers >= 3, got %d", readiness.TotalFollowers)
	}
	if readiness.FollowersReady < 1 {
		t.Errorf("expected FollowersReady >= 1, got %d", readiness.FollowersReady)
	}
	if len(readiness.AttentionAccounts) < 2 {
		t.Errorf("expected >= 2 AttentionAccounts, got %d", len(readiness.AttentionAccounts))
	}

	var foundExpiredFollower bool
	for _, acc := range readiness.AttentionAccounts {
		if acc.BrokerAccountID == "FOL002" {
			foundExpiredFollower = true
			if acc.AuthStatus != "error" {
				t.Errorf("expected FOL002 auth status 'error', got %q", acc.AuthStatus)
			}
			if acc.AuthError == "" {
				t.Errorf("expected FOL002 auth error to be populated, got empty")
			}
		}
	}
	if !foundExpiredFollower {
		t.Errorf("expected FOL002 in AttentionAccounts, was not found")
	}

	// 3. Test OrderIssues
	// Normal complete order
	normTag := "tag_norm_" + uuid.NewString()
	normID, err := store.InsertFollowerOrder(ctx, domain.FollowerOrder{
		FollowerID:      follower1,
		IdempotencyTag:  normTag,
		IntendedQty:     50,
		LotSize:         25,
		SizingReason:    domain.ReasonOK,
		Origin:          "copy_trade",
		Tradingsymbol:   "NIFTY24OCTFUT",
		Exchange:        "NFO",
		Product:         "NRML",
		TransactionType: "BUY",
		OrderType:       "MARKET",
	})
	if err != nil {
		t.Fatalf("insert norm order: %v", err)
	}
	_ = store.UpdateFollowerOrderPlaced(ctx, normID, "brk_norm", 50)
	_, _ = store.UpdateFollowerOrderStatus(ctx, "brk_norm", "COMPLETE", 50, decimal.NewFromInt(100))

	// Issue 1: Sized below one lot
	skipTag := "tag_skip_" + uuid.NewString()
	_, err = store.InsertFollowerOrder(ctx, domain.FollowerOrder{
		FollowerID:      follower2,
		IdempotencyTag:  skipTag,
		IntendedQty:     0,
		LotSize:         25,
		SizingReason:    domain.ReasonBelowOneLot,
		Origin:          "copy_trade",
		Tradingsymbol:   "BANKNIFTY24OCTFUT",
		Exchange:        "NFO",
		Product:         "NRML",
		TransactionType: "BUY",
		OrderType:       "MARKET",
	})
	if err != nil {
		t.Fatalf("insert skip order: %v", err)
	}

	// Issue 2: Rejected by broker (RMS margin)
	rejTag := "tag_rej_" + uuid.NewString()
	rejID, err := store.InsertFollowerOrder(ctx, domain.FollowerOrder{
		FollowerID:      follower3,
		IdempotencyTag:  rejTag,
		IntendedQty:     100,
		LotSize:         25,
		SizingReason:    domain.ReasonOK,
		Origin:          "copy_trade",
		Tradingsymbol:   "RELIANCE",
		Exchange:        "NSE",
		Product:         "CNC",
		TransactionType: "BUY",
		OrderType:       "LIMIT",
	})
	if err != nil {
		t.Fatalf("insert rej order: %v", err)
	}
	_ = store.UpdateFollowerOrderFailed(ctx, rejID, "REJECTED", "RMS: Margin Insufficient")

	issues, total, err := store.ListRecentOrderIssues(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListRecentOrderIssues failed: %v", err)
	}

	if total < 2 {
		t.Fatalf("expected total >= 2, got %d", total)
	}
	if len(issues) < 2 {
		t.Fatalf("expected >= 2 order issues, got %d", len(issues))
	}

	// Test page 1 with limit 1
	p1Issues, p1Total, err := store.ListRecentOrderIssues(ctx, 1, 0)
	if err != nil || len(p1Issues) != 1 || p1Total != total {
		t.Fatalf("page 1 failed: err=%v, count=%d, total=%d (want 1, %d)", err, len(p1Issues), p1Total, total)
	}

	// Test page 2 with limit 1 offset 1
	p2Issues, p2Total, err := store.ListRecentOrderIssues(ctx, 1, 1)
	if err != nil || len(p2Issues) != 1 || p2Total != total {
		t.Fatalf("page 2 failed: err=%v, count=%d, total=%d (want 1, %d)", err, len(p2Issues), p2Total, total)
	}
	if p1Issues[0].ID == p2Issues[0].ID {
		t.Fatalf("page 1 and page 2 returned same issue ID %d", p1Issues[0].ID)
	}

	var foundSkip, foundRej bool
	for _, iss := range issues {
		if iss.Tradingsymbol == "BANKNIFTY24OCTFUT" && iss.SizingReason == "below_one_lot" {
			foundSkip = true
		}
		if iss.Tradingsymbol == "RELIANCE" && iss.TerminalStatus == "REJECTED" && iss.LastError == "RMS: Margin Insufficient" {
			foundRej = true
		}
	}
	if !foundSkip {
		t.Errorf("did not find skipped order issue in %+v", issues)
	}
	if !foundRej {
		t.Errorf("did not find rejected order issue in %+v", issues)
	}
}

func TestAdminStore_Accounts_DynamicAuthExpiration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	userID := uuid.New()
	err := store.CreateUser(ctx, userID, "dynauth@envoytrade.com", "dynauth", "Dynamic Auth User", "hash", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	userCtx := domain.ContextWithUser(ctx, &domain.User{ID: userID})

	// 1. Account with VALID future expiry
	futureExpiry := time.Now().UTC().Add(10 * time.Hour)
	acctActive := uuid.New()
	err = store.CreateAccount(userCtx, acctActive, "Active Session Account", "follower", "zerodha", "ACT001", "key1", "sec1", "")
	if err != nil {
		t.Fatalf("CreateAccount active: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, acctActive, "tok_active", &futureExpiry, "authenticated", "")

	// 2. Account with EXPIRED token (token_expires_at in past, but auth_status originally saved as 'authenticated')
	pastExpiry := time.Now().UTC().Add(-2 * time.Hour)
	acctExpired := uuid.New()
	err = store.CreateAccount(userCtx, acctExpired, "Expired Session Account", "follower", "zerodha", "EXP001", "key2", "sec2", "")
	if err != nil {
		t.Fatalf("CreateAccount expired: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, acctExpired, "tok_expired", &pastExpiry, "authenticated", "")

	// 3. Account with NULL token expiry (no expiry recorded, but saved as 'authenticated')
	acctNullExpiry := uuid.New()
	err = store.CreateAccount(userCtx, acctNullExpiry, "Null Expiry Account", "follower", "zerodha", "NUL001", "key3", "sec3", "")
	if err != nil {
		t.Fatalf("CreateAccount null expiry: %v", err)
	}
	_ = store.SetAccountAccessToken(ctx, acctNullExpiry, "tok_null", nil, "authenticated", "")

	// Query via AccountsByUserID
	userAccounts, err := store.AccountsByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("AccountsByUserID failed: %v", err)
	}

	for _, a := range userAccounts {
		switch a.ID {
		case acctActive:
			if a.AuthStatus != "authenticated" {
				t.Errorf("expected active account auth status 'authenticated', got %q", a.AuthStatus)
			}
		case acctExpired:
			if a.AuthStatus != "error" {
				t.Errorf("expected expired account auth status 'error', got %q", a.AuthStatus)
			}
			if !strings.HasPrefix(a.AuthError, "Token expired") {
				t.Errorf("expected expired account auth error starting with 'Token expired', got %q", a.AuthError)
			}
		case acctNullExpiry:
			if a.AuthStatus != "error" {
				t.Errorf("expected null expiry account auth status 'error', got %q", a.AuthStatus)
			}
			if !strings.HasPrefix(a.AuthError, "Token expired") {
				t.Errorf("expected null expiry account auth error starting with 'Token expired', got %q", a.AuthError)
			}
		}
	}

	// Query via Accounts
	queriedAccounts, err := store.Accounts(userCtx, []uuid.UUID{acctActive, acctExpired, acctNullExpiry})
	if err != nil {
		t.Fatalf("Accounts failed: %v", err)
	}
	for _, a := range queriedAccounts {
		switch a.ID {
		case acctActive:
			if a.AuthStatus != "authenticated" {
				t.Errorf("expected active account auth status 'authenticated', got %q", a.AuthStatus)
			}
		case acctExpired:
			if a.AuthStatus != "error" {
				t.Errorf("expected expired account auth status 'error', got %q", a.AuthStatus)
			}
		case acctNullExpiry:
			if a.AuthStatus != "error" {
				t.Errorf("expected null expiry account auth status 'error', got %q", a.AuthStatus)
			}
		}
	}
}
