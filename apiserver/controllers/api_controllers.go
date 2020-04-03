package controllers

import (
	"encoding/json"
	adminCommon "gopherbin/admin/common"
	gErrors "gopherbin/errors"
	"gopherbin/paste/common"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// NewAPIController returns a new APIController
func NewAPIController(paster common.Paster, mgr adminCommon.UserManager) *APIController {
	return &APIController{
		paster:  paster,
		manager: mgr,
	}
}

// APIController implements handlers for the REST API
type APIController struct {
	paster  common.Paster
	manager adminCommon.UserManager
}

func handleError(w http.ResponseWriter, err error) {
	apiErr := APIErrorResponse{
		Details: err.Error(),
	}
	switch errors.Cause(err) {
	case gErrors.ErrNotFound:
		w.WriteHeader(http.StatusNotFound)
		apiErr.Error = "Not Found"
	case gErrors.ErrUnauthorized:
		w.WriteHeader(http.StatusUnauthorized)
		apiErr.Error = "Not Authorized"
	default:
		w.WriteHeader(http.StatusInternalServerError)
		apiErr.Error = "Server error"
	}
	json.NewEncoder(w).Encode(apiErr)
	return
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
	json.NewEncoder(w).Encode(pasteInfo)
}
