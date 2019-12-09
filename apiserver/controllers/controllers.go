package controllers

import (
	"gopherbin/paste/common"
	"net/http"

	"github.com/wader/gormstore"
)

// NewPasteController returns a new *PasteController
func NewPasteController(paster common.Paster, session *gormstore.Store) *PasteController {
	return &PasteController{
		paster:  paster,
		session: session,
	}
}

// PasteController implements handlers for the Gopherbin
// app
type PasteController struct {
	paster  common.Paster
	session *gormstore.Store
}

// RegisterHandler handles registration of a new user
func (p *PasteController) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := p.session.Get(r, "session_token")
	_, ok := session.Values["user_id"]
	if ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// user := &common.NewUser{}
	// json.NewDecoder(r.Body).Decode(user)
}

// LoginHandler handles application login requests
func (p *PasteController) LoginHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := p.session.Get(r, "session_token")
	_, ok := session.Values["user_id"]
	if ok {
		return
	}
}

// LogoutHandler handles application logout requests
func (p *PasteController) LogoutHandler(writer http.ResponseWriter, req *http.Request) {}
