package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/mock"

	"gopherbin/apiserver/controllers"
	adminCommon "gopherbin/admin/common"
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

// TestPasteViewHandler_NotFound covers the post-self-destruct case:
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

// TestPublicPasteViewHandler_NotFound covers the post-self-destruct case
// for public pastes: hard-deleted paste returns 404 on public endpoint.
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
