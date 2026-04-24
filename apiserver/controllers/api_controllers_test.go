package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"

	adminCommon "gopherbin/admin/common"
	"gopherbin/apiserver/controllers"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	pasteCommon "gopherbin/paste/common"
)

// ── mock Paster ───────────────────────────────────────────────────────────────

type mockPaster struct {
	createResult params.Paste
	createErr    error
	getResult    params.Paste
	getErr       error
	publicResult params.Paste
	publicErr    error
	listResult   params.PasteListResult
	listErr      error
	searchResult params.PasteListResult
	searchErr    error
	deleteErr    error
	privacyResult params.Paste
	privacyErr    error
	shareResult  params.TeamMember
	shareErr     error
	unshareErr   error
	sharesResult params.PasteShareListResponse
	sharesErr    error
}

func (m *mockPaster) Create(_ context.Context, _ []byte, _, _, _ string, _ *time.Time, _ bool, _ string, _ *int, _ map[string]string) (params.Paste, error) {
	return m.createResult, m.createErr
}
func (m *mockPaster) Get(_ context.Context, _ string) (params.Paste, error) {
	return m.getResult, m.getErr
}
func (m *mockPaster) GetPublicPaste(_ context.Context, _ string) (params.Paste, error) {
	return m.publicResult, m.publicErr
}
func (m *mockPaster) List(_ context.Context, _, _ int64) (params.PasteListResult, error) {
	return m.listResult, m.listErr
}
func (m *mockPaster) Search(_ context.Context, _ string, _, _ int64) (params.PasteListResult, error) {
	return m.searchResult, m.searchErr
}
func (m *mockPaster) Delete(_ context.Context, _ string) error { return m.deleteErr }
func (m *mockPaster) SetPrivacy(_ context.Context, _ string, _ bool) (params.Paste, error) {
	return m.privacyResult, m.privacyErr
}
func (m *mockPaster) ShareWithUser(_ context.Context, _, _ string) (params.TeamMember, error) {
	return m.shareResult, m.shareErr
}
func (m *mockPaster) UnshareWithUser(_ context.Context, _, _ string) error { return m.unshareErr }
func (m *mockPaster) ListShares(_ context.Context, _ string) (params.PasteShareListResponse, error) {
	return m.sharesResult, m.sharesErr
}

var _ pasteCommon.Paster = (*mockPaster)(nil)

// ── mock TeamManager ──────────────────────────────────────────────────────────

type mockTeamManager struct {
	createResult  params.Teams
	createErr     error
	deleteErr     error
	getResult     params.Teams
	getErr        error
	listResult    params.TeamListResult
	listErr       error
	addResult     params.TeamMember
	addErr        error
	membersResult []params.TeamMember
	membersErr    error
	removeErr     error
}

func (m *mockTeamManager) Create(_ context.Context, _ string) (params.Teams, error) {
	return m.createResult, m.createErr
}
func (m *mockTeamManager) Delete(_ context.Context, _ string) error { return m.deleteErr }
func (m *mockTeamManager) Get(_ context.Context, _ string) (params.Teams, error) {
	return m.getResult, m.getErr
}
func (m *mockTeamManager) List(_ context.Context, _, _ int64) (params.TeamListResult, error) {
	return m.listResult, m.listErr
}
func (m *mockTeamManager) AddMember(_ context.Context, _, _ string) (params.TeamMember, error) {
	return m.addResult, m.addErr
}
func (m *mockTeamManager) ListMembers(_ context.Context, _ string) ([]params.TeamMember, error) {
	return m.membersResult, m.membersErr
}
func (m *mockTeamManager) RemoveMember(_ context.Context, _, _ string) error { return m.removeErr }

var _ pasteCommon.TeamManager = (*mockTeamManager)(nil)

// ── mock UserManager ──────────────────────────────────────────────────────────

type mockUserManager struct {
	hasSuperUser    bool
	superUserResult params.Users
	superUserErr    error
	authCtx         context.Context
	authErr         error
	createResult    params.Users
	createErr       error
	updateResult    params.Users
	updateErr       error
	listResult      params.UserListResult
	listErr         error
	deleteErr       error
	blacklistErr    error
}

func (m *mockUserManager) HasSuperUser() bool { return m.hasSuperUser }
func (m *mockUserManager) CreateSuperUser(_ params.NewUserParams) (params.Users, error) {
	return m.superUserResult, m.superUserErr
}
func (m *mockUserManager) Authenticate(ctx context.Context, _ params.PasswordLoginParams) (context.Context, error) {
	if m.authCtx != nil {
		return m.authCtx, m.authErr
	}
	return ctx, m.authErr
}
func (m *mockUserManager) Create(_ context.Context, _ params.NewUserParams) (params.Users, error) {
	return m.createResult, m.createErr
}
func (m *mockUserManager) Update(_ context.Context, _ uint, _ params.UpdateUserPayload) (params.Users, error) {
	return m.updateResult, m.updateErr
}
func (m *mockUserManager) List(_ context.Context, _, _ int64) (params.UserListResult, error) {
	return m.listResult, m.listErr
}
func (m *mockUserManager) Delete(_ context.Context, _ uint) error  { return m.deleteErr }
func (m *mockUserManager) Enable(_ context.Context, _ uint) error  { return nil }
func (m *mockUserManager) Disable(_ context.Context, _ uint) error { return nil }
func (m *mockUserManager) Get(_ context.Context, _ uint) (params.Users, error) {
	return params.Users{}, nil
}
func (m *mockUserManager) ValidateToken(_ string) error           { return nil }
func (m *mockUserManager) BlacklistToken(_ string, _ int64) error { return m.blacklistErr }
func (m *mockUserManager) CleanTokens() error                     { return nil }

var _ adminCommon.UserManager = (*mockUserManager)(nil)

// ── helpers ───────────────────────────────────────────────────────────────────

const testSecret = "controllers-test-secret"

func newController(p pasteCommon.Paster, tm pasteCommon.TeamManager, m adminCommon.UserManager) *controllers.APIController {
	return controllers.NewAPIController(p, tm, m, config.JWTAuth{Secret: testSecret})
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

func withVars(r *http.Request, vars map[string]string) *http.Request {
	return mux.SetURLVars(r, vars)
}

func adminCtx(r *http.Request) *http.Request {
	ctx := auth.SetAdmin(auth.SetUserID(r.Context(), 1), true)
	return r.WithContext(ctx)
}

func withJWTClaim(r *http.Request) *http.Request {
	claim := auth.JWTClaims{
		UserID:  1,
		TokenID: "tok-logout",
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	ctx := auth.SetJWTClaim(r.Context(), claim)
	return r.WithContext(ctx)
}

// ── NotFoundHandler ───────────────────────────────────────────────────────────

func TestNotFoundHandler(t *testing.T) {
	c := newController(&mockPaster{}, &mockTeamManager{}, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.NotFoundHandler(rr, httptest.NewRequest(http.MethodGet, "/no-such-path", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── FirstRunHandler ───────────────────────────────────────────────────────────

func TestFirstRunHandler_AlreadyInitialized(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{hasSuperUser: true})
	rr := httptest.NewRecorder()
	c.FirstRunHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.NewUserParams{})))
	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rr.Code)
	}
}

func TestFirstRunHandler_BadJSON(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.FirstRunHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestFirstRunHandler_Success(t *testing.T) {
	created := params.Users{ID: 1, Username: "admin"}
	c := newController(nil, nil, &mockUserManager{superUserResult: created})
	rr := httptest.NewRecorder()
	c.FirstRunHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.NewUserParams{Username: "admin"})))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var got params.Users
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("want ID=1, got %d", got.ID)
	}
}

// ── LoginHandler ──────────────────────────────────────────────────────────────

func TestLoginHandler_BadJSON(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestLoginHandler_EmptyCredentials(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.PasswordLoginParams{})))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestLoginHandler_AuthError(t *testing.T) {
	mgr := &mockUserManager{authErr: gErrors.ErrUnauthorized}
	c := newController(nil, nil, mgr)
	rr := httptest.NewRecorder()
	body := jsonBody(t, params.PasswordLoginParams{Username: "u", Password: "p"})
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", body))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestLoginHandler_Success(t *testing.T) {
	user := params.Users{ID: 1, UpdatedAt: time.Now(), Enabled: true}
	authCtx := auth.PopulateContext(context.Background(), user)
	mgr := &mockUserManager{authCtx: authCtx}
	c := newController(nil, nil, mgr)
	rr := httptest.NewRecorder()
	body := jsonBody(t, params.PasswordLoginParams{Username: "u", Password: "p"})
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", body))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp params.JWTResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

// ── LogoutHandler ─────────────────────────────────────────────────────────────

func TestLogoutHandler_Success(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.LogoutHandler(rr, withJWTClaim(httptest.NewRequest(http.MethodPost, "/", nil)))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestLogoutHandler_BlacklistError(t *testing.T) {
	mgr := &mockUserManager{blacklistErr: gErrors.NewConflictError("already blacklisted")}
	c := newController(nil, nil, mgr)
	rr := httptest.NewRecorder()
	c.LogoutHandler(rr, withJWTClaim(httptest.NewRequest(http.MethodPost, "/", nil)))
	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rr.Code)
	}
}

// ── PasteViewHandler ──────────────────────────────────────────────────────────

func TestPasteViewHandler_Success(t *testing.T) {
	paste := params.Paste{PasteID: "abc", Name: "test"}
	c := newController(&mockPaster{getResult: paste}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.PasteViewHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var got params.Paste
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PasteID != "abc" {
		t.Errorf("want PasteID=abc, got %q", got.PasteID)
	}
}

func TestPasteViewHandler_NotFound(t *testing.T) {
	c := newController(&mockPaster{getErr: gErrors.NewNotFoundError("not found")}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.PasteViewHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── PasteDownloadHandler ──────────────────────────────────────────────────────

func TestPasteDownloadHandler_Success(t *testing.T) {
	paste := params.Paste{PasteID: "abc", Name: "file.txt", Data: []byte("hello")}
	c := newController(&mockPaster{getResult: paste}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.PasteDownloadHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if cd := rr.Header().Get("Content-Disposition"); cd == "" {
		t.Error("expected Content-Disposition header")
	}
}

func TestPasteDownloadHandler_NotFound(t *testing.T) {
	c := newController(&mockPaster{getErr: gErrors.NewNotFoundError("not found")}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.PasteDownloadHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── PublicPasteViewHandler ────────────────────────────────────────────────────

func TestPublicPasteViewHandler_Success(t *testing.T) {
	paste := params.Paste{PasteID: "pub1", Public: true}
	c := newController(&mockPaster{publicResult: paste}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "pub1"})
	c.PublicPasteViewHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestPublicPasteViewHandler_NotFound(t *testing.T) {
	c := newController(&mockPaster{publicErr: gErrors.NewNotFoundError("not found")}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "pub1"})
	c.PublicPasteViewHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── PasteListHandler ──────────────────────────────────────────────────────────

func TestPasteListHandler_Success(t *testing.T) {
	result := params.PasteListResult{TotalPages: 1, Pastes: []params.Paste{{PasteID: "x"}}}
	c := newController(&mockPaster{listResult: result}, nil, nil)
	rr := httptest.NewRecorder()
	c.PasteListHandler(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── SearchPasteHandler ────────────────────────────────────────────────────────

func TestSearchPasteHandler_MissingQuery(t *testing.T) {
	c := newController(&mockPaster{}, nil, nil)
	rr := httptest.NewRecorder()
	c.SearchPasteHandler(rr, httptest.NewRequest(http.MethodGet, "/search", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestSearchPasteHandler_Success(t *testing.T) {
	result := params.PasteListResult{Pastes: []params.Paste{{PasteID: "y"}}}
	c := newController(&mockPaster{searchResult: result}, nil, nil)
	rr := httptest.NewRecorder()
	c.SearchPasteHandler(rr, httptest.NewRequest(http.MethodGet, "/search?q=hello", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── DeletePasteHandler ────────────────────────────────────────────────────────

func TestDeletePasteHandler_Success(t *testing.T) {
	c := newController(&mockPaster{}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"pasteID": "abc"})
	c.DeletePasteHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestDeletePasteHandler_NotFound(t *testing.T) {
	c := newController(&mockPaster{deleteErr: gErrors.NewNotFoundError("not found")}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"pasteID": "abc"})
	c.DeletePasteHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── CreatePasteHandler ────────────────────────────────────────────────────────

func TestCreatePasteHandler_BadJSON(t *testing.T) {
	c := newController(&mockPaster{}, nil, nil)
	rr := httptest.NewRecorder()
	c.CreatePasteHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCreatePasteHandler_Success(t *testing.T) {
	created := params.Paste{PasteID: "new1", Name: "hello"}
	c := newController(&mockPaster{createResult: created}, nil, nil)
	rr := httptest.NewRecorder()
	body := jsonBody(t, params.Paste{Name: "hello", Data: []byte("content")})
	c.CreatePasteHandler(rr, httptest.NewRequest(http.MethodPost, "/", body))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var got params.Paste
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PasteID != "new1" {
		t.Errorf("want PasteID=new1, got %q", got.PasteID)
	}
}

// ── UpdatePasteHandler ────────────────────────────────────────────────────────

func TestUpdatePasteHandler_BadJSON(t *testing.T) {
	c := newController(&mockPaster{}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString("not-json")), map[string]string{"pasteID": "abc"})
	c.UpdatePasteHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdatePasteHandler_Success(t *testing.T) {
	updated := params.Paste{PasteID: "abc", Public: true}
	c := newController(&mockPaster{privacyResult: updated}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodPut, "/", jsonBody(t, params.UpdatePasteParams{Public: true})), map[string]string{"pasteID": "abc"})
	c.UpdatePasteHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── SharePasteHandler ─────────────────────────────────────────────────────────

func TestSharePasteHandler_BadJSON(t *testing.T) {
	c := newController(&mockPaster{}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")), map[string]string{"pasteID": "abc"})
	c.SharePasteHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestSharePasteHandler_Success(t *testing.T) {
	member := params.TeamMember{Username: "bob"}
	c := newController(&mockPaster{shareResult: member}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.UserActionRequest{UserID: "bob"})),
		map[string]string{"pasteID": "abc"},
	)
	c.SharePasteHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── UnsharePasteHandler ───────────────────────────────────────────────────────

func TestUnsharePasteHandler_Success(t *testing.T) {
	c := newController(&mockPaster{}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodDelete, "/", nil),
		map[string]string{"pasteID": "abc", "userID": "bob"},
	)
	c.UnsharePasteHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── ListSharesHandler ─────────────────────────────────────────────────────────

func TestListSharesHandler_Success(t *testing.T) {
	shares := params.PasteShareListResponse{Users: []params.TeamMember{{Username: "alice"}}}
	c := newController(&mockPaster{sharesResult: shares}, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.ListSharesHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── UserListHandler ───────────────────────────────────────────────────────────

func TestUserListHandler_Unauthorized(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.UserListHandler(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestUserListHandler_Success(t *testing.T) {
	result := params.UserListResult{Users: []params.Users{{ID: 1}}}
	c := newController(nil, nil, &mockUserManager{listResult: result})
	rr := httptest.NewRecorder()
	c.UserListHandler(rr, adminCtx(httptest.NewRequest(http.MethodGet, "/", nil)))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── NewUserHandler ────────────────────────────────────────────────────────────

func TestNewUserHandler_BadJSON(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	c.NewUserHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestNewUserHandler_Success(t *testing.T) {
	created := params.Users{ID: 2, Username: "bob"}
	c := newController(nil, nil, &mockUserManager{createResult: created})
	rr := httptest.NewRecorder()
	c.NewUserHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.NewUserParams{Username: "bob"})))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var got params.Users
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != 2 {
		t.Errorf("want ID=2, got %d", got.ID)
	}
}

// ── UpdateUserHandler ─────────────────────────────────────────────────────────

func TestUpdateUserHandler_InvalidID(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodPut, "/", jsonBody(t, params.UpdateUserPayload{})),
		map[string]string{"userID": "not-a-number"},
	)
	c.UpdateUserHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdateUserHandler_Success(t *testing.T) {
	updated := params.Users{ID: 1, Username: "alice"}
	c := newController(nil, nil, &mockUserManager{updateResult: updated})
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodPut, "/", jsonBody(t, params.UpdateUserPayload{})),
		map[string]string{"userID": "1"},
	)
	c.UpdateUserHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── DeleteUserHandler ─────────────────────────────────────────────────────────

func TestDeleteUserHandler_InvalidID(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodDelete, "/", nil),
		map[string]string{"userID": "not-a-number"},
	)
	c.DeleteUserHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestDeleteUserHandler_Success(t *testing.T) {
	c := newController(nil, nil, &mockUserManager{})
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodDelete, "/", nil),
		map[string]string{"userID": "1"},
	)
	c.DeleteUserHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── NewTeamHandler ────────────────────────────────────────────────────────────

func TestNewTeamHandler_BadJSON(t *testing.T) {
	c := newController(nil, &mockTeamManager{}, nil)
	rr := httptest.NewRecorder()
	c.NewTeamHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestNewTeamHandler_Success(t *testing.T) {
	team := params.Teams{Name: "dev"}
	c := newController(nil, &mockTeamManager{createResult: team}, nil)
	rr := httptest.NewRecorder()
	c.NewTeamHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.NewTeamParams{Name: "dev"})))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var got params.Teams
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "dev" {
		t.Errorf("want Name=dev, got %q", got.Name)
	}
}

// ── DeleteTeamHandler ─────────────────────────────────────────────────────────

func TestDeleteTeamHandler_Success(t *testing.T) {
	c := newController(nil, &mockTeamManager{}, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"teamName": "dev"})
	c.DeleteTeamHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── GetTeamHandler ────────────────────────────────────────────────────────────

func TestGetTeamHandler_Success(t *testing.T) {
	team := params.Teams{Name: "dev"}
	c := newController(nil, &mockTeamManager{getResult: team}, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"teamName": "dev"})
	c.GetTeamHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestGetTeamHandler_NotFound(t *testing.T) {
	c := newController(nil, &mockTeamManager{getErr: gErrors.NewNotFoundError("not found")}, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"teamName": "dev"})
	c.GetTeamHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── ListTeamsHandler ──────────────────────────────────────────────────────────

func TestListTeamsHandler_Success(t *testing.T) {
	result := params.TeamListResult{Teams: []params.Teams{{Name: "dev"}}}
	c := newController(nil, &mockTeamManager{listResult: result}, nil)
	rr := httptest.NewRecorder()
	c.ListTeamsHandler(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── AddTeamMemberHandler ──────────────────────────────────────────────────────

func TestAddTeamMemberHandler_BadJSON(t *testing.T) {
	c := newController(nil, &mockTeamManager{}, nil)
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")),
		map[string]string{"teamName": "dev"},
	)
	c.AddTeamMemberHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestAddTeamMemberHandler_Success(t *testing.T) {
	member := params.TeamMember{Username: "alice"}
	c := newController(nil, &mockTeamManager{addResult: member}, nil)
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.UserActionRequest{UserID: "alice"})),
		map[string]string{"teamName": "dev"},
	)
	c.AddTeamMemberHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── ListTeamMembersHandler ────────────────────────────────────────────────────

func TestListTeamMembersHandler_Success(t *testing.T) {
	members := []params.TeamMember{{Username: "alice"}, {Username: "bob"}}
	c := newController(nil, &mockTeamManager{membersResult: members}, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"teamName": "dev"})
	c.ListTeamMembersHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── RemoveTeamMemberHandler ───────────────────────────────────────────────────

func TestRemoveTeamMemberHandler_Success(t *testing.T) {
	c := newController(nil, &mockTeamManager{}, nil)
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodDelete, "/", nil),
		map[string]string{"teamName": "dev", "member": "alice"},
	)
	c.RemoveTeamMemberHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}
