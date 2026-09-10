package httpapi_test

import (
	"context"

	"envoytrade/internal/domain"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// stubStore is a hand-written fake satisfying httpapi.GroupsStore,
// httpapi.AccountsStore, and httpapi.ActionsStore — same style as
// internal/kite/callback's fakes (no mock framework).
type stubStore struct {
	groups    []domain.GroupSummary
	groupsErr error

	detail    domain.GroupDetail
	detailErr error

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

	deleteAccountErr  error
	deleteAccountArgs []uuid.UUID

	deleteFollowLinkErr  error
	deleteFollowLinkArgs []uuid.UUID
}

type createAccountCall struct {
	ID           uuid.UUID
	Name         string
	Role         string
	Broker       string
	BrokerUserID string
	ApiSecret    string
}

type updateFollowLinkTermsCall struct {
	FollowerID     uuid.UUID
	CapitalRatio   decimal.Decimal
	MaxQtyPerOrder *int
}

type setStatusCall struct {
	ID     uuid.UUID
	Status string
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
	if s.detail.GroupID == uuid.Nil && s.detail.MasterID == uuid.Nil {
		return domain.GroupDetail{
			GroupID:         id,
			GroupName:       "Group " + id.String()[:8],
			MasterID:        id,
			MasterAccountID: "ZX1234",
			MasterActive:    true,
		}, nil
	}
	return s.detail, nil
}

func (s *stubStore) CreateGroup(_ context.Context, id uuid.UUID, name string, masterID uuid.UUID) error {
	return nil
}

func (s *stubStore) UpdateGroup(_ context.Context, id uuid.UUID, name *string, masterID *uuid.UUID) error {
	return nil
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

func (s *stubStore) CreateAccount(_ context.Context, id uuid.UUID, name, role, broker, brokerUserID, apiSecret string) error {
	s.createAccountArgs = append(s.createAccountArgs, createAccountCall{id, name, role, broker, brokerUserID, apiSecret})
	return s.createAccountErr
}

func (s *stubStore) SetAccountName(_ context.Context, id uuid.UUID, name string) error {
	return nil
}

func (s *stubStore) CreateFollowLink(_ context.Context, link domain.FollowLink) error {
	s.createFollowLinkArgs = append(s.createFollowLinkArgs, link)
	return s.createFollowLinkErr
}

func (s *stubStore) UpdateFollowLinkTerms(_ context.Context, followerID uuid.UUID, capitalRatio decimal.Decimal, maxQtyPerOrder *int) error {
	s.updateFollowLinkTermsArgs = append(s.updateFollowLinkTermsArgs, updateFollowLinkTermsCall{followerID, capitalRatio, maxQtyPerOrder})
	return s.updateFollowLinkTermsErr
}

func (s *stubStore) SetAccountStatus(_ context.Context, id uuid.UUID, status string) error {
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

// stubActionEngine is a hand-written fake satisfying httpapi.Engine.
type stubActionEngine struct {
	err   error
	calls []domain.MasterFill
}

func (e *stubActionEngine) HandleMasterFill(_ context.Context, fill domain.MasterFill) error {
	e.calls = append(e.calls, fill)
	return e.err
}
