package controllers

import (
	"context"
	"fmt"
	"gopherbin/params"
	"html/template"
	"net/http"
	"strings"
	"time"

	adminCommon "gopherbin/admin/common"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/paste/common"

	"github.com/juju/loggo"

	// "github.com/wader/gormstore"
	"github.com/gobuffalo/packr/v2"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

var log = loggo.GetLogger("gopherbin.apiserver.controllers")

const (
	sessTokenName = "session_token"
)

var templateBox = packr.New("templates", "../../templates/html")

// NewSessionAuthMiddleware returns a new session based auth middleware
func NewSessionAuthMiddleware(public []string, assetURLs []string, sess sessions.Store, manager adminCommon.UserManager) (auth.Middleware, error) {
	return &authenticationMiddleware{
		publicPaths: public,
		assetURLs:   assetURLs,
		session:     sess,
		manager:     manager,
	}, nil
}

type authenticationMiddleware struct {
	publicPaths []string
	assetURLs   []string
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

func (amw *authenticationMiddleware) isStatic(path string) bool {
	for _, val := range amw.assetURLs {
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
		// Anonymous
		return ctx, nil
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
		sess, err := amw.session.Get(r, sessTokenName)
		if err != nil {
			log.Errorf("failed to get session: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx, err := amw.sessionToContext(r.Context(), sess)
		if err != nil {
			log.Errorf("failed to convert session to ctx: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if amw.isStatic(r.URL.Path) == true {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if amw.manager.HasSuperUser() == false {
			http.Redirect(w, r.WithContext(ctx), "/firstrun", http.StatusSeeOther)
			return
		}

		if amw.isPublic(r.URL.Path) == true {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if auth.IsEnabled(ctx) == false {
			log.Errorf("User is not enabled")
			http.Redirect(w, r.WithContext(ctx), "/login", http.StatusSeeOther)
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
	ctx := r.Context()
	if auth.IsAnonymous(ctx) == false {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	s, err := templateBox.FindString("login.html")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	lm := &ErrorResponse{
		Errors: map[string]string{},
	}
	t, err := template.New("login").Parse(s)
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case "GET":
		t.Execute(w, nil)
		return
	case "POST":
		session, err := p.session.Get(r, sessTokenName)
		if err != nil {
			log.Errorf("%v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		err = r.ParseForm()
		if err != nil {
			log.Errorf("%v", err)
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
				log.Errorf("%v", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
			t.Execute(w, lm)
			return
		}
		session.Values["authenticated"] = true
		session.Values["user_id"] = auth.UserID(ctx)
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
	}

}

// LogoutHandler handles application logout requests
func (p *PasteController) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := p.session.Get(r, sessTokenName)
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

type indexRet struct {
	UserInfo  params.Users
	Languages map[string]string
	Errors    map[string]string
}

// PasteViewHandler displays a paste
func (p *PasteController) PasteViewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pasteInfo, err := p.paster.Get(ctx, pasteID)
	if err != nil {
		log.Errorf("%v", err)
		switch err {
		case gErrors.ErrNotFound:
			w.WriteHeader(http.StatusNotFound)
		case gErrors.ErrUnauthorized:
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	s, err := templateBox.FindString("paste.html")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t, err := template.New("paste").Parse(s)
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t.Execute(w, pasteInfo)
	return
}

// IndexHandler handles the index
func (p *PasteController) IndexHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userInfo, err := p.manager.Get(ctx, auth.UserID(ctx))
	if err != nil {
		if err == gErrors.ErrUnauthorized || err == gErrors.ErrNotFound {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	tplCtx := indexRet{
		UserInfo:  userInfo,
		Languages: LanguageMappings,
		Errors:    map[string]string{},
	}
	s, err := templateBox.FindString("index.html")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t, err := template.New("index").Parse(s)
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case "GET":
		t.Execute(w, tplCtx)
		return
	case "POST":
		err = r.ParseForm()
		if err != nil {
			log.Errorf("%v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		data := r.Form.Get("data")
		title := r.Form.Get("title")
		date := r.Form.Get("date")
		lang := r.Form.Get("language")

		var pasteExpiration *time.Time
		if date != "" {
			parsedTime, err := time.Parse("01/02/2006", date)
			if err != nil {
				tplCtx.Errors["DateError"] = "invalid date"
			} else {
				pasteExpiration = &parsedTime
			}
		}

		if data == "" {
			tplCtx.Errors["DataError"] = "empty paste body"
		}

		if title == "" || len(title) > 255 {
			tplCtx.Errors["TitleError"] = "title must be between 1 and 250 characters"
		}

		if lang != "" {
			if _, ok := LanguageMappings[lang]; ok {
				lang = ""
			}
		}
		if len(tplCtx.Errors) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			t.Execute(w, tplCtx)
			return
		}
		// Create(ctx context.Context, data, title, language string, expires time.Time, isPublic bool) (paste params.Paste, err error)
		pasteInfo, err := p.paster.Create(ctx, data, title, lang, pasteExpiration, false)
		if err != nil {
			switch err {
			case gErrors.ErrNotFound:
				w.WriteHeader(http.StatusNotFound)
			case gErrors.ErrUnauthorized:
				w.WriteHeader(http.StatusUnauthorized)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/p/%s", pasteInfo.ID), http.StatusFound)
		return
	}
}

// FirstRunHandler handles the index
func (p *PasteController) FirstRunHandler(w http.ResponseWriter, r *http.Request) {
	if p.manager.HasSuperUser() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s, err := templateBox.FindString("firstrun.html")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t, err := template.New("firstrun").Parse(s)
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t.Execute(w, nil)
}
