package controllers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"

	adminMocks "gopherbin/admin/common/mocks"
	"gopherbin/apiserver/controllers"
	"gopherbin/config"
	"gopherbin/params"
	pasteMocks "gopherbin/paste/common/mocks"
)

func newController(p *pasteMocks.MockPaster, tm *pasteMocks.MockTeamManager, m *adminMocks.MockUserManager) *controllers.APIController {
	return controllers.NewAPIController(p, tm, m, config.JWTAuth{Secret: "test-secret"})
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

func TestCreatePasteHandler_WithSelfDestructLimit(t *testing.T) {
	p := pasteMocks.NewMockPaster(t)
	views := 3
	created := params.Paste{PasteID: "new1", Name: "secret", ViewsRemaining: &views}
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
