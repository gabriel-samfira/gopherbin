package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	adminCommon "gopherbin/admin/common"
	"gopherbin/apiserver/responses"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"

	"github.com/golang-jwt/jwt"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// NewAPIController returns a new APIController
func NewAPIController(paster common.Paster, teamManager common.TeamManager, mgr adminCommon.UserManager, cfg config.JWTAuth) *APIController {
	return &APIController{
		paster:      paster,
		manager:     mgr,
		teamManager: nil,
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

// NotFoundHandler is returned when an invalid URL is acccessed
func (p *APIController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.URL.Path)
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
	expireToken := time.Now().Add(p.cfg.TimeToLive.Duration()).Unix()
	claims := auth.JWTClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireToken,
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
	err := p.manager.BlacklistToken(claim.TokenID, claim.StandardClaims.ExpiresAt)
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

	res, err := p.paster.List(ctx, pageInt, maxResults)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// SearchPasteHandler searches for pastes by name
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

	res, err := p.paster.Search(ctx, query, pageInt, maxResults)
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
		fmt.Println(err)
		handleError(w, gErrors.ErrBadRequest)
		return
	}

	pasteInfo, err := p.paster.Create(
		ctx, pasteData.Data, pasteData.Name,
		pasteData.Language, pasteData.Description,
		pasteData.Expires, pasteData.Public, "",
		pasteData.Metadata)
	if err != nil {
		fmt.Println(err)
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

	newTeam, err := p.teamManager.Create(ctx, newTeamParams.Name)
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

	var addTeamMemberParams params.UserActionRequest
	if err := json.NewDecoder(r.Body).Decode(&addTeamMemberParams); err != nil {
		handleError(w, gErrors.ErrBadRequest)
		return
	}

	newMember, err := p.teamManager.AddMember(ctx, teamName, addTeamMemberParams.UserID)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newMember)

}

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
