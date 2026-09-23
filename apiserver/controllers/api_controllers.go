package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	return &APIController{
		paster:      paster,
		manager:     mgr,
		teamManager: teamManager,
		cfg:         cfg,
	}
}

// APIController implements handlers for the REST API
type APIController struct {
	paster      common.Paster
	manager     adminCommon.UserManager
	teamManager common.TeamManager
	cfg         config.JWTAuth
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
	case *gErrors.BadRequestError:
		w.WriteHeader(http.StatusBadRequest)
		apiErr.Error = "Bad Request"
	case *gErrors.DuplicateUserError, *gErrors.ConflictError:
		w.WriteHeader(http.StatusConflict)
		apiErr.Error = "Conflict"
	default:
		w.WriteHeader(http.StatusInternalServerError)
		apiErr.Error = "Server error"
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

// NotFoundHandler is returned when an invalid URL is acccessed
func (p *APIController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(responses.NotFoundResponse)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
		return
	}

	if err := loginInfo.Validate(); err != nil {
		handleError(w, err)
		return
	}
	ctx := r.Context()
	ctx, err := p.manager.Authenticate(ctx, loginInfo)
	if err != nil {
		handleError(w, err)
		return
	}
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

	pasteInfo, err := p.paster.Get(ctx, pasteID)
	if err != nil {
		handleError(w, err)
		return
	}

	w.Header().Set("Access-Control-Expose-Headers", "x-suggested-filename, Content-Disposition")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", pasteInfo.Name))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("x-suggested-filename", pasteInfo.Name)
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
	page := r.URL.Query().Get("page")
	pageInt, _ := strconv.ParseInt(page, 10, 64)
	maxResultsOpt := r.URL.Query().Get("max_results")
	maxResults, _ := strconv.ParseInt(maxResultsOpt, 10, 64)
	if maxResults == 0 {
		maxResults = 50
	}
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

	page := r.URL.Query().Get("page")
	pageInt, _ := strconv.ParseInt(page, 10, 64)
	maxResultsOpt := r.URL.Query().Get("max_results")
	maxResults, _ := strconv.ParseInt(maxResultsOpt, 10, 64)
	if maxResults == 0 {
		maxResults = 50
	}
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

	page := r.URL.Query().Get("page")
	pageInt, _ := strconv.ParseInt(page, 10, 64)
	maxResultsOpt := r.URL.Query().Get("max_results")
	maxResults, _ := strconv.ParseInt(maxResultsOpt, 10, 64)
	if maxResults == 0 {
		maxResults = 50
	}

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

	var pasteData params.Paste
	if err := json.NewDecoder(r.Body).Decode(&pasteData); err != nil {
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.NewBadRequestError("failed to unmarshal request: %v", err))
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
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
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
		handleError(w, gErrors.ErrBadRequest)
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
	page := r.URL.Query().Get("page")
	pageInt, _ := strconv.ParseInt(page, 10, 64)
	maxResultsOpt := r.URL.Query().Get("max_results")
	maxResults, _ := strconv.ParseInt(maxResultsOpt, 10, 64)
	if maxResults == 0 {
		maxResults = 50
	}

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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
	var err error
	switch mux.Vars(r)["action"] {
	case "accept":
		var team params.Teams
		team, err = p.teamManager.AcceptTransfer(ctx, teamName)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(team)
			return
		}
	case "decline", "cancel":
		if mux.Vars(r)["action"] == "decline" {
			err = p.teamManager.DeclineTransfer(ctx, teamName)
		} else {
			err = p.teamManager.CancelTransfer(ctx, teamName)
		}
	default:
		handleError(w, gErrors.ErrBadRequest)
		return
	}
	if err != nil {
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
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
		handleError(w, gErrors.ErrBadRequest)
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
		handleError(w, gErrors.ErrBadRequest)
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
