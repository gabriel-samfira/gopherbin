package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopherbin/admin/common"
	"gopherbin/apiserver/responses"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/params"
)

// fakeClock is the injectable clock the limiter tests drive.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestLoginRateLimiterBlocksAfterMaxFailures(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	l := newLoginRateLimiter(clock.now)
	key := loginKey{ip: "10.0.0.1", username: "alice"}

	for i := 0; i < loginMaxFailures; i++ {
		if !l.allow(key) {
			t.Fatalf("attempt %d: want allowed, got blocked", i+1)
		}
		l.recordFailure(key)
	}
	if l.allow(key) {
		t.Fatalf("after %d failures the key must be blocked", loginMaxFailures)
	}
	if got := l.retryAfter(key); got != 900 {
		t.Fatalf("retryAfter at window start = %d, want 900", got)
	}
	// A blocked bucket must report 0 only once under limit again.
	clock.advance(loginWindowLength)
	if l.retryAfter(key) != 0 {
		t.Fatalf("retryAfter after full window expiry must be 0")
	}
}

func TestLoginRateLimiterWindowExpiry(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	l := newLoginRateLimiter(clock.now)
	key := loginKey{ip: "10.0.0.2", username: "bob"}

	// Ten failures one second apart: the oldest expires first.
	for i := 0; i < loginMaxFailures; i++ {
		l.recordFailure(key)
		clock.advance(time.Second)
	}
	// now = t0+10s; oldest failure (t0) expires at t0+15m.
	if l.allow(key) {
		t.Fatal("must still be blocked before the oldest failure expires")
	}
	want := int(loginWindowLength.Seconds()) - 10 // 890s until t0+15m
	if got := l.retryAfter(key); got != want {
		t.Fatalf("retryAfter = %d, want %d", got, want)
	}

	// Just before the oldest expires: still blocked.
	clock.advance(loginWindowLength - 10*time.Second - time.Millisecond)
	if l.allow(key) {
		t.Fatal("must still be blocked 1ms before window expiry")
	}

	// Exactly at t0+15m the oldest failure falls out of the sliding window.
	clock.advance(time.Millisecond)
	if !l.allow(key) {
		t.Fatal("must be allowed once the oldest failure leaves the window")
	}
}

func TestLoginRateLimiterKeyIsolation(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	l := newLoginRateLimiter(clock.now)
	blocked := loginKey{ip: "10.0.0.1", username: "alice"}
	for i := 0; i < loginMaxFailures; i++ {
		l.recordFailure(blocked)
	}
	if l.allow(blocked) {
		t.Fatal("blocked key must be blocked")
	}
	otherIP := loginKey{ip: "10.0.0.2", username: "alice"}
	otherUser := loginKey{ip: "10.0.0.1", username: "bob"}
	if !l.allow(otherIP) {
		t.Fatal("different IP must not share the budget")
	}
	if !l.allow(otherUser) {
		t.Fatal("different username must not share the budget")
	}
}

func TestLoginAttemptKeyLowercasesAndSplitsIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "203.0.113.7:55555"
	got := loginAttemptKey(r, "MiXeDcAsE")
	want := loginKey{ip: "203.0.113.7", username: "mixedcase"}
	if got != want {
		t.Fatalf("loginAttemptKey = %+v, want %+v", got, want)
	}
	// Same username in different case must map to the same bucket.
	r2 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r2.RemoteAddr = "203.0.113.7:40000"
	if loginAttemptKey(r2, "mixedcase") != loginAttemptKey(r, "MIXEDCASE") {
		t.Fatal("case-insensitive key expected")
	}
}

func TestClientIPIgnoresProxyHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "198.51.100.4:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.Header.Set("X-Real-IP", "203.0.113.9")
	if got := clientIP(r); got != "198.51.100.4" {
		t.Fatalf("clientIP = %q, want the peer address 198.51.100.4", got)
	}
	// No port (or bare IPv6 fallback): returned unchanged.
	r2 := &http.Request{RemoteAddr: "barehost"}
	if got := clientIP(r2); got != "barehost" {
		t.Fatalf("clientIP = %q, want barehost", got)
	}
}

func TestLoginRateLimiterSuccessClears(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	l := newLoginRateLimiter(clock.now)
	key := loginKey{ip: "10.0.0.3", username: "carol"}

	for i := 0; i < loginMaxFailures-1; i++ {
		l.recordFailure(key)
	}
	l.recordSuccess(key)
	// A success resets the bucket: a full fresh budget must be available.
	for i := 0; i < loginMaxFailures; i++ {
		if !l.allow(key) {
			t.Fatalf("success must clear prior failures (attempt %d)", i+1)
		}
		l.recordFailure(key)
	}
	if l.allow(key) {
		t.Fatal("bucket must block again after a fresh set of failures")
	}
}

func TestLoginRateLimiterLazyEviction(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	l := newLoginRateLimiter(clock.now)

	// Grow the map past the eviction threshold with live (unexpired) keys.
	for i := 0; i <= loginEvictThreshold; i++ {
		l.recordFailure(loginKey{ip: fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256), username: "u"})
	}
	if len(l.failures) <= loginEvictThreshold {
		t.Fatalf("map size %d should exceed threshold %d", len(l.failures), loginEvictThreshold)
	}
	// Sweep ran but found nothing expired: nothing dropped.
	if len(l.failures) != loginEvictThreshold+1 {
		t.Fatalf("live keys must survive eviction, got %d", len(l.failures))
	}

	// Once every window expired, the next insert sweeps all stale keys away.
	clock.advance(loginWindowLength)
	l.recordFailure(loginKey{ip: "192.0.2.1", username: "fresh"})
	if len(l.failures) != 1 {
		t.Fatalf("eviction must drop expired keys, map size = %d, want 1", len(l.failures))
	}
	if _, ok := l.failures[loginKey{ip: "192.0.2.1", username: "fresh"}]; !ok {
		t.Fatal("the freshly recorded key must survive eviction")
	}
}

// --- LoginHandler wiring ---------------------------------------------------

// stubUserManager implements admin/common.UserManager for the login tests.
// Only Authenticate is functional; everything else fails loudly.
type stubUserManager struct {
	authCalls int
	// fail makes Authenticate reject with the same error the real manager
	// returns for bad credentials.
	fail   bool
	userID uint
}

var _ common.UserManager = (*stubUserManager)(nil)

func (s *stubUserManager) Authenticate(ctx context.Context, _ params.PasswordLoginParams) (context.Context, error) {
	s.authCalls++
	if s.fail {
		return ctx, gErrors.NewUnauthorizedError("invalid username or password")
	}
	return ctx, nil
}

func (s *stubUserManager) Create(context.Context, params.NewUserParams) (params.Users, error) {
	return params.Users{}, gErrors.ErrBadRequest
}
func (s *stubUserManager) Get(context.Context, uint) (params.Users, error) {
	return params.Users{}, gErrors.ErrBadRequest
}
func (s *stubUserManager) Update(context.Context, uint, params.UpdateUserPayload) (params.Users, error) {
	return params.Users{}, gErrors.ErrBadRequest
}
func (s *stubUserManager) List(context.Context, int64, int64) (params.UserListResult, error) {
	return params.UserListResult{}, gErrors.ErrBadRequest
}
func (s *stubUserManager) Delete(context.Context, uint) error  { return gErrors.ErrBadRequest }
func (s *stubUserManager) Enable(context.Context, uint) error  { return gErrors.ErrBadRequest }
func (s *stubUserManager) Disable(context.Context, uint) error { return gErrors.ErrBadRequest }
func (s *stubUserManager) SearchUsers(context.Context, string, string) ([]params.UserSearchResult, error) {
	return nil, gErrors.ErrBadRequest
}
func (s *stubUserManager) HasSuperUser() bool { return true }
func (s *stubUserManager) CreateSuperUser(params.NewUserParams) (params.Users, error) {
	return params.Users{}, gErrors.ErrBadRequest
}
func (s *stubUserManager) ValidateToken(string) error         { return gErrors.ErrBadRequest }
func (s *stubUserManager) BlacklistToken(string, int64) error { return gErrors.ErrBadRequest }
func (s *stubUserManager) CleanTokens() error                 { return gErrors.ErrBadRequest }

func newLoginTestController(mgr *stubUserManager, clock *fakeClock) *APIController {
	c := NewAPIController(nil, nil, mgr, config.JWTAuth{Secret: "unit-test-secret", TimeToLive: "1h"})
	c.loginLimiter = newLoginRateLimiter(clock.now)
	return c
}

func postLogin(t *testing.T, c *APIController, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	r.RemoteAddr = "203.0.113.7:44444"
	w := httptest.NewRecorder()
	c.LoginHandler(w, r)
	return w
}

func TestLoginHandlerRateLimitsFailuresWithoutCallingManager(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	mgr := &stubUserManager{fail: true}
	c := newLoginTestController(mgr, clock)

	var firstBody string
	for i := 0; i < loginMaxFailures; i++ {
		w := postLogin(t, c, "victim", "wrong")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: status = %d, want 401", i+1, w.Code)
		}
		if i == 0 {
			firstBody = w.Body.String()
		} else if w.Body.String() != firstBody {
			t.Fatalf("failure %d: body drifted: %q vs %q", i+1, w.Body.String(), firstBody)
		}
	}
	if mgr.authCalls != loginMaxFailures {
		t.Fatalf("manager called %d times, want %d", mgr.authCalls, loginMaxFailures)
	}

	// Limit exceeded: identical body, Retry-After set, manager untouched.
	w := postLogin(t, c, "victim", "wrong")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("rate-limited status = %d, want 401", w.Code)
	}
	if w.Body.String() != firstBody {
		t.Fatalf("rate-limited body %q must equal bad-credentials body %q", w.Body.String(), firstBody)
	}
	var apiErr responses.APIErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("rate-limited body is not valid JSON: %q", w.Body.String())
	}
	if apiErr.Error != "Not Authorized" || apiErr.Details != "invalid username or password" {
		t.Fatalf("rate-limited body = %+v, want uniform bad-credentials shape", apiErr)
	}
	ra := w.Header().Get("Retry-After")
	if secs, err := strconv.Atoi(ra); err != nil || secs < 1 || secs > 900 {
		t.Fatalf("Retry-After = %q, want 1..900", ra)
	}
	if mgr.authCalls != loginMaxFailures {
		t.Fatalf("rate-limited attempt reached the manager (%d calls)", mgr.authCalls)
	}

	// Different username on the same IP keeps its own budget.
	if w := postLogin(t, c, "someoneelse", "wrong"); w.Code != http.StatusUnauthorized {
		t.Fatalf("other username status = %d, want 401 (and not rate-limited early)", w.Code)
	}
	if mgr.authCalls != loginMaxFailures+1 {
		t.Fatalf("manager calls = %d, want %d", mgr.authCalls, loginMaxFailures+1)
	}

	// After the window the victim can try again.
	clock.advance(loginWindowLength)
	_ = postLogin(t, c, "victim", "wrong")
	if mgr.authCalls != loginMaxFailures+2 {
		t.Fatalf("after window expiry manager calls = %d, want %d", mgr.authCalls, loginMaxFailures+2)
	}
}

func TestLoginHandlerSuccessClearsFailuresAndIssuesToken(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1700000000, 0)}
	mgr := &stubUserManager{fail: true}
	c := newLoginTestController(mgr, clock)

	// Stay under the limit here: the limiter is fail-closed by design, so a
	// login attempted while fully blocked is rejected before the manager
	// sees it (uniform 401) even if the credentials have become valid.
	for i := 0; i < loginMaxFailures/2; i++ {
		postLogin(t, c, "victim", "wrong")
	}

	// Credentials become correct: the manager is consulted and the bucket resets.
	mgr.fail = false
	w := postLogin(t, c, "victim", "right")
	if w.Code != http.StatusOK {
		t.Fatalf("success status = %d: %s", w.Code, w.Body.String())
	}
	var resp params.JWTResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Token == "" {
		t.Fatalf("expected token in body, got %q (%v)", w.Body.String(), err)
	}
	if n := len(c.loginLimiter.failures); n != 0 {
		t.Fatalf("success must clear the bucket, map still holds %d keys", n)
	}

	// A full fresh failure budget must now be available: the limiter may
	// only engage on attempt loginMaxFailures+1, proving the half-used
	// budget was cleared by the success.
	mgr.fail = true
	blockedAt := -1
	for i := 0; i <= loginMaxFailures; i++ {
		before := mgr.authCalls
		postLogin(t, c, "victim", "wrong")
		if mgr.authCalls == before {
			blockedAt = i
			break
		}
	}
	if blockedAt != loginMaxFailures {
		t.Fatalf("limiter engaged at attempt %d, want %d (stale failures survived the success)",
			blockedAt, loginMaxFailures)
	}
}
