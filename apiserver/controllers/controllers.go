package controllers

import (
	"gopherbin/params"
	"html/template"
	"net/http"

	adminCommon "gopherbin/admin/common"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/paste/common"

	// "github.com/wader/gormstore"
	"github.com/gobuffalo/packr/v2"
	"github.com/gorilla/sessions"
)

var templateBox = packr.NewBox("../templates/html")
var assetsBox = packr.NewBox("../templates/assets")

// NewPasteController returns a new *PasteController
func NewPasteController(paster common.Paster, session sessions.Store, admin adminCommon.UserManager) *PasteController {
	return &PasteController{
		paster:  paster,
		session: session,
		manager: admin,
	}
}

// PasteController implements handlers for the Gopherbin
// app
type PasteController struct {
	paster  common.Paster
	session sessions.Store
	manager adminCommon.UserManager
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
	session, err := p.session.Get(r, "session_token")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, ok := session.Values["user_id"]
	if ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s, err := templateBox.FindString("login.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	lm := &ErrorResponse{
		Errors: map[string]string{},
	}
	t, err := template.New("login").Parse(s)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case "GET":
		t.Execute(w, nil)
		return
	case "POST":
		err = r.ParseForm()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		passwd := r.Form.Get("password")
		username := r.Form.Get("username")

		if username == "" || passwd == "" {
			lm.Errors["Authentication"] = "Invalid username or password"
			w.WriteHeader(http.StatusUnauthorized)
			t.Execute(w, lm)
			return
		}

		loginParams := params.PasswordLoginParams{
			Username: username,
			Password: passwd,
		}

		ctx, err := p.manager.Authenticate(r.Context(), loginParams)
		if err != nil {
			if err == gErrors.ErrUnauthorized {
				lm.Errors["Authentication"] = "Invalid username or password"
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				lm.Errors["Authentication"] = "An unknown error occured"
				w.WriteHeader(http.StatusInternalServerError)
			}
			t.Execute(w, lm)
			return
		}
		session.Values["authenticated"] = true
		session.Values["userID"] = auth.UserID(ctx)
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}

}

// LogoutHandler handles application logout requests
func (p *PasteController) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := p.session.Get(r, "session_token")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	session.Options.MaxAge = -1
	session.Save(r, w)
}

// IndexHandler handles the index
func (p *PasteController) IndexHandler(w http.ResponseWriter, r *http.Request) {
	session, err := p.session.Get(r, "session_token")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, ok := session.Values["user_id"]
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	s, err := templateBox.FindString("index.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t, err := template.New("index").Parse(s)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case "GET":
		t.Execute(w, nil)
		return
	case "POST":
	}

	// if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
	// 	http.Error(w, "Forbidden", http.StatusForbidden)
	// 	return
	// }
}
