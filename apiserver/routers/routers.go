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

package routers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gorillaHandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/juju/loggo"

	"gopherbin/apiserver/controllers"
	"gopherbin/auth"
	"gopherbin/templates"
)

var log = loggo.GetLogger("gopherbin.apiserver.routes")

func maxAge(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var age time.Duration
		ext := filepath.Ext(r.URL.String())
		switch ext {
		case ".css", ".js":
			age = (time.Hour * 24 * 365) / time.Second
		case ".jpg", ".jpeg", ".gif", ".png", ".ico":
			age = (time.Hour * 24 * 30) / time.Second
		default:
			age = 0
		}

		if age > 0 {
			w.Header().Add("Cache-Control", fmt.Sprintf("max-age=%d, public, must-revalidate, proxy-revalidate", age))
		}

		h.ServeHTTP(w, r)
	})
}

// AddWebURLs adds web UI specific URLs to the router
func AddWebURLs(router *mux.Router, han *controllers.PasteController, authMiddleware auth.Middleware) error {
	// This is temporary. The Web UI will pe completely replaced
	// by a single page application that will leverage the REST API
	staticRouter := router.PathPrefix("/static").Subrouter()
	staticRouter.PathPrefix("/").Handler(gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.StripPrefix("/static/", maxAge(http.FileServer(templates.AssetsBox))))).Methods("GET")

	authRouter := router.PathPrefix("/auth").Subrouter()
	authRouter.Handle("/{login:login\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.LoginHandler))).Methods("GET", "POST")
	authRouter.Handle("/{logout:logout\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.LogoutHandler))).Methods("GET")

	uiRouter := router.PathPrefix("").Subrouter()
	uiRouter.Use(authMiddleware.Middleware)

	uiRouter.Handle("/{login:login\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.LoginHandler))).Methods("GET", "POST")
	uiRouter.Handle("/{logout:logout\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.LogoutHandler))).Methods("GET")
	uiRouter.Handle("/", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.IndexHandler))).Methods("GET", "POST")
	uiRouter.Handle("/firstrun", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.FirstRunHandler))).Methods("GET")
	uiRouter.Handle("/{p:p\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.PasteListHandler))).Methods("GET")
	uiRouter.Handle("/p/{pasteID}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET")
	uiRouter.Handle("/p/{pasteID}/{delete:delete\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.DeletePasteHandler))).Methods("DELETE")
	uiRouter.Handle("/admin/{users:users\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.UserListHandler))).Methods("GET")
	uiRouter.Handle("/admin/{new-user:new-user\\/?}", gorillaHandlers.CombinedLoggingHandler(os.Stdout, http.HandlerFunc(han.NewUserHandler))).Methods("GET", "POST")

	return nil
}

// AddAPIURLs adds REST API urls
func AddAPIURLs(router *mux.Router, han *controllers.APIController, authMiddleware auth.Middleware) error {
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(authMiddleware.Middleware)

	log := gorillaHandlers.CombinedLoggingHandler

	// Duplicate the route to allow fetching a paste, both with and without a traling slash.
	// StrictSlashes generates an extra request, which I am not willing to add. There is no
	// good way to match both cases where you have a trailing slash and one where you don't.
	// It is beyond me why this was never added, but i'd rather duplicate the route then
	// use StrictSlashes.
	apiRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET")
	apiRouter.Handle("/paste/{pasteID}/", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET")
	// Delete paste handlers
	apiRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.DeletePasteHandler))).Methods("DELETE")
	apiRouter.Handle("/paste/{pasteID}/", log(os.Stdout, http.HandlerFunc(han.DeletePasteHandler))).Methods("DELETE")
	// paste list
	apiRouter.Handle("/{paste:paste\\/?}", log(os.Stdout, http.HandlerFunc(han.PasteListHandler))).Methods("GET")

	// admin routes
	apiRouter.Handle("/admin/{users:users\\/?}", log(os.Stdout, http.HandlerFunc(han.UserListHandler))).Methods("GET")

	apiRouter.PathPrefix("/").Handler(log(os.Stdout, http.HandlerFunc(han.NotFoundHandler)))
	return nil
}
