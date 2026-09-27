package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	adminCommon "gopherbin/admin/common"
	"gopherbin/apiserver/responses"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// NewAPIController returns a new APIController
func NewAPIController(paster common.Paster, teamManager common.TeamManager, mgr adminCommon.UserManager, cfg config.JWTAuth) *APIController {
	if _, ok := paster.(accessBudgetPeeker); !ok {
		// Loud rather than silent: without the peek capability the
		// X-Consume-Access drive-by gate cannot tell budget pastes from
		// unlimited ones and stays inactive (pre-gate behavior).
		log.Printf("apiserver: warning: paster does not implement PeekMaxAccesses; " +
			"the X-Consume-Access gate for limited-access pastes is INACTIVE")
	}
	return &APIController{
		paster:       paster,
		manager:      mgr,
		teamManager:  teamManager,
		cfg:          cfg,
		loginLimiter: newLoginRateLimiter(nil),
	}
}

// APIController implements handlers for the REST API
type APIController struct {
	paster      common.Paster
	manager     adminCommon.UserManager
	teamManager common.TeamManager
	cfg         config.JWTAuth
	// loginLimiter throttles failed login attempts per (clientIP, username)
	// to blunt password brute-forcing. See ratelimit.go.
	loginLimiter *loginRateLimiter
}

// decodeJSONError maps a JSON body decode failure to the client-facing error.
// A body truncated by the transport-level size cap surfaces as an
// http.MaxBytesError, which handleError renders as 413; anything else is a
// plain malformed-request 400.
func decodeJSONError(err error) error {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return err
	}
	return gErrors.ErrBadRequest
}

func handleError(w http.ResponseWriter, err error) {
	w.Header().Add("Content-Type", "application/json")
	origErr := errors.Cause(err)
	apiErr := responses.APIErrorResponse{
		Details: origErr.Error(),
	}

	switch origErr.(type) {
	case *gErrors.NotFoundError:
		w.WriteHeader(http.StatusNotFound)
		apiErr.Error = "Not Found"
	case *gErrors.UnauthorizedError:
		w.WriteHeader(http.StatusUnauthorized)
		apiErr.Error = "Not Authorized"
	case *gErrors.ForbiddenError:
		w.WriteHeader(http.StatusForbidden)
		apiErr.Error = "Forbidden"
	case *gErrors.BadRequestError:
		w.WriteHeader(http.StatusBadRequest)
		apiErr.Error = "Bad Request"
	case *gErrors.DuplicateUserError, *gErrors.ConflictError:
		w.WriteHeader(http.StatusConflict)
		apiErr.Error = "Conflict"
	default:
		// Transport-level body-size violations: the transport wraps request
		// bodies in http.MaxBytesReader, whose errors surface as
		// *http.MaxBytesError. They are client errors, mapped to 413 with a
		// fixed message so no internal detail reaches the client. Checked
		// against the original (possibly wrapped) err, not the Cause.
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			apiErr.Error = "Payload Too Large"
			apiErr.Details = "request body exceeds the server limit"
			break
		}
		// Anything else is an unhandled internal error (SQL text, FTS
		// syntax errors, driver output, ...). It must be logged server-side
		// and never echoed to clients.
		log.Printf("apiserver: unhandled internal error: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		apiErr.Error = "Server error"
		apiErr.Details = "an internal error occurred"
	}

	json.NewEncoder(w).Encode(apiErr)
}

// parseScope normalizes the scope query parameter to one of the values
// supported by the Paster interface, defaulting to ScopeAll.
func parseScope(scope string) string {
	switch scope {
	case common.ScopeMine, common.ScopeShared:
		return scope
	default:
		return common.ScopeAll
	}
}

// Default and maximum values accepted for the max_results pagination
// parameter.
const (
	defaultMaxResults = 50
	maxMaxResults     = 100
)

// clampPagination normalizes the page and max_results query parameters before
// they are handed to the manager layers. Page is clamped to >= 1 (a negative
// page previously produced a negative OFFSET and a bogus total_pages).
// maxResults is clamped to 1..100 because values such as -1 make GORM drop
// the LIMIT clause entirely and return whole tables. A zero (absent or
// unparsable) maxResults keeps the previous handler default of 50.
func clampPagination(page, maxResults int64) (int64, int64) {
	if page < 1 {
		page = 1
	}
	if maxResults == 0 {
		maxResults = defaultMaxResults
	}
	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > maxMaxResults {
		maxResults = maxMaxResults
	}
	return page, maxResults
}

// maxDownloadNameRunes caps the paste name as it appears in download headers.
const maxDownloadNameRunes = 200

// sanitizeDownloadName makes a paste name safe to place in response headers:
// it strips the characters that would break out of an RFC 6266 quoted-string
// (double quote, backslash) and all control and format characters (CR, LF,
// DEL, C1 controls, zero-width and similar non-ASCII Cf characters), then
// truncates on a rune boundary at 200 runes, replacing the tail with an
// ellipsis. An empty result falls back to a generic name.
func sanitizeDownloadName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r == '"' || r == '\\' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	clean := []rune(strings.TrimSpace(b.String()))
	if len(clean) > maxDownloadNameRunes {
		clean = append(clean[:maxDownloadNameRunes-1], '…')
	}
	if len(clean) == 0 {
		return "paste"
	}
	return string(clean)
}

// rfc5987Rest escapes the few characters url.PathEscape leaves alone that are
// not valid RFC 5987 attr-chars in an extended parameter value.
var rfc5987Rest = strings.NewReplacer(
	"(", "%28", ")", "%29", ",", "%2C", ":", "%3A", ";", "%3B", "?", "%3F", "@", "%40",
)

// contentDisposition builds an RFC 6266 Content-Disposition value for a paste
// download. The sanitized name is quoted for the legacy filename parameter,
// and an ASCII-only RFC 5987 filename* parameter (percent-encoded UTF-8) is
// appended so Unicode names survive intact.
func contentDisposition(name string) string {
	safe := sanitizeDownloadName(name)
	encoded := rfc5987Rest.Replace(url.PathEscape(safe))
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, safe, encoded)
}

// NotFoundHandler is returned when an invalid URL is acccessed
func (p *APIController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(responses.NotFoundResponse)
}

// ConsumeAccessHeader must be sent (any non-empty value) on the paste-read
// routes whose read consumes one of a paste's limited accesses
// (max_accesses set). Browsers cannot attach custom headers to drive-by
// requests (an <img> src cannot set them at all; a cross-origin fetch that
// tries one is blocked by the CORS preflight unless its origin is one of
// the configured cors_origins), so its presence is a positive confirmation
// that a real viewer asked for the content, not a page burning pastes from
// a third-party origin. The web UI first reads without it and repeats the
// read with it once the user confirmed the view (see
// webui/svelte-app/src/lib/api/pastes.ts).
const ConsumeAccessHeader = "X-Consume-Access"

// accessBudgetPeeker is an OPTIONAL read-only capability a common.Paster
// implementation may provide to report a paste's access budget without
// consuming one of its accesses. The gate below needs the budget BEFORE
// calling Get/GetPublicPaste, which consume unconditionally on budget
// pastes, so the check cannot go through the ordinary getters; only a
// side-effect-free read (loadPaste-style) can. Get and GetPublicPaste
// remain unchanged. When the wired paster does not implement this
// capability the gate cannot distinguish budget pastes from unlimited
// ones and keeps the pre-gate behavior (NewAPIController logs a warning
// at startup so the gap is visible rather than silent).
type accessBudgetPeeker interface {
	PeekMaxAccesses(ctx context.Context, pasteID string, publicOnly bool) (*int, error)
}

// confirmLimitedAccess guards the read handlers that consume an access
// (GET /paste/{id}, GET /public/paste/{id}, GET /paste/{id}/download).
// It returns true when the consuming getter call may proceed: the client
// confirmed the view with the X-Consume-Access header, or the paste has no
// access budget (a view of such a paste cannot consume anything, so it
// behaves exactly as before this gate existed). A budget paste read without
// the header is answered with a typed 403 and no manager call is made, so
// nothing is consumed and nothing is destroyed.
// publicOnly selects the anonymous public-paste semantics for the peek.
func (p *APIController) confirmLimitedAccess(w http.ResponseWriter, r *http.Request, pasteID string, publicOnly bool) bool {
	// Any non-empty value counts: the header's presence is the
	// confirmation, its content carries no meaning.
	if r.Header.Get(ConsumeAccessHeader) != "" {
		return true
	}
	peeker, ok := p.paster.(accessBudgetPeeker)
	if !ok {
		// Capability absent: the budget cannot be inspected without
		// consuming an access to find out; keep serving as before.
		return true
	}
	maxAccesses, err := peeker.PeekMaxAccesses(r.Context(), pasteID, publicOnly)
	if err != nil || maxAccesses == nil {
		// Unknown, unreadable or not found: proceed and let the getter
		// produce exactly the same not-found / internal responses it
		// produced before this gate existed.
		return true
	}
	handleError(w, gErrors.NewForbiddenError(
		"viewing this paste consumes one of its limited accesses; send header %s to confirm the view", ConsumeAccessHeader))
	return false
}

// FirstRunHandler initializez gopherbin
func (p *APIController) FirstRunHandler(w http.ResponseWriter, r *http.Request) {
	if p.manager.HasSuperUser() {
		err := gErrors.NewConflictError("already initialized")
		handleError(w, err)
		return
	}

	var newUserParams params.NewUserParams
	if err := json.NewDecoder(r.Body).Decode(&newUserParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	newUser, err := p.manager.CreateSuperUser(newUserParams)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newUser)
}

// LoginHandler returns a jwt token
func (p *APIController) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var loginInfo params.PasswordLoginParams
	if err := json.NewDecoder(r.Body).Decode(&loginInfo); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	if err := loginInfo.Validate(); err != nil {
		handleError(w, err)
		return
	}
	// Brute-force defense: while this (clientIP, username) pair is over the
	// failure budget, answer with the exact body the manager produces for
	// bad credentials, without calling it. The attempt is counted before
	// authenticating (reserve) so parallel requests cannot overrun the
	// budget; a success clears the bucket. See ratelimit.go.
	attemptKey := loginAttemptKey(r, loginInfo.Username)
	if !p.loginLimiter.reserve(attemptKey) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", strconv.Itoa(p.loginLimiter.retryAfter(attemptKey)))
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Not Authorized",
			Details: "invalid username or password",
		})
		return
	}
	ctx := r.Context()
	ctx, err := p.manager.Authenticate(ctx, loginInfo)
	if err != nil {
		handleError(w, err)
		return
	}
	p.loginLimiter.recordSuccess(attemptKey)
	tokenID, err := util.GetRandomString(16)
	if err != nil {
		handleError(w, err)
		return
	}
	expireToken := time.Now().Add(p.cfg.TimeToLive.Duration())
	expires := &jwt.NumericDate{
		Time: expireToken,
	}
	claims := auth.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: expires,
			Issuer:    "gopherbin",
		},
		UserID:      auth.UserID(ctx),
		UpdatedAt:   auth.UpdatedAt(ctx),
		TokenID:     tokenID,
		IsAdmin:     auth.IsAdmin(ctx),
		IsSuperUser: auth.IsSuperUser(ctx),
		FullName:    auth.FullName(ctx),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(p.cfg.Secret))
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(params.JWTResponse{Token: tokenString})
}

// LogoutHandler will blacklist the token ID
func (p *APIController) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	claim := auth.JWTClaim(ctx)
	err := p.manager.BlacklistToken(claim.TokenID, claim.RegisteredClaims.ExpiresAt.Unix())
	if err != nil {
		handleError(w, err)
		return
	}
}

// PasteViewHandler returns details about a single paste
func (p *APIController) PasteViewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !p.confirmLimitedAccess(w, r, pasteID, false) {
		return
	}
	pasteInfo, err := p.paster.Get(ctx, pasteID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pasteInfo)
}

// PasteDownloadHandler serves a paste as a downloadable file.
func (p *APIController) PasteDownloadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !p.confirmLimitedAccess(w, r, pasteID, false) {
		return
	}

	// Consumes one of a budget paste's accesses: gated above like
	// PasteViewHandler.
	pasteInfo, err := p.paster.Get(ctx, pasteID)
	if err != nil {
		handleError(w, err)
		return
	}

	w.Header().Set("Access-Control-Expose-Headers", "x-suggested-filename, Content-Disposition")
	w.Header().Set("Content-Disposition", contentDisposition(pasteInfo.Name))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("x-suggested-filename", sanitizeDownloadName(pasteInfo.Name))
	w.Write(pasteInfo.Data)
}

// PublicPasteViewHandler returns details about a single public paste
func (p *APIController) PublicPasteViewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !p.confirmLimitedAccess(w, r, pasteID, true) {
		return
	}
	pasteInfo, err := p.paster.GetPublicPaste(ctx, pasteID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pasteInfo)
}

// PasteListHandler returns a list of pastes
func (p *APIController) PasteListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pageInt, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	maxResults, _ := strconv.ParseInt(r.URL.Query().Get("max_results"), 10, 64)
	pageInt, maxResults = clampPagination(pageInt, maxResults)
	scope := parseScope(r.URL.Query().Get("scope"))

	labels, team := parseListFilters(r)
	res, err := p.paster.List(ctx, pageInt, maxResults, scope, labels, team)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// SearchPasteHandler searches for pastes by name
// parseListFilters extracts the optional labels (comma separated) and team
// query parameters used by the paste list and search endpoints.
func parseListFilters(r *http.Request) ([]string, string) {
	var labels []string
	if raw := r.URL.Query().Get("labels"); raw != "" {
		for _, l := range strings.Split(raw, ",") {
			if l = strings.TrimSpace(l); l != "" {
				labels = append(labels, l)
			}
		}
	}
	return labels, strings.TrimSpace(r.URL.Query().Get("team"))
}

func (p *APIController) SearchPasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No search query specified",
		})
		return
	}

	pageInt, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	maxResults, _ := strconv.ParseInt(r.URL.Query().Get("max_results"), 10, 64)
	pageInt, maxResults = clampPagination(pageInt, maxResults)
	scope := parseScope(r.URL.Query().Get("scope"))

	labels, team := parseListFilters(r)
	res, err := p.paster.Search(ctx, query, pageInt, maxResults, scope, labels, team)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// DeletePasteHandler deletes a single paste
func (p *APIController) DeletePasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No paste ID specified",
		})
		return
	}
	if err := p.paster.Delete(ctx, pasteID); err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// UserListHandler handles the list of pastes
func (p *APIController) UserListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !auth.IsSuperUser(ctx) && !auth.IsAdmin(ctx) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(responses.UnauthorizedResponse)
		return
	}

	pageInt, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	maxResults, _ := strconv.ParseInt(r.URL.Query().Get("max_results"), 10, 64)
	pageInt, maxResults = clampPagination(pageInt, maxResults)

	res, err := p.manager.List(ctx, pageInt, maxResults)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// CreatePasteHandler creates a new paste
func (p *APIController) CreatePasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var pasteData params.NewPasteParams
	if err := json.NewDecoder(r.Body).Decode(&pasteData); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	var labelNames []string
	for _, l := range pasteData.Labels {
		if strings.TrimSpace(l.Name) != "" {
			labelNames = append(labelNames, l.Name)
		}
	}

	pasteInfo, err := p.paster.Create(
		ctx, pasteData.Data, pasteData.Name,
		pasteData.Language, pasteData.Description,
		pasteData.Expires, pasteData.Public, pasteData.Team,
		pasteData.Metadata, pasteData.MaxAccesses, labelNames)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pasteInfo)
}

// UpdatePasteHandler
func (p *APIController) UpdatePasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No paste ID specified",
		})
		return
	}

	var pasteData params.UpdatePasteParams
	if err := json.NewDecoder(r.Body).Decode(&pasteData); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	pasteInfo, err := p.paster.SetPrivacy(ctx, pasteID, pasteData.Public)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pasteInfo)
}

// TransferPasteHandler transfers the ownership of a paste to another user
func (p *APIController) TransferPasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No paste ID specified",
		})
		return
	}

	var transferParams params.UserActionRequest
	if err := json.NewDecoder(r.Body).Decode(&transferParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	pasteInfo, err := p.paster.TransferOwnership(ctx, pasteID, transferParams.UserID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pasteInfo)
}

// SharePasteHandler shares a paste with a user.
func (p *APIController) SharePasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No paste ID specified",
		})
		return
	}

	var userID params.UserActionRequest
	if err := json.NewDecoder(r.Body).Decode(&userID); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	targetUser, err := p.paster.ShareWithUser(ctx, pasteID, userID.UserID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targetUser)
}

// SharePasteHandler shares a paste with a user.
func (p *APIController) UnsharePasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	userID, userOK := vars["userID"]
	if !ok || !userOK {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "User ID or paste ID is missing",
		})
		return
	}

	if err := p.paster.UnshareWithUser(ctx, pasteID, userID); err != nil {
		handleError(w, err)
		return
	}
}

// UserListHandler handles the list of pastes
func (p *APIController) ListSharesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No paste ID specified",
		})
		return
	}

	res, err := p.paster.ListShares(ctx, pasteID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

//
// Admin user handlers
//

// NewUserHandler creates a new user
func (p *APIController) NewUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var newUserParams params.NewUserParams
	if err := json.NewDecoder(r.Body).Decode(&newUserParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	newUser, err := p.manager.Create(ctx, newUserParams)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newUser)
}

// GetUserHandler returns a single user by ID
func (p *APIController) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	userID, ok := vars["userID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no user ID specified",
		})
		return
	}
	userIDInt, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "invalid user ID",
		})
		return
	}

	user, err := p.manager.Get(ctx, uint(userIDInt))
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// UpdateUserHandler will update a user
func (p *APIController) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	userID, ok := vars["userID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no user ID specified",
		})
		return
	}

	userIDInt, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "invalid user ID",
		})
		return
	}
	var updateUserPayload params.UpdateUserPayload
	if err := json.NewDecoder(r.Body).Decode(&updateUserPayload); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	updatedUser, err := p.manager.Update(ctx, uint(userIDInt), updateUserPayload)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}

// DeleteUserHandler deletes a user
func (p *APIController) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	userID, ok := vars["userID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no user ID specified",
		})
		return
	}
	userIDInt, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "invalid user ID",
		})
		return
	}
	err = p.manager.Delete(ctx, uint(userIDInt))
	if err != nil {
		handleError(w, err)
		return
	}
}

//
// Teams handlers
//

// NewUserHandler creates a new team
func (p *APIController) NewTeamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var newTeamParams params.NewTeamParams

	if err := json.NewDecoder(r.Body).Decode(&newTeamParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}

	newTeam, err := p.teamManager.Create(ctx, newTeamParams.Name, strings.TrimSpace(newTeamParams.Description))
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newTeam)
}

// DeleteTeamHandler deletes a team
func (p *APIController) DeleteTeamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	teamName, ok := vars["teamName"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no team name specified",
		})
		return
	}
	err := p.teamManager.Delete(ctx, teamName)
	if err != nil {
		handleError(w, err)
		return
	}
}

// DeleteTeamHandler deletes a team
func (p *APIController) GetTeamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	teamName, ok := vars["teamName"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no team name specified",
		})
		return
	}
	team, err := p.teamManager.Get(ctx, teamName)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}

func (p *APIController) ListTeamsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pageInt, _ := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64)
	maxResults, _ := strconv.ParseInt(r.URL.Query().Get("max_results"), 10, 64)
	pageInt, maxResults = clampPagination(pageInt, maxResults)

	res, err := p.teamManager.List(ctx, pageInt, maxResults)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (p *APIController) AddTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	teamName, ok := vars["teamName"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no team name specified",
		})
		return
	}

	var addTeamMemberParams params.TeamMemberParams
	if err := json.NewDecoder(r.Body).Decode(&addTeamMemberParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	if err := addTeamMemberParams.Validate(); err != nil {
		handleError(w, err)
		return
	}

	newMember, err := p.teamManager.AddMember(ctx, teamName, addTeamMemberParams.UserID, addTeamMemberParams.Role)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newMember)

}

// ListTeamMembersHandler returns the members of a team
func (p *APIController) ListTeamMembersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	teamName, ok := vars["teamName"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no team name specified",
		})
		return
	}

	res, err := p.teamManager.ListMembers(ctx, teamName)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (p *APIController) RemoveTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	teamName, teamOK := vars["teamName"]
	member, memberOK := vars["member"]
	if !teamOK || !memberOK {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no team or member name specified",
		})
		return
	}
	err := p.teamManager.RemoveMember(ctx, teamName, member)
	if err != nil {
		handleError(w, err)
		return
	}
}

// teamNameParam extracts the teamName route variable, responding with 400
// when it is missing.
func teamNameParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	teamName, ok := mux.Vars(r)["teamName"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no team name specified",
		})
		return "", false
	}
	return teamName, true
}

// AcceptTeamInviteHandler accepts a pending team invitation for the caller.
func (p *APIController) AcceptTeamInviteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, ok := teamNameParam(w, r)
	if !ok {
		return
	}
	team, err := p.teamManager.AcceptInvite(ctx, teamName)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}

// DeclineTeamInviteHandler rejects a pending team invitation for the caller.
func (p *APIController) DeclineTeamInviteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, ok := teamNameParam(w, r)
	if !ok {
		return
	}
	if err := p.teamManager.DeclineInvite(ctx, teamName); err != nil {
		handleError(w, err)
		return
	}
}

// LeaveTeamHandler removes the caller from a team they have joined.
func (p *APIController) LeaveTeamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, ok := teamNameParam(w, r)
	if !ok {
		return
	}
	if err := p.teamManager.LeaveTeam(ctx, teamName); err != nil {
		handleError(w, err)
		return
	}
}

// UpdateTeamHandler renames a team and/or changes its description. Owner only.
func (p *APIController) UpdateTeamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, ok := teamNameParam(w, r)
	if !ok {
		return
	}
	var updateParams params.UpdateTeamParams
	if err := json.NewDecoder(r.Body).Decode(&updateParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	updated, err := p.teamManager.Update(ctx, teamName, updateParams)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// SetTeamLabelsHandler replaces the label vocabulary of a team.
func (p *APIController) SetTeamLabelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, ok := teamNameParam(w, r)
	if !ok {
		return
	}
	var labelParams params.TeamLabelsParams
	if err := json.NewDecoder(r.Body).Decode(&labelParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	updated, err := p.teamManager.SetLabels(ctx, teamName, labelParams.Labels)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// SetPasteLabelsHandler replaces the labels attached to a paste.
func (p *APIController) SetPasteLabelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pasteID, ok := mux.Vars(r)["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "No paste ID specified",
		})
		return
	}
	var labelParams params.PasteLabelsParams
	if err := json.NewDecoder(r.Body).Decode(&labelParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	updated, err := p.paster.SetLabels(ctx, pasteID, labelParams.Labels)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// SetTeamMemberRoleHandler assigns a role to an existing team member.
func (p *APIController) SetTeamMemberRoleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, teamOK := teamNameParam(w, r)
	if !teamOK {
		return
	}
	member, ok := mux.Vars(r)["member"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(responses.APIErrorResponse{
			Error:   "Bad Request",
			Details: "no member name specified",
		})
		return
	}
	var roleParams params.SetTeamMemberRoleParams
	if err := json.NewDecoder(r.Body).Decode(&roleParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	if err := roleParams.Validate(); err != nil {
		handleError(w, err)
		return
	}
	updated, err := p.teamManager.SetMemberRole(ctx, teamName, member, roleParams.Role)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// TransferTeamHandler requests a transfer of team ownership to a member.
func (p *APIController) TransferTeamHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, teamOK := teamNameParam(w, r)
	if !teamOK {
		return
	}
	var transferParams params.TeamTransferParams
	if err := json.NewDecoder(r.Body).Decode(&transferParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	team, err := p.teamManager.RequestTransfer(ctx, teamName, transferParams.UserID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}

// TeamActionHandler serves the body-less team transfer actions (accept,
// decline, cancel) selected from the route path.
func (p *APIController) TeamTransferActionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	teamName, teamOK := teamNameParam(w, r)
	if !teamOK {
		return
	}
	var team params.Teams
	var err error
	switch mux.Vars(r)["action"] {
	case "accept":
		team, err = p.teamManager.AcceptTransfer(ctx, teamName)
	case "decline":
		if err = p.teamManager.DeclineTransfer(ctx, teamName); err == nil {
			team, err = p.teamManager.Get(ctx, teamName)
		}
	case "cancel":
		if err = p.teamManager.CancelTransfer(ctx, teamName); err == nil {
			team, err = p.teamManager.Get(ctx, teamName)
		}
	default:
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	if err != nil {
		handleError(w, err)
		return
	}
	// Every action answers with the updated team, as documented.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}

// ListTeamTransfersHandler returns the ownership transfers awaiting the
// caller, for the site-wide notice.
func (p *APIController) ListTeamTransfersHandler(w http.ResponseWriter, r *http.Request) {
	transfers, err := p.teamManager.ListPendingTransfers(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transfers)
}

// ListTeamInvitesHandler returns the pending team invitations of the caller,
// powering the site-wide invitation notice in the header.
func (p *APIController) ListTeamInvitesHandler(w http.ResponseWriter, r *http.Request) {
	invites, err := p.teamManager.ListPendingInvites(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invites)
}

// ListLabelsHandler returns the label vocabulary the caller can use.
func (p *APIController) ListLabelsHandler(w http.ResponseWriter, r *http.Request) {
	vocabulary, err := p.paster.ListLabels(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vocabulary)
}

// ListOwnedLabelsHandler returns the caller's personal labels with usage
// counts, for the settings page.
func (p *APIController) ListOwnedLabelsHandler(w http.ResponseWriter, r *http.Request) {
	owned, err := p.paster.ListOwnedLabels(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(owned)
}

// UpdateLabelHandler renames and/or recolors a label the caller manages.
func (p *APIController) UpdateLabelHandler(w http.ResponseWriter, r *http.Request) {
	labelIDRaw, ok := mux.Vars(r)["labelID"]
	if !ok {
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	labelID, err := strconv.ParseUint(labelIDRaw, 10, 64)
	if err != nil || labelID == 0 {
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	var updateParams params.UpdateLabelParams
	if err := json.NewDecoder(r.Body).Decode(&updateParams); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	if err := updateParams.Validate(); err != nil {
		handleError(w, err)
		return
	}
	info, err := p.paster.UpdateLabel(r.Context(), uint(labelID), updateParams)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// DeleteLabelHandler detaches a managed label from every paste and removes
// it from its vocabulary.
func (p *APIController) DeleteLabelHandler(w http.ResponseWriter, r *http.Request) {
	labelIDRaw, ok := mux.Vars(r)["labelID"]
	if !ok {
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	labelID, err := strconv.ParseUint(labelIDRaw, 10, 64)
	if err != nil || labelID == 0 {
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	if err := p.paster.DeleteLabel(r.Context(), uint(labelID)); err != nil {
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SearchUsersHandler powers the team-invite type-ahead. It only returns
// enabled users who allow themselves to be discovered.
func (p *APIController) SearchUsersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	excludeTeam := strings.TrimSpace(r.URL.Query().Get("team"))
	results, err := p.manager.SearchUsers(ctx, query, excludeTeam)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// GetMeHandler returns the authenticated user's own profile and settings.
func (p *APIController) GetMeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.UserID(ctx)
	if userID == 0 {
		handleError(w, gErrors.ErrUnauthorized)
		return
	}
	userInfo, err := p.manager.Get(ctx, userID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userInfo)
}

// UpdateMeHandler updates the authenticated user's own settings.
func (p *APIController) UpdateMeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.UserID(ctx)
	if userID == 0 {
		handleError(w, gErrors.ErrUnauthorized)
		return
	}
	var settings params.MeSettingsParams
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		handleError(w, decodeJSONError(err))
		return
	}
	if settings.Discoverable == nil {
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	userInfo, err := p.manager.Update(ctx, userID, params.UpdateUserPayload{
		Discoverable: settings.Discoverable,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userInfo)
}
