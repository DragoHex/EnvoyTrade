package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"envoytrade/internal/auth"
	"envoytrade/internal/domain"
	"envoytrade/internal/httpapi"

	"github.com/google/uuid"
)

type memAuthStore struct {
	mu           sync.Mutex
	usersByID    map[uuid.UUID]*domain.User
	usersByEmail map[string]*domain.User
	usersByName  map[string]*domain.User
	sessions     map[string]*domain.SessionWithUser
}

func newMemAuthStore() *memAuthStore {
	return &memAuthStore{
		usersByID:    make(map[uuid.UUID]*domain.User),
		usersByEmail: make(map[string]*domain.User),
		usersByName:  make(map[string]*domain.User),
		sessions:     make(map[string]*domain.SessionWithUser),
	}
}

func (m *memAuthStore) CreateUser(ctx context.Context, id uuid.UUID, email, username, name, passwordHash, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lowerEmail := strings.ToLower(email)
	lowerUsername := strings.ToLower(username)

	if _, exists := m.usersByEmail[lowerEmail]; exists {
		return domain.ErrDuplicate
	}
	if _, exists := m.usersByName[lowerUsername]; exists {
		return domain.ErrDuplicate
	}

	u := &domain.User{
		ID:           id,
		Email:        email,
		Username:     username,
		Name:         name,
		PasswordHash: passwordHash,
		Role:         role,
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.usersByID[id] = u
	m.usersByEmail[lowerEmail] = u
	m.usersByName[lowerUsername] = u
	return nil
}

func (m *memAuthStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByEmail[strings.ToLower(email)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *memAuthStore) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByName[strings.ToLower(username)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *memAuthStore) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *memAuthStore) CreateSession(ctx context.Context, tokenHash string, userID uuid.UUID, ip, userAgent string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByID[userID]
	if !ok {
		return domain.ErrNotFound
	}
	m.sessions[tokenHash] = &domain.SessionWithUser{
		Session: domain.Session{
			ID:         int64(len(m.sessions) + 1),
			TokenHash:  tokenHash,
			UserID:     userID,
			IPAddress:  &ip,
			UserAgent:  &userAgent,
			ExpiresAt:  expiresAt,
			CreatedAt:  time.Now(),
			LastSeenAt: time.Now(),
		},
		User: *u,
	}
	return nil
}

func (m *memAuthStore) GetSessionWithUser(ctx context.Context, tokenHash string) (*domain.SessionWithUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[tokenHash]
	if !ok || s.Session.ExpiresAt.Before(time.Now()) {
		return nil, domain.ErrNotFound
	}
	return s, nil
}

func (m *memAuthStore) TouchSession(ctx context.Context, tokenHash string, newExpiry time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[tokenHash]
	if !ok {
		return domain.ErrNotFound
	}
	s.Session.LastSeenAt = time.Now()
	s.Session.ExpiresAt = newExpiry
	return nil
}

func (m *memAuthStore) DeleteSession(ctx context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, tokenHash)
	return nil
}

func (m *memAuthStore) UpdateUserProfile(ctx context.Context, id uuid.UUID, email, username, name, phone, address, gstNumber string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, ok := m.usersByID[id]
	if !ok {
		return domain.ErrNotFound
	}

	lowerEmail := strings.ToLower(email)
	lowerUsername := strings.ToLower(username)

	if existing, exists := m.usersByEmail[lowerEmail]; exists && existing.ID != id {
		return domain.ErrDuplicate
	}
	if existing, exists := m.usersByName[lowerUsername]; exists && existing.ID != id {
		return domain.ErrDuplicate
	}

	delete(m.usersByEmail, strings.ToLower(u.Email))
	delete(m.usersByName, strings.ToLower(u.Username))

	u.Email = email
	u.Username = username
	u.Name = name
	u.Phone = phone
	u.Address = address
	u.GSTNumber = gstNumber
	u.UpdatedAt = time.Now()

	m.usersByEmail[lowerEmail] = u
	m.usersByName[lowerUsername] = u
	return nil
}

func (m *memAuthStore) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, ok := m.usersByID[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.PasswordHash = passwordHash
	u.UpdatedAt = time.Now()
	return nil
}

func (m *memAuthStore) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for th, sess := range m.sessions {
		if sess.Session.UserID == userID && th != tokenHash {
			delete(m.sessions, th)
		}
	}
	return nil
}

// fullStubStore embeds stubStore and memAuthStore to satisfy httpapi.Store
type fullStubStore struct {
	*stubStore
	*memAuthStore
}

func (f *fullStubStore) BypassAuth() bool {
	return false
}

func (f *fullStubStore) CreateUser(ctx context.Context, id uuid.UUID, email, username, name, passwordHash, role string) error {
	return f.memAuthStore.CreateUser(ctx, id, email, username, name, passwordHash, role)
}

func (f *fullStubStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return f.memAuthStore.GetUserByEmail(ctx, email)
}

func (f *fullStubStore) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	return f.memAuthStore.GetUserByUsername(ctx, username)
}

func (f *fullStubStore) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return f.memAuthStore.GetUserByID(ctx, id)
}

func (f *fullStubStore) CreateSession(ctx context.Context, tokenHash string, userID uuid.UUID, ip, userAgent string, expiresAt time.Time) error {
	return f.memAuthStore.CreateSession(ctx, tokenHash, userID, ip, userAgent, expiresAt)
}

func (f *fullStubStore) GetSessionWithUser(ctx context.Context, tokenHash string) (*domain.SessionWithUser, error) {
	return f.memAuthStore.GetSessionWithUser(ctx, tokenHash)
}

func (f *fullStubStore) TouchSession(ctx context.Context, tokenHash string, newExpiry time.Time) error {
	return f.memAuthStore.TouchSession(ctx, tokenHash, newExpiry)
}

func (f *fullStubStore) DeleteSession(ctx context.Context, tokenHash string) error {
	return f.memAuthStore.DeleteSession(ctx, tokenHash)
}

func (f *fullStubStore) UpdateUserProfile(ctx context.Context, id uuid.UUID, email, username, name, phone, address, gstNumber string) error {
	return f.memAuthStore.UpdateUserProfile(ctx, id, email, username, name, phone, address, gstNumber)
}

func (f *fullStubStore) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return f.memAuthStore.UpdateUserPassword(ctx, id, passwordHash)
}

func (f *fullStubStore) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, tokenHash string) error {
	return f.memAuthStore.DeleteOtherSessions(ctx, userID, tokenHash)
}

func newFullStubStore() *fullStubStore {
	return &fullStubStore{
		stubStore:    &stubStore{},
		memAuthStore: newMemAuthStore(),
	}
}

func TestAuthRegister(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	t.Run("successful registration creates user and returns 201 with cookie", func(t *testing.T) {
		body := map[string]string{
			"email":    "trader@example.com",
			"username": "trader1",
			"password": "Password123!",
			"name":     "Alice Trader",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
		}

		cookies := rec.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == "envoytrade_session" {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatalf("expected envoytrade_session cookie to be set")
		}
		if !sessionCookie.HttpOnly {
			t.Fatalf("session cookie must be HttpOnly")
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		userMap, ok := resp["user"].(map[string]any)
		if !ok {
			t.Fatalf("expected 'user' object in response, got %v", resp)
		}
		if userMap["email"] != "trader@example.com" {
			t.Errorf("expected email 'trader@example.com', got %v", userMap["email"])
		}
		if userMap["username"] != "trader1" {
			t.Errorf("expected username 'trader1', got %v", userMap["username"])
		}
	})

	t.Run("duplicate email returns 409 Conflict", func(t *testing.T) {
		body := map[string]string{
			"email":    "trader@example.com", // already registered
			"username": "trader2",
			"password": "Password123!",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected status 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duplicate username returns 409 Conflict", func(t *testing.T) {
		body := map[string]string{
			"email":    "other@example.com",
			"username": "trader1", // already registered
			"password": "Password123!",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected status 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid or missing email returns 400 Bad Request", func(t *testing.T) {
		cases := []struct {
			name  string
			email string
		}{
			{"empty email", ""},
			{"no at sign", "notanemail"},
			{"no domain", "user@"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				body := map[string]string{
					"email":    tc.email,
					"username": "user" + uuid.NewString()[:6],
					"password": "Password123!",
				}
				jsonBytes, _ := json.Marshal(body)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(jsonBytes))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 for %s, got %d", tc.name, rec.Code)
				}
			})
		}
	})

	t.Run("short password returns 400 Bad Request", func(t *testing.T) {
		body := map[string]string{
			"email":    "valid@example.com",
			"username": "validuser",
			"password": "123", // too short
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for short password, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestAuthLogin(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	// Seed user with bcrypt password
	hash, err := auth.HashPassword("CorrectPassword123")
	if err != nil {
		t.Fatalf("hash err: %v", err)
	}
	userID := uuid.New()
	_ = store.CreateUser(context.Background(), userID, "bob@example.com", "bob", "Bob", hash, "user")

	t.Run("valid email and password returns 200 and sets session cookie", func(t *testing.T) {
		body := map[string]string{
			"email":    "bob@example.com",
			"password": "CorrectPassword123",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		cookies := rec.Result().Cookies()
		var found bool
		for _, c := range cookies {
			if c.Name == "envoytrade_session" && c.Value != "" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected envoytrade_session cookie to be returned")
		}
	})

	t.Run("wrong password returns 401 Unauthorized", func(t *testing.T) {
		body := map[string]string{
			"email":    "bob@example.com",
			"password": "WrongPassword!",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong password, got %d", rec.Code)
		}
	})

	t.Run("unknown email returns 401 Unauthorized", func(t *testing.T) {
		body := map[string]string{
			"email":    "nonexistent@example.com",
			"password": "CorrectPassword123",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for nonexistent user, got %d", rec.Code)
		}
	})

	t.Run("suspended user returns 403 Forbidden with proper message", func(t *testing.T) {
		suspendedID := uuid.New()
		_ = store.CreateUser(context.Background(), suspendedID, "suspended@example.com", "suspended", "Suspended User", hash, "user")
		store.memAuthStore.usersByEmail["suspended@example.com"].Active = false

		body := map[string]string{
			"email":    "suspended@example.com",
			"password": "CorrectPassword123",
		}
		jsonBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for suspended user, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Your account has been suspended") {
			t.Fatalf("expected suspended error message, got: %s", rec.Body.String())
		}
	})
}

func TestAuthMeAndLogout(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	hash, _ := auth.HashPassword("MySecretPass1")
	userID := uuid.New()
	_ = store.CreateUser(context.Background(), userID, "charlie@example.com", "charlie", "Charlie", hash, "user")

	plainToken, tokenHash, _ := auth.GenerateSessionToken()
	_ = store.CreateSession(context.Background(), tokenHash, userID, "127.0.0.1", "test-agent", time.Now().Add(24*time.Hour))

	t.Run("GET /auth/me with cookie returns user profile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		userMap := resp["user"].(map[string]any)
		if userMap["email"] != "charlie@example.com" {
			t.Errorf("expected charlie@example.com, got %v", userMap["email"])
		}
	})

	t.Run("GET /auth/me with Authorization Bearer header returns user profile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+plainToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /auth/me unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated /auth/me, got %d", rec.Code)
		}
	})

	t.Run("POST /auth/logout clears cookie and revokes session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on logout, got %d", rec.Code)
		}

		// Verify cookie cleared
		cookies := rec.Result().Cookies()
		var cleared bool
		for _, c := range cookies {
			if c.Name == "envoytrade_session" && c.MaxAge < 0 {
				cleared = true
				break
			}
		}
		if !cleared {
			t.Errorf("expected session cookie to have MaxAge < 0")
		}

		// Subsequent /auth/me should fail
		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req2.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec2 := httptest.NewRecorder()

		handler.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 after logout, got %d", rec2.Code)
		}
	})
}

func TestRouteProtection(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	t.Run("GET /api/v1/accounts rejected without session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated /accounts, got %d", rec.Code)
		}
	})

	t.Run("GET /api/v1/groups rejected without session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated /groups, got %d", rec.Code)
		}
	})
}

func TestSessionLifecycleAndMaxExpiry(t *testing.T) {
	store := newFullStubStore()
	handler := httpapi.NewRouter(store, &stubActionEngine{})

	// Register a test user
	regBody, _ := json.Marshal(map[string]string{
		"email":    "trader.expiry@example.com",
		"username": "trader_expiry",
		"password": "Password123!",
		"name":     "Expiry Tester",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	handler.ServeHTTP(regRec, regReq)

	if regRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 on registration, got %d", regRec.Code)
	}

	var plainToken string
	var sessionCookie *http.Cookie
	for _, c := range regRec.Result().Cookies() {
		if c.Name == "envoytrade_session" {
			sessionCookie = c
			plainToken = c.Value
			break
		}
	}
	if plainToken == "" {
		t.Fatalf("session token not found in cookies")
	}

	tokenHash := auth.HashSessionToken(plainToken)

	t.Run("registration sets 3-day initial expiry and 7-day cookie MaxAge", func(t *testing.T) {
		// Cookie MaxAge should be 7 days (604800 seconds)
		expectedCookieMaxAge := int((7 * 24 * time.Hour).Seconds())
		if sessionCookie.MaxAge != expectedCookieMaxAge {
			t.Errorf("expected cookie MaxAge %d (7 days), got %d", expectedCookieMaxAge, sessionCookie.MaxAge)
		}

		sess := store.sessions[tokenHash]
		if sess == nil {
			t.Fatalf("session not found in store")
		}

		// Initial expires_at should be ~3 days from now
		expectedExpiry := time.Now().Add(3 * 24 * time.Hour)
		diff := sess.Session.ExpiresAt.Sub(expectedExpiry)
		if diff < -5*time.Second || diff > 5*time.Second {
			t.Errorf("expected initial expires_at around %v (3 days), got %v (diff: %v)", expectedExpiry, sess.Session.ExpiresAt, diff)
		}
	})

	t.Run("requests within 1 hour touch window do not touch database", func(t *testing.T) {
		sess := store.sessions[tokenHash]
		originalLastSeen := time.Now().Add(-30 * time.Minute)
		originalExpiry := time.Now().Add(2 * 24 * time.Hour)
		sess.Session.LastSeenAt = originalLastSeen
		sess.Session.ExpiresAt = originalExpiry

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on /auth/me, got %d", rec.Code)
		}

		if !sess.Session.LastSeenAt.Equal(originalLastSeen) {
			t.Errorf("expected LastSeenAt not to change within 1h throttle window, but changed from %v to %v", originalLastSeen, sess.Session.LastSeenAt)
		}
		if !sess.Session.ExpiresAt.Equal(originalExpiry) {
			t.Errorf("expected ExpiresAt not to change within 1h throttle window, but changed from %v to %v", originalExpiry, sess.Session.ExpiresAt)
		}
	})

	t.Run("requests after 1 hour extend session by 3 days", func(t *testing.T) {
		sess := store.sessions[tokenHash]
		// Created 2 hours ago, last seen 2 hours ago
		sess.Session.CreatedAt = time.Now().Add(-2 * time.Hour)
		sess.Session.LastSeenAt = time.Now().Add(-2 * time.Hour)
		sess.Session.ExpiresAt = time.Now().Add(1 * time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on /auth/me, got %d", rec.Code)
		}

		expectedNewExpiry := time.Now().Add(3 * 24 * time.Hour)
		diff := sess.Session.ExpiresAt.Sub(expectedNewExpiry)
		if diff < -5*time.Second || diff > 5*time.Second {
			t.Errorf("expected extended expires_at around %v (3 days), got %v (diff: %v)", expectedNewExpiry, sess.Session.ExpiresAt, diff)
		}
	})

	t.Run("touch extension is capped at 7 days from CreatedAt", func(t *testing.T) {
		sess := store.sessions[tokenHash]
		// Session created 5 days ago, last seen 2 hours ago
		createdAt := time.Now().Add(-5 * 24 * time.Hour)
		sess.Session.CreatedAt = createdAt
		sess.Session.LastSeenAt = time.Now().Add(-2 * time.Hour)
		sess.Session.ExpiresAt = time.Now().Add(1 * 24 * time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 on /auth/me, got %d", rec.Code)
		}

		// A 3-day extension from now would be day 8, but max ceiling is createdAt + 7 days
		maxCeiling := createdAt.Add(7 * 24 * time.Hour)
		diff := sess.Session.ExpiresAt.Sub(maxCeiling)
		if diff < -5*time.Second || diff > 5*time.Second {
			t.Errorf("expected expires_at capped at %v (createdAt + 7 days), got %v (diff: %v)", maxCeiling, sess.Session.ExpiresAt, diff)
		}
	})

	t.Run("session past 7 days from CreatedAt is rejected with 401 even if active", func(t *testing.T) {
		sess := store.sessions[tokenHash]
		// Session created 7 days and 5 minutes ago
		sess.Session.CreatedAt = time.Now().Add(-7*24*time.Hour - 5*time.Minute)
		sess.Session.LastSeenAt = time.Now().Add(-5 * time.Minute)
		sess.Session.ExpiresAt = time.Now().Add(1 * time.Hour)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for session exceeding 7 days, got %d", rec.Code)
		}
	})

	t.Run("session idle past 3-day window is rejected with 401", func(t *testing.T) {
		sess := store.sessions[tokenHash]
		// Session created 2 days ago, but expires_at has passed
		sess.Session.CreatedAt = time.Now().Add(-2 * 24 * time.Hour)
		sess.Session.LastSeenAt = time.Now().Add(-4 * 24 * time.Hour)
		sess.Session.ExpiresAt = time.Now().Add(-1 * time.Minute)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.AddCookie(&http.Cookie{Name: "envoytrade_session", Value: plainToken})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for expired session, got %d", rec.Code)
		}
	})
}
