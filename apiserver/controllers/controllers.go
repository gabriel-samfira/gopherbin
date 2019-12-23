package controllers

import (
	"context"
	"gopherbin/params"
	"html/template"
	"net/http"
	"strings"

	adminCommon "gopherbin/admin/common"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/paste/common"

	// "github.com/wader/gormstore"
	"github.com/gobuffalo/packr/v2"
	"github.com/gorilla/sessions"
)

const (
	sessTokenName = "session_token"
)

var templateBox = packr.New("templates", "../../templates/html")

// NewSessionAuthMiddleware returns a new session based auth middleware
func NewSessionAuthMiddleware(public []string, sess sessions.Store, manager adminCommon.UserManager) (auth.Middleware, error) {
	return &authenticationMiddleware{
		publicPaths: public,
		session:     sess,
		manager:     manager,
	}, nil
}

type authenticationMiddleware struct {
	publicPaths []string
	session     sessions.Store
	manager     adminCommon.UserManager
}

func (amw *authenticationMiddleware) isPublic(path string) bool {
	for _, val := range amw.publicPaths {
		if strings.HasPrefix(path, val) == true {
			return true
		}
	}
	return false
}

func (amw *authenticationMiddleware) sessionToContext(ctx context.Context, sess *sessions.Session) (context.Context, error) {
	if sess == nil {
		return ctx, gErrors.ErrUnauthorized
	}
	userID, ok := sess.Values["user_id"]
	if !ok {
		return ctx, gErrors.ErrUnauthorized
	}
	ctx = auth.SetUserID(ctx, userID.(int64))
	userInfo, err := amw.manager.Get(ctx, userID.(int64))
	if err != nil {
		return ctx, err
	}
	return auth.PopulateContext(ctx, userInfo), nil
}

// Middleware function, which will be called for each request
func (amw *authenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if amw.isPublic(r.URL.Path) == true {
			next.ServeHTTP(w, r)
			return
		}

		if amw.manager.HasSuperUser() == false {
			http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
			return
		}

		sess, err := amw.session.Get(r, sessTokenName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx, err := amw.sessionToContext(r.Context(), sess)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if auth.IsEnabled(ctx) == false {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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
	session, _ := p.session.Get(r, sessTokenName)
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
	session, err := p.session.Get(r, sessTokenName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	if auth.IsAnonymous(ctx) == false {
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

		ctx, err := p.manager.Authenticate(ctx, loginParams)
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
	session, err := p.session.Get(r, sessTokenName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	session.Options.MaxAge = -1
	session.Save(r, w)
}

// IndexHandler handles the index
func (p *PasteController) IndexHandler(w http.ResponseWriter, r *http.Request) {
	session, err := p.session.Get(r, sessTokenName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	userID, ok := session.Values["user_id"]
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	ctx := auth.SetUserID(r.Context(), userID.(int64))

	userInfo, err := p.manager.Get(ctx, userID.(int64))
	if err != nil {
		if err == gErrors.ErrUnauthorized || err == gErrors.ErrNotFound {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
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
		t.Execute(w, userInfo)
		return
	case "POST":
	}

	// if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
	// 	http.Error(w, "Forbidden", http.StatusForbidden)
	// 	return
	// }
}
