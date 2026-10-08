package httpapi_test

import (
	"context"
	"os"
	"time"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func init() {
	if os.Getenv("ENCRYPTION_KEY") == "" {
		_ = os.Setenv("ENCRYPTION_KEY", "envoytrade-test-encryption-key-32b")
	}
}

// stubStore is a hand-written fake satisfying httpapi.GroupsStore,
// httpapi.AccountsStore, and httpapi.ActionsStore — same style as
// internal/kite/callback's fakes (no mock framework).
type stubStore struct {
	groups    []domain.GroupSummary
	groupsErr error
	createGroupErr error
	updateGroupErr error

	detail       domain.GroupDetail
	detailErr    error
	groupDetails map[uuid.UUID]domain.GroupDetail

	setEnabledErr  error
	setEnabledArgs []setEnabledCall

	setActiveErr  error
	setActiveArgs []setActiveCall

	latestFill    domain.MasterFill
	latestFillErr error

	resolveMasterID    uuid.UUID
	resolveMasterIDErr error

	accounts    []domain.Account
	accountsErr error
	accountsIDs []uuid.UUID // last ids arg getAccounts was called with

	accountRoles   map[uuid.UUID]string // AccountRole returns domain.ErrNotFound if id absent
	accountRoleErr error

	createAccountErr  error
	createAccountArgs []createAccountCall

	createFollowLinkErr  error
	createFollowLinkArgs []domain.FollowLink

	updateFollowLinkTermsErr  error
	updateFollowLinkTermsArgs []updateFollowLinkTermsCall

	setStatusErr  error
	setStatusArgs []setStatusCall

	setIPAddressErr  error
	setIPAddressArgs []setIPAddressCall

	setAPIKeyErr  error
	setAPIKeyArgs []setAPIKeyCall

	setAPISecretErr  error
	setAPISecretArgs []setAPISecretCall

	setAccessTokenErr  error
	setAccessTokenArgs []setAccessTokenCall

	setEncryptedCredentialsErr  error
	setEncryptedCredentialsArgs []setEncryptedCredentialsCall

	deleteAccountErr  error
	deleteAccountArgs []uuid.UUID

	deleteFollowLinkErr  error
	deleteFollowLinkArgs []uuid.UUID

	ordersDetail      domain.AccountOrdersDetail
	ordersDetailErr   error
	ordersDetailIDs   []uuid.UUID
	ordersDetailTab   string
	ordersDetailPage  int
	ordersDetailLimit int

	proxyIPs         []domain.ProxyIP
	proxyIPsErr      error
	availableIPs     []domain.ProxyIP
	availableIPsErr  error
	proxyIPByAddr    domain.ProxyIP
	proxyIPByAddrErr error
}

type createAccountCall struct {
	ID                  uuid.UUID
	Name                string
	Role                string
	Broker              string
	BrokerUserID        string
	ApiKey              string
	ApiSecret           string
	IPAddress           string
	EncryptedPassword   string
	EncryptedTotpSecret string
}

type setEncryptedCredentialsCall struct {
	ID                  uuid.UUID
	EncryptedPassword   string
	EncryptedTotpSecret string
}

type setIPAddressCall struct {
	ID uuid.UUID
	IP string
}

type setAPIKeyCall struct {
	ID     uuid.UUID
	ApiKey string
}

type setAPISecretCall struct {
	ID        uuid.UUID
	ApiSecret string
}

type setAccessTokenCall struct {
	ID         uuid.UUID
	Token      string
	ExpiresAt  *time.Time
	AuthStatus string
	AuthError  string
}

type updateFollowLinkTermsCall struct {
	FollowerID     uuid.UUID
	CloneFactor    decimal.Decimal
	MaxQtyPerOrder *int
}

type setStatusCall struct {
	ID     uuid.UUID
	Status domain.AccountStatus
}

type setEnabledCall struct {
	FollowerID uuid.UUID
	Enabled    bool
}

type setActiveCall struct {
	AccountID uuid.UUID
	Active    bool
}

func (s *stubStore) Groups(context.Context) ([]domain.GroupSummary, error) {
	return s.groups, s.groupsErr
}

func (s *stubStore) GroupDetail(_ context.Context, id uuid.UUID) (domain.GroupDetail, error) {
	if s.detailErr != nil {
		return domain.GroupDetail{}, s.detailErr
	}
	if s.groupDetails != nil {
		if d, ok := s.groupDetails[id]; ok {
			return d, nil
		}
	}
	if s.detail.GroupID != uuid.Nil || s.detail.MasterID != uuid.Nil {
		if s.detail.GroupID == id || s.detail.MasterID == id {
			return s.detail, nil
		}
	}
	if s.accountRoles != nil && s.accountRoles[id] == "master" {
		return domain.GroupDetail{
			GroupID:         id,
			GroupName:       "Group " + id.String()[:8],
			MasterID:        id,
			MasterAccountID: "ZX1234",
			MasterActive:    true,
		}, nil
	}
	return domain.GroupDetail{}, domain.ErrNotFound
}

func (s *stubStore) CreateGroup(_ context.Context, id uuid.UUID, name string, masterID uuid.UUID) error {
	return s.createGroupErr
}

func (s *stubStore) UpdateGroup(_ context.Context, id uuid.UUID, name *string, masterID *uuid.UUID) error {
	return s.updateGroupErr
}

func (s *stubStore) DeleteGroup(_ context.Context, id uuid.UUID) error {
	return nil
}

func (s *stubStore) SetFollowLinkEnabled(_ context.Context, followerID uuid.UUID, enabled bool) error {
	s.setEnabledArgs = append(s.setEnabledArgs, setEnabledCall{followerID, enabled})
	return s.setEnabledErr
}

func (s *stubStore) SetAccountActive(_ context.Context, id uuid.UUID, active bool) error {
	s.setActiveArgs = append(s.setActiveArgs, setActiveCall{id, active})
	return s.setActiveErr
}

func (s *stubStore) LatestMasterFill(context.Context, uuid.UUID) (domain.MasterFill, error) {
	return s.latestFill, s.latestFillErr
}

func (s *stubStore) ResolveMasterID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return s.resolveMasterID, s.resolveMasterIDErr
}

func (s *stubStore) Accounts(_ context.Context, ids []uuid.UUID) ([]domain.Account, error) {
	s.accountsIDs = ids
	if s.accountsErr != nil {
		return nil, s.accountsErr
	}
	if ids == nil {
		return s.accounts, nil
	}
	byID := make(map[uuid.UUID]domain.Account, len(s.accounts))
	for _, a := range s.accounts {
		byID[a.ID] = a
	}
	filtered := make([]domain.Account, 0, len(ids))
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

func (s *stubStore) AccountRole(_ context.Context, id uuid.UUID) (string, error) {
	if s.accountRoleErr != nil {
		return "", s.accountRoleErr
	}
	role, ok := s.accountRoles[id]
	if !ok {
		return "", domain.ErrNotFound
	}
	return role, nil
}

func (s *stubStore) CreateAccount(_ context.Context, id uuid.UUID, name, role, broker, brokerUserID, apiKey, apiSecret, ipAddress string) error {
	s.createAccountArgs = append(s.createAccountArgs, createAccountCall{id, name, role, broker, brokerUserID, apiKey, apiSecret, ipAddress, "", ""})
	return s.createAccountErr
}

func (s *stubStore) CreateAccountWithCredentials(_ context.Context, id uuid.UUID, name, role, broker, brokerUserID, apiKey, apiSecret, ipAddress, encPassword, encTotpSecret string) error {
	s.createAccountArgs = append(s.createAccountArgs, createAccountCall{id, name, role, broker, brokerUserID, apiKey, apiSecret, ipAddress, encPassword, encTotpSecret})
	return s.createAccountErr
}

func (s *stubStore) SetAccountEncryptedCredentials(_ context.Context, id uuid.UUID, encPassword, encTotpSecret string) error {
	s.setEncryptedCredentialsArgs = append(s.setEncryptedCredentialsArgs, setEncryptedCredentialsCall{id, encPassword, encTotpSecret})
	return s.setEncryptedCredentialsErr
}

func (s *stubStore) SetAccountIPAddress(_ context.Context, id uuid.UUID, ip string) error {
	s.setIPAddressArgs = append(s.setIPAddressArgs, setIPAddressCall{id, ip})
	return s.setIPAddressErr
}

func (s *stubStore) SetAccountAPIKey(_ context.Context, id uuid.UUID, apiKey string) error {
	s.setAPIKeyArgs = append(s.setAPIKeyArgs, setAPIKeyCall{id, apiKey})
	return s.setAPIKeyErr
}

func (s *stubStore) SetAccountAPISecret(_ context.Context, id uuid.UUID, apiSecret string) error {
	s.setAPISecretArgs = append(s.setAPISecretArgs, setAPISecretCall{id, apiSecret})
	return s.setAPISecretErr
}

func (s *stubStore) SetAccountAccessToken(_ context.Context, id uuid.UUID, token string, expiresAt *time.Time, authStatus, authError string) error {
	s.setAccessTokenArgs = append(s.setAccessTokenArgs, setAccessTokenCall{id, token, expiresAt, authStatus, authError})
	return s.setAccessTokenErr
}

func (s *stubStore) SetAccountName(_ context.Context, id uuid.UUID, name string) error {
	return nil
}

func (s *stubStore) CreateFollowLink(_ context.Context, link domain.FollowLink) error {
	s.createFollowLinkArgs = append(s.createFollowLinkArgs, link)
	return s.createFollowLinkErr
}

func (s *stubStore) UpdateFollowLinkTerms(_ context.Context, followerID uuid.UUID, cloneFactor decimal.Decimal, maxQtyPerOrder *int) error {
	s.updateFollowLinkTermsArgs = append(s.updateFollowLinkTermsArgs, updateFollowLinkTermsCall{followerID, cloneFactor, maxQtyPerOrder})
	return s.updateFollowLinkTermsErr
}

func (s *stubStore) SetAccountStatus(_ context.Context, id uuid.UUID, status domain.AccountStatus) error {
	s.setStatusArgs = append(s.setStatusArgs, setStatusCall{id, status})
	return s.setStatusErr
}

func (s *stubStore) DeleteAccount(_ context.Context, id uuid.UUID) error {
	s.deleteAccountArgs = append(s.deleteAccountArgs, id)
	return s.deleteAccountErr
}

func (s *stubStore) DeleteFollowLink(_ context.Context, followerID uuid.UUID) error {
	s.deleteFollowLinkArgs = append(s.deleteFollowLinkArgs, followerID)
	return s.deleteFollowLinkErr
}

func (s *stubStore) AccountOrders(_ context.Context, id uuid.UUID, tab string, page int, limit int) (domain.AccountOrdersDetail, error) {
	s.ordersDetailIDs = append(s.ordersDetailIDs, id)
	s.ordersDetailTab = tab
	s.ordersDetailPage = page
	s.ordersDetailLimit = limit
	if s.ordersDetailErr != nil {
		return domain.AccountOrdersDetail{}, s.ordersDetailErr
	}
	if s.ordersDetail.AccountID == uuid.Nil {
		s.ordersDetail.AccountID = id
	}
	return s.ordersDetail, nil
}

func (s *stubStore) BypassAuth() bool {
	return true
}

func (s *stubStore) CreateUser(_ context.Context, _ uuid.UUID, _, _, _, _, _ string) error {
	return nil
}

func (s *stubStore) GetUserByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (s *stubStore) GetUserByUsername(_ context.Context, _ string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (s *stubStore) GetUserByID(_ context.Context, _ uuid.UUID) (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (s *stubStore) CreateSession(_ context.Context, _ string, _ uuid.UUID, _, _ string, _ time.Time) error {
	return nil
}

func (s *stubStore) GetSessionWithUser(_ context.Context, _ string) (*domain.SessionWithUser, error) {
	return nil, domain.ErrNotFound
}

func (s *stubStore) TouchSession(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (s *stubStore) DeleteSession(_ context.Context, _ string) error {
	return nil
}

func (s *stubStore) UpdateUserProfile(_ context.Context, _ uuid.UUID, _, _, _, _, _, _ string) error {
	return nil
}

func (s *stubStore) UpdateUserPassword(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (s *stubStore) DeleteOtherSessions(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (s *stubStore) ListProxyIPs(_ context.Context) ([]domain.ProxyIP, error) {
	if s.proxyIPsErr != nil {
		return nil, s.proxyIPsErr
	}
	return s.proxyIPs, nil
}

func (s *stubStore) AvailableProxyIPs(_ context.Context, _ string, _ *uuid.UUID) ([]domain.ProxyIP, error) {
	if s.availableIPsErr != nil {
		return nil, s.availableIPsErr
	}
	return s.availableIPs, nil
}

func (s *stubStore) ProxyIPByAddress(_ context.Context, ipAddress string) (domain.ProxyIP, error) {
	if s.proxyIPByAddrErr != nil {
		return domain.ProxyIP{}, s.proxyIPByAddrErr
	}
	if s.proxyIPByAddr.IPAddress != "" {
		return s.proxyIPByAddr, nil
	}
	for _, p := range s.proxyIPs {
		if p.IPAddress == ipAddress {
			return p, nil
		}
	}
	for _, p := range s.availableIPs {
		if p.IPAddress == ipAddress {
			return p, nil
		}
	}
	return domain.ProxyIP{IPAddress: ipAddress, Host: "dc46-mum-01.algoip.in", Port: 443}, nil
}

// stubActionEngine is a hand-written fake satisfying httpapi.Engine.
type stubActionEngine struct {
	err   error
	calls []domain.MasterFill
}

func (e *stubActionEngine) HandleMasterFill(_ context.Context, fill domain.MasterFill) error {
	e.calls = append(e.calls, fill)
	return e.err
}

type stubFollowerRegistrar struct {
	registered   []uuid.UUID
	unregistered []uuid.UUID
	registerErr  error
}

func (s *stubFollowerRegistrar) RegisterFollower(_ context.Context, followerID uuid.UUID) error {
	s.registered = append(s.registered, followerID)
	return s.registerErr
}

func (s *stubFollowerRegistrar) UnregisterFollower(followerID uuid.UUID) {
	s.unregistered = append(s.unregistered, followerID)
}
