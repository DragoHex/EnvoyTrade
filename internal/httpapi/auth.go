package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"envoytrade/internal/auth"
	"envoytrade/internal/domain"

	"github.com/google/uuid"
)

const (
	sessionCookieName = "envoytrade_session"
	sessionDuration   = 24 * time.Hour
)

var (
	indianPhoneRegex = regexp.MustCompile(`^[6-9]\d{9}$`)
	gstRegex         = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z]{1}[1-9A-Z]{1}Z[0-9A-Z]{1}$`)
)

// AuthStore declares the persistence methods needed for user and session authentication.
type AuthStore interface {
	CreateUser(ctx context.Context, id uuid.UUID, email, username, name, passwordHash, role string) error
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	UpdateUserProfile(ctx context.Context, id uuid.UUID, email, username, name, phone, address, gstNumber string) error
	UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	CreateSession(ctx context.Context, tokenHash string, userID uuid.UUID, ip, userAgent string, expiresAt time.Time) error
	GetSessionWithUser(ctx context.Context, tokenHash string) (*domain.SessionWithUser, error)
	TouchSession(ctx context.Context, tokenHash string, newExpiry time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteOtherSessions(ctx context.Context, userID uuid.UUID, tokenHash string) error
}

type userResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	GSTNumber string `json:"gstNumber"`
	Role      string `json:"role"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Username:  u.Username,
		Name:      u.Name,
		Phone:     u.Phone,
		Address:   u.Address,
		GSTNumber: u.GSTNumber,
		Role:      u.Role,
	}
}

type registerRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *handlers) postRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to secure password")
		return
	}

	userID := uuid.New()
	err = h.store.CreateUser(r.Context(), userID, req.Email, req.Username, req.Name, passwordHash, "user")
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			writeError(w, http.StatusConflict, "user with this email or username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Create session
	plainToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session")
		return
	}

	expiresAt := time.Now().Add(sessionDuration)
	if err := h.store.CreateSession(r.Context(), tokenHash, userID, r.RemoteAddr, r.UserAgent(), expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	setSessionCookie(w, r, plainToken, sessionDuration)

	createdUser := &domain.User{
		ID:       userID,
		Email:    req.Email,
		Username: req.Username,
		Name:     req.Name,
		Role:     "user",
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user":  toUserResponse(createdUser),
		"token": plainToken,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *handlers) postLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "authentication error")
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	plainToken, tokenHash, err := auth.GenerateSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session")
		return
	}

	expiresAt := time.Now().Add(sessionDuration)
	if err := h.store.CreateSession(r.Context(), tokenHash, user.ID, r.RemoteAddr, r.UserAgent(), expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	setSessionCookie(w, r, plainToken, sessionDuration)

	writeJSON(w, http.StatusOK, map[string]any{
		"user":  toUserResponse(user),
		"token": plainToken,
	})
}

func (h *handlers) postLogout(w http.ResponseWriter, r *http.Request) {
	plainToken := extractToken(r)
	if plainToken != "" {
		tokenHash := auth.HashSessionToken(plainToken)
		_ = h.store.DeleteSession(r.Context(), tokenHash)
	}

	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) getMe(w http.ResponseWriter, r *http.Request) {
	user, ok := domain.UserFromContext(r.Context())
	if !ok || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	dbUser, err := h.store.GetUserByID(r.Context(), user.ID)
	if err == nil && dbUser != nil {
		user = dbUser
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": toUserResponse(user),
	})
}

type updateProfileRequest struct {
	Email     string `json:"email"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	GSTNumber string `json:"gstNumber"`
}

func cleanPhone(phone string) string {
	var digits strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	s := digits.String()
	if strings.HasPrefix(s, "91") && len(s) == 12 {
		s = s[2:]
	} else if strings.HasPrefix(s, "0") && len(s) == 11 {
		s = s[1:]
	}
	return s
}

func (h *handlers) putUserProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := domain.UserFromContext(r.Context())
	if !ok || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)
	req.GSTNumber = strings.ToUpper(strings.TrimSpace(req.GSTNumber))

	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	normalizedPhone := ""
	if phone := strings.TrimSpace(req.Phone); phone != "" {
		digits := cleanPhone(phone)
		if !indianPhoneRegex.MatchString(digits) {
			writeError(w, http.StatusBadRequest, "invalid phone number: must be a 10-digit Indian mobile number")
			return
		}
		normalizedPhone = "+91" + digits
	}

	if req.GSTNumber != "" {
		if !gstRegex.MatchString(req.GSTNumber) {
			writeError(w, http.StatusBadRequest, "invalid GSTIN format")
			return
		}
	}

	err := h.store.UpdateUserProfile(r.Context(), user.ID, req.Email, req.Username, req.Name, normalizedPhone, req.Address, req.GSTNumber)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			writeError(w, http.StatusConflict, "user with this email or username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	updatedUser, err := h.store.GetUserByID(r.Context(), user.ID)
	if err != nil || updatedUser == nil {
		updatedUser = &domain.User{
			ID:        user.ID,
			Email:     req.Email,
			Username:  req.Username,
			Name:      req.Name,
			Phone:     normalizedPhone,
			Address:   req.Address,
			GSTNumber: req.GSTNumber,
			Role:      user.Role,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user": toUserResponse(updatedUser),
	})
}

type updatePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

func (h *handlers) postUserPassword(w http.ResponseWriter, r *http.Request) {
	user, ok := domain.UserFromContext(r.Context())
	if !ok || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req updatePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current password and new password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	dbUser, err := h.store.GetUserByID(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retrieve user")
		return
	}

	if !auth.CheckPassword(req.OldPassword, dbUser.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "incorrect current password")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.store.UpdateUserPassword(r.Context(), user.ID, newHash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	plainToken := extractToken(r)
	if plainToken != "" {
		currentTokenHash := auth.HashSessionToken(plainToken)
		_ = h.store.DeleteOtherSessions(r.Context(), user.ID, currentTokenHash)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return os.Getenv("COOKIE_SECURE") == "true"
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, duration time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(duration.Seconds()),
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func extractToken(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}
