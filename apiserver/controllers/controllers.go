// Copyright 2019 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package controllers

import (
	"context"
	"fmt"
	"gopherbin/params"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	adminCommon "gopherbin/admin/common"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/paste/common"
	"gopherbin/templates"
	"gopherbin/util"

	"github.com/juju/loggo"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
)

var log = loggo.GetLogger("gopherbin.apiserver.controllers")

const (
	sessTokenName = "session_token"
)

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
	rev, _ := sess.Values["updated_at"]
	ctx = auth.SetUserID(ctx, userID.(int64))
	userInfo, err := amw.manager.Get(ctx, userID.(int64))
	if err != nil {
		return ctx, err
	}
	if rev != userInfo.UpdatedAt.String() {
		return ctx, gErrors.ErrInvalidSession
	}
	return auth.PopulateContext(ctx, userInfo), nil
}

// Middleware function, which will be called for each request
func (amw *authenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if amw.isStatic(r.URL.Path) == true {
			next.ServeHTTP(w, r)
			return
		}

		if amw.manager.HasSuperUser() == false {
			http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
			return
		}

		if amw.isPublic(r.URL.Path) == true {
			next.ServeHTTP(w, r)
			return
		}

		sess, err := amw.session.Get(r, sessTokenName)
		if err != nil {
			log.Errorf("failed to get session: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		loginWithNext := fmt.Sprintf("/login?next=%s", r.URL.Path)
		ctx, err := amw.sessionToContext(r.Context(), sess)
		if err != nil {
			if err == gErrors.ErrInvalidSession {
				sess.Options.MaxAge = -1
				sess.Save(r, w)
			}
			log.Errorf("failed to convert session to ctx: %v", err)
			http.Redirect(w, r, loginWithNext, http.StatusSeeOther)
			return
		}

		if auth.IsAnonymous(ctx) {
			http.Redirect(w, r.WithContext(ctx), loginWithNext, http.StatusSeeOther)
			return
		}

		if auth.IsEnabled(ctx) == false {
			log.Errorf("User is not enabled")
			http.Redirect(w, r.WithContext(ctx), loginWithNext, http.StatusSeeOther)
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
		funcMap: template.FuncMap{
			"dict": dict,
		},
	}
}

// PasteController implements handlers for the Gopherbin
// app
type PasteController struct {
	paster  common.Paster
	session sessions.Store
	manager adminCommon.UserManager
	funcMap template.FuncMap
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

// FirstRunHandler handles the index
func (p *PasteController) FirstRunHandler(w http.ResponseWriter, r *http.Request) {
	if p.manager.HasSuperUser() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s, err := templates.TemplateBox.FindString("firstrun.html")
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

// LoginHandler handles application login requests
func (p *PasteController) LoginHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if auth.IsAnonymous(ctx) == false {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	next := r.URL.Query().Get("next")

	s, err := templates.TemplateBox.FindString("login.html")
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
		// session.Values["user_id"] = auth.UserID(ctx)
		// session.Values["updated_at"] = auth.UpdatedAt(ctx)
		session.Values["updated_at"] = auth.UpdatedAt(ctx)
		session.Options.MaxAge = 31536000
		session.Save(r, w)
		if next != "" {
			http.Redirect(w, r, next, http.StatusFound)
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
		return
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
	err = session.Save(r, w)
	if err != nil {
		log.Errorf("%v", err)
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

type indexRet struct {
	UserInfo  params.Users
	Languages map[string]string
	Errors    map[string]string
}

type pasteRet struct {
	UserInfo params.Users
	Paste    params.Paste
	Error    string
}

// PasteViewHandler displays a paste
func (p *PasteController) PasteViewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userInfo, err := p.manager.Get(ctx, auth.UserID(ctx))
	if err != nil {
		if err == gErrors.ErrUnauthorized || err == gErrors.ErrNotFound {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	t, err := p.getTemplateWithHelpers("paste")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	ret := pasteRet{
		UserInfo: userInfo,
	}
	pasteInfo, err := p.paster.Get(ctx, pasteID)
	if err != nil {
		log.Errorf("%v", err)
		switch errors.Cause(err) {
		case gErrors.ErrNotFound:
			w.WriteHeader(http.StatusNotFound)
			ret.Error = "Not Found"
			t.Execute(w, ret)
		case gErrors.ErrUnauthorized:
			w.WriteHeader(http.StatusUnauthorized)
			ret.Error = "Not Authorized"
			t.Execute(w, ret)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			ret.Error = "Server error. Check back later."
			t.Execute(w, ret)
		}
		return
	}
	ret.Paste = pasteInfo
	t.Execute(w, ret)
	return
}

func (p *PasteController) getTemplateWithHelpers(name string) (*template.Template, error) {
	withExt := fmt.Sprintf("%s.html", name)
	s, err := templates.TemplateBox.FindString(withExt)
	if err != nil {
		return nil, err
	}

	nav, err := templates.TemplateBox.FindString("navbar.html")
	if err != nil {
		return nil, err
	}

	head, err := templates.TemplateBox.FindString("header.html")
	if err != nil {
		return nil, err
	}

	withHead, err := template.New("header").Funcs(p.funcMap).Parse(head)
	if err != nil {
		return nil, err
	}
	tplWithNav, err := withHead.New("navbar").Funcs(p.funcMap).Parse(nav)
	if err != nil {
		return nil, err
	}

	t, err := tplWithNav.New(name).Funcs(p.funcMap).Parse(s)
	if err != nil {
		return nil, err
	}
	return t, nil
}

type listView struct {
	UserInfo params.Users
	Pastes   params.PasteListResult
}

type userListView struct {
	UserInfo params.Users
	Users    params.UserListResult
}

// UserListHandler handles the list of pastes
func (p *PasteController) UserListHandler(w http.ResponseWriter, r *http.Request) {
	t, err := p.getTemplateWithHelpers("user_list")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if auth.IsSuperUser(ctx) == false && auth.IsAdmin(ctx) == false {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
		return
	}
	userInfo, err := p.manager.Get(ctx, auth.UserID(ctx))
	if err != nil {
		if err == gErrors.ErrUnauthorized || err == gErrors.ErrNotFound {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	page := r.URL.Query().Get("page")
	pageInt, _ := strconv.ParseInt(page, 10, 64)
	// TODO: make this user defined
	var maxResults int64 = 50
	res, err := p.manager.List(ctx, pageInt, maxResults)
	if err != nil {
		switch errors.Cause(err) {
		case gErrors.ErrNotFound:
			w.WriteHeader(http.StatusNotFound)
		case gErrors.ErrUnauthorized:
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	retCtx := userListView{
		UserInfo: userInfo,
		Users:    res,
	}
	t.Execute(w, retCtx)
	return
}

// PasteListHandler handles the list of pastes
func (p *PasteController) PasteListHandler(w http.ResponseWriter, r *http.Request) {
	t, err := p.getTemplateWithHelpers("paste_list")
	if err != nil {
		log.Errorf("%v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	userInfo, err := p.manager.Get(ctx, auth.UserID(ctx))
	if err != nil {
		if err == gErrors.ErrUnauthorized || err == gErrors.ErrNotFound {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	page := r.URL.Query().Get("page")
	pageInt, _ := strconv.ParseInt(page, 10, 64)
	// TODO: make this user defined
	var maxResults int64 = 50
	res, err := p.paster.List(ctx, pageInt, maxResults)
	if err != nil {
		switch errors.Cause(err) {
		case gErrors.ErrNotFound:
			w.WriteHeader(http.StatusNotFound)
		case gErrors.ErrUnauthorized:
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	retCtx := listView{
		UserInfo: userInfo,
		Pastes:   res,
	}
	t.Execute(w, retCtx)
	return
}

// DeletePasteHandler deletes a paste
func (p *PasteController) DeletePasteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	pasteID, ok := vars["pasteID"]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := p.paster.Delete(ctx, pasteID); err != nil {
		switch errors.Cause(err) {
		case gErrors.ErrNotFound:
			w.WriteHeader(http.StatusNotFound)
		case gErrors.ErrUnauthorized:
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
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

	t, err := p.getTemplateWithHelpers("index")
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

		publicOpt := r.Form.Get("public")
		var public bool
		if publicOpt == "on" {
			public = true
		}

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
			if !hasLanguage(lang) {
				lang = ""
			}
		}
		if len(tplCtx.Errors) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			t.Execute(w, tplCtx)
			return
		}
		// Create(ctx context.Context, data, title, language string, expires time.Time, isPublic bool) (paste params.Paste, err error)
		pasteInfo, err := p.paster.Create(ctx, data, title, lang, pasteExpiration, public)
		if err != nil {
			switch errors.Cause(err) {
			case gErrors.ErrNotFound:
				w.WriteHeader(http.StatusNotFound)
			case gErrors.ErrUnauthorized:
				w.WriteHeader(http.StatusUnauthorized)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/p/%s", pasteInfo.PasteID), http.StatusFound)
		return
	}
}

type newUserRet struct {
	UserInfo params.Users
	Errors   map[string]string
}

// NewUserHandler creates a new user
func (p *PasteController) NewUserHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if auth.IsSuperUser(ctx) == false && auth.IsAdmin(ctx) == false {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
		return
	}

	userInfo, err := p.manager.Get(ctx, auth.UserID(ctx))
	if err != nil {
		if err == gErrors.ErrUnauthorized || err == gErrors.ErrNotFound {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
	}
	tplCtx := newUserRet{
		UserInfo: userInfo,
		Errors:   map[string]string{},
	}

	t, err := p.getTemplateWithHelpers("new_user")
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
		email := r.Form.Get("email")
		fullname := r.Form.Get("fullname")
		password := r.Form.Get("password")
		password2 := r.Form.Get("password2")
		makeAdminOpt := r.Form.Get("makeadmin")
		enabledOpt := r.Form.Get("enabled")

		var makeAdmin bool
		var enabled bool
		if makeAdminOpt == "on" && auth.IsSuperUser(ctx) {
			makeAdmin = true
		}

		if enabledOpt == "on" {
			enabled = true
		}

		if fullname == "" || len(fullname) > 254 {
			tplCtx.Errors["FullNameError"] = "Full name must be between 1 and 254 characters"
		}

		if email == "" || !util.IsValidEmail(email) {
			tplCtx.Errors["EmailError"] = "invalid email"
		}

		if password == "" || password != password2 {
			tplCtx.Errors["PasswordError"] = "password must not be empty and must match"
		}

		newUserParams := params.NewUserParams{
			Email:    email,
			FullName: fullname,
			IsAdmin:  makeAdmin,
			Password: password,
			Enabled:  enabled,
		}
		_, err = p.manager.Create(ctx, newUserParams)
		if err != nil {
			switch errors.Cause(err) {
			case gErrors.ErrNotFound:
				w.WriteHeader(http.StatusNotFound)
			case gErrors.ErrUnauthorized:
				w.WriteHeader(http.StatusUnauthorized)
				tplCtx.Errors["UnauthorizedError"] = "You are not authorized to perform this operation"
			case gErrors.ErrDuplicateUser:
				w.WriteHeader(http.StatusBadRequest)
				tplCtx.Errors["EmailError"] = "User already exists"
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
			if len(tplCtx.Errors) > 0 {
				w.WriteHeader(http.StatusBadRequest)
				t.Execute(w, tplCtx)
				return
			}
		}
		http.Redirect(w, r, "/admin/users", http.StatusFound)
		return
	}
}
