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
	"github.com/stretchr/testify/mock"

	admMocks "gopherbin/admin/common/mocks"
	adminCommon "gopherbin/admin/common"
	"gopherbin/apiserver/controllers"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	pasteMocks "gopherbin/paste/common/mocks"
	pasteCommon "gopherbin/paste/common"
)

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
	c := newController(nil, nil, nil)
	rr := httptest.NewRecorder()
	c.NotFoundHandler(rr, httptest.NewRequest(http.MethodGet, "/no-such-path", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── FirstRunHandler ───────────────────────────────────────────────────────────

func TestFirstRunHandler_AlreadyInitialized(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	m.EXPECT().HasSuperUser().Return(true)

	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.FirstRunHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.NewUserParams{})))
	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rr.Code)
	}
}

func TestFirstRunHandler_BadJSON(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	m.EXPECT().HasSuperUser().Return(false)

	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.FirstRunHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestFirstRunHandler_Success(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	created := params.Users{ID: 1, Username: "admin"}
	m.EXPECT().HasSuperUser().Return(false)
	m.EXPECT().CreateSuperUser(mock.Anything).Return(created, nil)

	c := newController(nil, nil, m)
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
	m := admMocks.NewMockUserManager(t)
	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestLoginHandler_EmptyCredentials(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", jsonBody(t, params.PasswordLoginParams{})))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestLoginHandler_AuthError(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	m.EXPECT().Authenticate(mock.Anything, mock.Anything).Return(context.Background(), gErrors.ErrUnauthorized)

	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	body := jsonBody(t, params.PasswordLoginParams{Username: "u", Password: "p"})
	c.LoginHandler(rr, httptest.NewRequest(http.MethodPost, "/", body))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestLoginHandler_Success(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	user := params.Users{ID: 1, UpdatedAt: time.Now(), Enabled: true}
	authCtx := auth.PopulateContext(context.Background(), user)
	m.EXPECT().Authenticate(mock.Anything, mock.Anything).Return(authCtx, nil)

	c := newController(nil, nil, m)
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
	m := admMocks.NewMockUserManager(t)
	m.EXPECT().BlacklistToken(mock.Anything, mock.Anything).Return(nil)

	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.LogoutHandler(rr, withJWTClaim(httptest.NewRequest(http.MethodPost, "/", nil)))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestLogoutHandler_BlacklistError(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	m.EXPECT().BlacklistToken(mock.Anything, mock.Anything).Return(gErrors.NewConflictError("already blacklisted"))

	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.LogoutHandler(rr, withJWTClaim(httptest.NewRequest(http.MethodPost, "/", nil)))
	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rr.Code)
	}
}

// ── CreatePasteHandler ────────────────────────────────────────────────────────

func TestCreatePasteHandler_BadJSON(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	c.CreatePasteHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCreatePasteHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	created := params.Paste{PasteID: "new1", Name: "hello"}
	p.EXPECT().Create(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(created, nil)

	c := newController(p, nil, nil)
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
	if got.ViewsRemaining != nil {
		t.Errorf("want ViewsRemaining=nil, got %v", *got.ViewsRemaining)
	}
}

func TestCreatePasteHandler_WithSelfDestructLimit(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	views := 3
	created := params.Paste{PasteID: "new2", Name: "secret", ViewsRemaining: &views}
	p.EXPECT().Create(
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything,
		mock.MatchedBy(func(v *int) bool { return v != nil && *v == 3 }),
		mock.Anything,
	).Return(created, nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	body := jsonBody(t, params.Paste{Name: "secret", ViewsRemaining: &views})
	c.CreatePasteHandler(rr, httptest.NewRequest(http.MethodPost, "/", body))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var got params.Paste
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ViewsRemaining == nil || *got.ViewsRemaining != 3 {
		t.Errorf("want ViewsRemaining=3, got %v", got.ViewsRemaining)
	}
}

// ── PasteViewHandler ──────────────────────────────────────────────────────────

func TestPasteViewHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	paste := params.Paste{PasteID: "abc", Name: "test"}
	p.EXPECT().Get(mock.Anything, "abc").Return(paste, nil)

	c := newController(p, nil, nil)
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

// TestPasteViewHandler_NotFound also covers the post-self-destruct case:
// once a paste exhausts its view count it is hard-deleted and any
// subsequent GET returns 404.
func TestPasteViewHandler_NotFound(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	p.EXPECT().Get(mock.Anything, "abc").Return(params.Paste{}, gErrors.NewNotFoundError("not found"))

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.PasteViewHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── PasteDownloadHandler ──────────────────────────────────────────────────────

func TestPasteDownloadHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	paste := params.Paste{PasteID: "abc", Name: "file.txt", Data: []byte("hello")}
	p.EXPECT().Get(mock.Anything, "abc").Return(paste, nil)

	c := newController(p, nil, nil)
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
	p := pasteMocks.NewMockPaster(t)
	p.EXPECT().Get(mock.Anything, "abc").Return(params.Paste{}, gErrors.NewNotFoundError("not found"))

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.PasteDownloadHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── PublicPasteViewHandler ────────────────────────────────────────────────────

func TestPublicPasteViewHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	paste := params.Paste{PasteID: "pub1", Public: true}
	p.EXPECT().GetPublicPaste(mock.Anything, "pub1").Return(paste, nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "pub1"})
	c.PublicPasteViewHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// TestPublicPasteViewHandler_NotFound also covers the post-self-destruct case
// for public pastes.
func TestPublicPasteViewHandler_NotFound(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	p.EXPECT().GetPublicPaste(mock.Anything, "pub1").Return(params.Paste{}, gErrors.NewNotFoundError("not found"))

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "pub1"})
	c.PublicPasteViewHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── PasteListHandler ──────────────────────────────────────────────────────────

func TestPasteListHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	result := params.PasteListResult{TotalPages: 1, Pastes: []params.Paste{{PasteID: "x"}}}
	p.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).Return(result, nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	c.PasteListHandler(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── SearchPasteHandler ────────────────────────────────────────────────────────

func TestSearchPasteHandler_MissingQuery(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	c.SearchPasteHandler(rr, httptest.NewRequest(http.MethodGet, "/search", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestSearchPasteHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	result := params.PasteListResult{Pastes: []params.Paste{{PasteID: "y"}}}
	p.EXPECT().Search(mock.Anything, "hello", mock.Anything, mock.Anything).Return(result, nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	c.SearchPasteHandler(rr, httptest.NewRequest(http.MethodGet, "/search?q=hello", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── DeletePasteHandler ────────────────────────────────────────────────────────

func TestDeletePasteHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	p.EXPECT().Delete(mock.Anything, "abc").Return(nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"pasteID": "abc"})
	c.DeletePasteHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestDeletePasteHandler_NotFound(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	p.EXPECT().Delete(mock.Anything, "abc").Return(gErrors.NewNotFoundError("not found"))

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"pasteID": "abc"})
	c.DeletePasteHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── UpdatePasteHandler ────────────────────────────────────────────────────────

func TestUpdatePasteHandler_BadJSON(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString("not-json")), map[string]string{"pasteID": "abc"})
	c.UpdatePasteHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdatePasteHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	updated := params.Paste{PasteID: "abc", Public: true}
	p.EXPECT().SetPrivacy(mock.Anything, "abc", true).Return(updated, nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(
		httptest.NewRequest(http.MethodPut, "/", jsonBody(t, params.UpdatePasteParams{Public: true})),
		map[string]string{"pasteID": "abc"},
	)
	c.UpdatePasteHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── SharePasteHandler ─────────────────────────────────────────────────────────

func TestSharePasteHandler_BadJSON(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")), map[string]string{"pasteID": "abc"})
	c.SharePasteHandler(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestSharePasteHandler_Success(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	member := params.TeamMember{Username: "bob"}
	p.EXPECT().ShareWithUser(mock.Anything, "abc", "bob").Return(member, nil)

	c := newController(p, nil, nil)
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
	p := pasteMocks.NewMockPaster(t)
	p.EXPECT().UnshareWithUser(mock.Anything, "abc", "bob").Return(nil)

	c := newController(p, nil, nil)
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
	p := pasteMocks.NewMockPaster(t)
	shares := params.PasteShareListResponse{Users: []params.TeamMember{{Username: "alice"}}}
	p.EXPECT().ListShares(mock.Anything, "abc").Return(shares, nil)

	c := newController(p, nil, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"pasteID": "abc"})
	c.ListSharesHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── UserListHandler ───────────────────────────────────────────────────────────

func TestUserListHandler_Unauthorized(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.UserListHandler(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestUserListHandler_Success(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	result := params.UserListResult{Users: []params.Users{{ID: 1}}}
	m.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).Return(result, nil)

	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.UserListHandler(rr, adminCtx(httptest.NewRequest(http.MethodGet, "/", nil)))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── NewUserHandler ────────────────────────────────────────────────────────────

func TestNewUserHandler_BadJSON(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	c := newController(nil, nil, m)
	rr := httptest.NewRecorder()
	c.NewUserHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestNewUserHandler_Success(t *testing.T) {
	m := admMocks.NewMockUserManager(t)
	created := params.Users{ID: 2, Username: "bob"}
	m.EXPECT().Create(mock.Anything, mock.Anything).Return(created, nil)

	c := newController(nil, nil, m)
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
	m := admMocks.NewMockUserManager(t)
	c := newController(nil, nil, m)
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
	m := admMocks.NewMockUserManager(t)
	updated := params.Users{ID: 1, Username: "alice"}
	m.EXPECT().Update(mock.Anything, uint(1), mock.Anything).Return(updated, nil)

	c := newController(nil, nil, m)
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
	m := admMocks.NewMockUserManager(t)
	c := newController(nil, nil, m)
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
	m := admMocks.NewMockUserManager(t)
	m.EXPECT().Delete(mock.Anything, uint(1)).Return(nil)

	c := newController(nil, nil, m)
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
	tm := pasteMocks.NewMockTeamManager(t)
	c := newController(nil, tm, nil)
	rr := httptest.NewRecorder()
	c.NewTeamHandler(rr, httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-json")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestNewTeamHandler_Success(t *testing.T) {
	tm := pasteMocks.NewMockTeamManager(t)
	team := params.Teams{Name: "dev"}
	tm.EXPECT().Create(mock.Anything, "dev").Return(team, nil)

	c := newController(nil, tm, nil)
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
	tm := pasteMocks.NewMockTeamManager(t)
	tm.EXPECT().Delete(mock.Anything, "dev").Return(nil)

	c := newController(nil, tm, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodDelete, "/", nil), map[string]string{"teamName": "dev"})
	c.DeleteTeamHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── GetTeamHandler ────────────────────────────────────────────────────────────

func TestGetTeamHandler_Success(t *testing.T) {
	tm := pasteMocks.NewMockTeamManager(t)
	team := params.Teams{Name: "dev"}
	tm.EXPECT().Get(mock.Anything, "dev").Return(team, nil)

	c := newController(nil, tm, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"teamName": "dev"})
	c.GetTeamHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestGetTeamHandler_NotFound(t *testing.T) {
	tm := pasteMocks.NewMockTeamManager(t)
	tm.EXPECT().Get(mock.Anything, "dev").Return(params.Teams{}, gErrors.NewNotFoundError("not found"))

	c := newController(nil, tm, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"teamName": "dev"})
	c.GetTeamHandler(rr, r)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// ── ListTeamsHandler ──────────────────────────────────────────────────────────

func TestListTeamsHandler_Success(t *testing.T) {
	tm := pasteMocks.NewMockTeamManager(t)
	result := params.TeamListResult{Teams: []params.Teams{{Name: "dev"}}}
	tm.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).Return(result, nil)

	c := newController(nil, tm, nil)
	rr := httptest.NewRecorder()
	c.ListTeamsHandler(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── AddTeamMemberHandler ──────────────────────────────────────────────────────

func TestAddTeamMemberHandler_BadJSON(t *testing.T) {
	tm := pasteMocks.NewMockTeamManager(t)
	c := newController(nil, tm, nil)
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
	tm := pasteMocks.NewMockTeamManager(t)
	member := params.TeamMember{Username: "alice"}
	tm.EXPECT().AddMember(mock.Anything, "dev", "alice").Return(member, nil)

	c := newController(nil, tm, nil)
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
	tm := pasteMocks.NewMockTeamManager(t)
	members := []params.TeamMember{{Username: "alice"}, {Username: "bob"}}
	tm.EXPECT().ListMembers(mock.Anything, "dev").Return(members, nil)

	c := newController(nil, tm, nil)
	rr := httptest.NewRecorder()
	r := withVars(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{"teamName": "dev"})
	c.ListTeamMembersHandler(rr, r)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── RemoveTeamMemberHandler ───────────────────────────────────────────────────

func TestRemoveTeamMemberHandler_Success(t *testing.T) {
	tm := pasteMocks.NewMockTeamManager(t)
	tm.EXPECT().RemoveMember(mock.Anything, "dev", "alice").Return(nil)

	c := newController(nil, tm, nil)
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
