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

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// NewAPIController returns a new APIController
func NewAPIController(paster common.Paster, mgr adminCommon.UserManager, cfg config.JWTAuth) *APIController {
	return &APIController{
		paster:  paster,
		manager: mgr,
		cfg:     cfg,
	}
}

// APIController implements handlers for the REST API
type APIController struct {
	paster  common.Paster
	manager adminCommon.UserManager
	cfg     config.JWTAuth
}

func handleError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	apiErr := responses.APIErrorResponse{
		Details: err.Error(),
	}
	switch errors.Cause(err) {
	case gErrors.ErrNotFound:
		w.WriteHeader(http.StatusNotFound)
		apiErr.Error = "Not Found"
	case gErrors.ErrUnauthorized:
		w.WriteHeader(http.StatusUnauthorized)
		apiErr.Error = "Not Authorized"
	case gErrors.ErrBadRequest:
		w.WriteHeader(http.StatusBadRequest)
		apiErr.Error = "Bad Request"
	default:
		w.WriteHeader(http.StatusInternalServerError)
		apiErr.Error = "Server error"
	}
	json.NewEncoder(w).Encode(apiErr)
	return
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
	expireToken := time.Now().Add(p.cfg.TimeToLive.Duration).Unix()
	claims := auth.JWTClaims{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireToken,
			Issuer:    "gopherbin",
		},
		UserID:    auth.UserID(ctx),
		UpdatedAt: auth.UpdatedAt(ctx),
		TokenID:   tokenID,
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
	fmt.Println(claim)
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
	if auth.IsSuperUser(ctx) == false && auth.IsAdmin(ctx) == false {
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

	pasteInfo, err := p.paster.Create(
		ctx, pasteData.Data, pasteData.Name,
		pasteData.Language, pasteData.Expires,
		pasteData.Public, pasteData.Encrypted)
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pasteInfo)
}

// NotFoundHandler is returned when an invalid URL is acccessed
func (p *APIController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses.NotFoundResponse)
}
