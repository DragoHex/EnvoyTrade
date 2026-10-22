package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"envoytrade/internal/auth"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

// AdminUserStore defines persistence methods required for admin user and session management.
type AdminUserStore interface {
	ListUsers(ctx context.Context, limit, offset int) ([]domain.UserSummary, int, error)
	CreateUser(ctx context.Context, id uuid.UUID, email, username, name, passwordHash, role string) error
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateUserRoleAndStatus(ctx context.Context, userID uuid.UUID, role string, active *bool) error
	ListUserSessions(ctx context.Context, userID uuid.UUID) ([]domain.Session, error)
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) (int64, error)
	AccountsByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Account, error)
}

type adminUsersResponse struct {
	Users  []domain.UserSummary `json:"users"`
	Total  int                  `json:"total"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

type createAdminUserRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type patchAdminUserRequest struct {
	Role   *string `json:"role"`
	Active *bool   `json:"active"`
}

// getAdminUsers handles GET /api/v1/admin/users.
func (h *handlers) getAdminUsers(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 50 {
		limit = 50
	}

	offset := 0
	if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
		if parsed, err := strconv.Atoi(rawOffset); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	users, total, err := h.store.ListUsers(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminUsersResponse{
		Users:  users,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// postAdminUser handles POST /api/v1/admin/users.
func (h *handlers) postAdminUser(w http.ResponseWriter, r *http.Request) {
	var req createAdminUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	username := strings.ToLower(strings.TrimSpace(req.Username))
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = username
	}

	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = domain.RoleUser
	}
	if role != domain.RoleAdmin && role != domain.RoleUser {
		writeError(w, http.StatusBadRequest, "role must be either 'admin' or 'user'")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	userID := uuid.New()
	if err := h.store.CreateUser(r.Context(), userID, email, username, name, hash, role); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			writeError(w, http.StatusConflict, "email or username already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"userId": userID.String(),
	})
}

// patchAdminUser handles PATCH /api/v1/admin/users/{id}.
func (h *handlers) patchAdminUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req patchAdminUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var roleVal string
	if req.Role != nil {
		roleVal = strings.ToLower(strings.TrimSpace(*req.Role))
		if roleVal != domain.RoleAdmin && roleVal != domain.RoleUser {
			writeError(w, http.StatusBadRequest, "role must be either 'admin' or 'user'")
			return
		}
	}

	targetUser, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to inspect user: "+err.Error())
		return
	}

	if targetUser.Role == domain.RoleAdmin && req.Active != nil && !*req.Active {
		writeError(w, http.StatusBadRequest, "admin accounts cannot be suspended")
		return
	}

	if err := h.store.UpdateUserRoleAndStatus(r.Context(), userID, roleVal, req.Active); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update user: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getAdminUserSessions handles GET /api/v1/admin/users/{id}/sessions.
func (h *handlers) getAdminUserSessions(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	sessions, err := h.store.ListUserSessions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list user sessions: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
	})
}

// deleteAdminUserSessions handles DELETE /api/v1/admin/users/{id}/sessions.
func (h *handlers) deleteAdminUserSessions(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	count, err := h.store.RevokeAllUserSessions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke sessions: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"revokedCount": count,
	})
}

// getAdminUserAccounts handles GET /api/v1/admin/users/{id}/accounts.
func (h *handlers) getAdminUserAccounts(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get user: "+err.Error())
		return
	}

	accounts, err := h.store.AccountsByUserID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list user accounts: "+err.Error())
		return
	}

	resp := make([]accountResponse, len(accounts))
	for i, a := range accounts {
		resp[i] = toAccountResponse(a)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":     toUserResponse(user),
		"accounts": resp,
	})
}
