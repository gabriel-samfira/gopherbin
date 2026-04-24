package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	adminCommon "gopherbin/admin/common"
	"gopherbin/auth"
	"gopherbin/config"
	"gopherbin/params"
)

const testSecret = "test-jwt-secret"

type mockManager struct {
	hasSuperUser bool
	user         params.Users
	getUserErr   error
	validateErr  error
}

func (m *mockManager) HasSuperUser() bool { return m.hasSuperUser }
func (m *mockManager) Get(_ context.Context, _ uint) (params.Users, error) {
	return m.user, m.getUserErr
}
func (m *mockManager) ValidateToken(_ string) error { return m.validateErr }
func (m *mockManager) Create(_ context.Context, _ params.NewUserParams) (params.Users, error) {
	return params.Users{}, nil
}
func (m *mockManager) Update(_ context.Context, _ uint, _ params.UpdateUserPayload) (params.Users, error) {
	return params.Users{}, nil
}
func (m *mockManager) List(_ context.Context, _, _ int64) (params.UserListResult, error) {
	return params.UserListResult{}, nil
}
func (m *mockManager) Delete(_ context.Context, _ uint) error  { return nil }
func (m *mockManager) Enable(_ context.Context, _ uint) error  { return nil }
func (m *mockManager) Disable(_ context.Context, _ uint) error { return nil }
func (m *mockManager) Authenticate(_ context.Context, _ params.PasswordLoginParams) (context.Context, error) {
	return context.Background(), nil
}
func (m *mockManager) CreateSuperUser(_ params.NewUserParams) (params.Users, error) {
	return params.Users{}, nil
}
func (m *mockManager) BlacklistToken(_ string, _ int64) error { return nil }
func (m *mockManager) CleanTokens() error                    { return nil }

var _ adminCommon.UserManager = (*mockManager)(nil)

func makeJWT(t *testing.T, claims auth.JWTClaims, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newJWTMiddleware(t *testing.T, mgr adminCommon.UserManager) auth.Middleware {
	t.Helper()
	mw, err := auth.NewjwtMiddleware(mgr, config.JWTAuth{Secret: testSecret})
	if err != nil {
		t.Fatalf("NewjwtMiddleware: %v", err)
	}
	return mw
}

// ── JWTClaim context helpers ──────────────────────────────────────────────────

func TestSetJWTClaim(t *testing.T) {
	claim := auth.JWTClaims{UserID: 99, TokenID: "tok-abc", IsAdmin: true}
	ctx := auth.SetJWTClaim(context.Background(), claim)
	got := auth.JWTClaim(ctx)
	if got.UserID != 99 || got.TokenID != "tok-abc" || !got.IsAdmin {
		t.Errorf("JWTClaim: unexpected value %+v", got)
	}
}

func TestJWTClaim_EmptyContext(t *testing.T) {
	got := auth.JWTClaim(context.Background())
	if got.UserID != 0 || got.TokenID != "" {
		t.Errorf("JWTClaim on empty context: want zero value, got %+v", got)
	}
}

// ── InitRequired middleware ───────────────────────────────────────────────────

func TestInitRequiredMiddleware_NoSuperUser(t *testing.T) {
	mw, err := auth.NewInitRequiredMiddleware(&mockManager{hasSuperUser: false})
	if err != nil {
		t.Fatalf("NewInitRequiredMiddleware: %v", err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	mw.Middleware(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rr.Code)
	}
}

func TestInitRequiredMiddleware_HasSuperUser(t *testing.T) {
	mw, err := auth.NewInitRequiredMiddleware(&mockManager{hasSuperUser: true})
	if err != nil {
		t.Fatalf("NewInitRequiredMiddleware: %v", err)
	}
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	rr := httptest.NewRecorder()
	mw.Middleware(next).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if !nextCalled {
		t.Error("next handler was not called")
	}
}

// ── JWT middleware ────────────────────────────────────────────────────────────

func TestJWTMiddleware_MissingAuthHeader(t *testing.T) {
	mw := newJWTMiddleware(t, &mockManager{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_MalformedHeader(t *testing.T) {
	mw := newJWTMiddleware(t, &mockManager{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "notabearer")
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_InvalidSignature(t *testing.T) {
	updatedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mgr := &mockManager{user: params.Users{ID: 1, Enabled: true, UpdatedAt: updatedAt}}
	mw := newJWTMiddleware(t, mgr)
	token := makeJWT(t, auth.JWTClaims{UserID: 1, UpdatedAt: updatedAt.String()}, "wrong-secret")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	updatedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mgr := &mockManager{user: params.Users{ID: 1, Enabled: true, UpdatedAt: updatedAt}}
	mw := newJWTMiddleware(t, mgr)
	token := makeJWT(t, auth.JWTClaims{
		UserID:    1,
		TokenID:   "tok-1",
		UpdatedAt: updatedAt.String(),
	}, testSecret)
	nextCalled := false
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if !nextCalled {
		t.Error("next handler not called")
	}
}

func TestJWTMiddleware_DisabledUser(t *testing.T) {
	updatedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mgr := &mockManager{user: params.Users{ID: 2, Enabled: false, UpdatedAt: updatedAt}}
	mw := newJWTMiddleware(t, mgr)
	token := makeJWT(t, auth.JWTClaims{
		UserID:    2,
		TokenID:   "tok-2",
		UpdatedAt: updatedAt.String(),
	}, testSecret)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_StaleUpdatedAt(t *testing.T) {
	updatedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mgr := &mockManager{user: params.Users{ID: 3, Enabled: true, UpdatedAt: updatedAt}}
	mw := newJWTMiddleware(t, mgr)
	stale := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	token := makeJWT(t, auth.JWTClaims{
		UserID:    3,
		TokenID:   "tok-3",
		UpdatedAt: stale.String(),
	}, testSecret)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_BlacklistedToken(t *testing.T) {
	updatedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mgr := &mockManager{
		user:        params.Users{ID: 4, Enabled: true, UpdatedAt: updatedAt},
		validateErr: fmt.Errorf("token is blacklisted"),
	}
	mw := newJWTMiddleware(t, mgr)
	token := makeJWT(t, auth.JWTClaims{
		UserID:    4,
		TokenID:   "tok-4",
		UpdatedAt: updatedAt.String(),
	}, testSecret)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}
