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
