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
	"net/http"
	"os"

	"github.com/NYTimes/gziphandler"
	gorillaHandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"gopherbin/apiserver/controllers"
	"gopherbin/auth"
	"gopherbin/webui"
)

// AddAPIURLs adds REST API urls
func AddAPIURLs(router *mux.Router, han *controllers.APIController, authMiddleware, initMiddleware auth.Middleware) error {
	log := gorillaHandlers.CombinedLoggingHandler

	apiSubRouter := router.PathPrefix("/api/v1").Subrouter()

	// First run
	// FirstRunHandler
	firstRunRouter := apiSubRouter.PathPrefix("/first-run").Subrouter()
	firstRunRouter.Handle("/", log(os.Stdout, http.HandlerFunc(han.FirstRunHandler))).Methods("POST", "OPTIONS")

	// Public API endpoints
	publicRouter := apiSubRouter.PathPrefix("/public").Subrouter()
	publicRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.PublicPasteViewHandler))).Methods("GET", "OPTIONS")

	// Login
	authRouter := apiSubRouter.PathPrefix("/auth").Subrouter()
	authRouter.Handle("/{login:login\\/?}", log(os.Stdout, http.HandlerFunc(han.LoginHandler))).Methods("POST", "OPTIONS")
	authRouter.Use(initMiddleware.Middleware)

	// Private API endpoints
	apiRouter := apiSubRouter.PathPrefix("").Subrouter()
	apiRouter.Use(initMiddleware.Middleware)
	apiRouter.Use(authMiddleware.Middleware)
	// Duplicate the route to allow fetching a paste, both with and without a traling slash.
	// StrictSlashes generates an extra request, which I am not willing to add. There is no
	// good way to match both cases where you have a trailing slash and one where you don't.
	// It is beyond me why this was never added, but i'd rather duplicate the route then
	// use StrictSlashes.
	apiRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET", "OPTIONS")
	// Update paste
	apiRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.UpdatePasteHandler))).Methods("PUT", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/", log(os.Stdout, http.HandlerFunc(han.UpdatePasteHandler))).Methods("PUT", "OPTIONS")
	// Delete paste handlers
	apiRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.DeletePasteHandler))).Methods("DELETE", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/", log(os.Stdout, http.HandlerFunc(han.DeletePasteHandler))).Methods("DELETE", "OPTIONS")
	// paste list
	apiRouter.Handle("/{paste:paste\\/?}", log(os.Stdout, http.HandlerFunc(han.PasteListHandler))).Methods("GET", "OPTIONS")
	// Create paste
	apiRouter.Handle("/{paste:paste\\/?}", log(os.Stdout, http.HandlerFunc(han.CreatePasteHandler))).Methods("POST", "OPTIONS")
	// logout
	apiRouter.Handle("/{logout:logout\\/?}", log(os.Stdout, http.HandlerFunc(han.LogoutHandler))).Methods("GET", "OPTIONS")
	// admin routes
	apiRouter.Handle("/admin/{users:users\\/?}", log(os.Stdout, http.HandlerFunc(han.UserListHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/admin/{users:users\\/?}", log(os.Stdout, http.HandlerFunc(han.NewUserHandler))).Methods("POST", "OPTIONS")
	// update user
	apiRouter.Handle("/admin/users/{userID}", log(os.Stdout, http.HandlerFunc(han.UpdateUserHandler))).Methods("PUT", "OPTIONS")
	apiRouter.Handle("/admin/users/{userID}/", log(os.Stdout, http.HandlerFunc(han.UpdateUserHandler))).Methods("PUT", "OPTIONS")
	// delete user
	apiRouter.Handle("/admin/users/{userID}", log(os.Stdout, http.HandlerFunc(han.DeleteUserHandler))).Methods("DELETE", "OPTIONS")
	apiRouter.Handle("/admin/users/{userID}/", log(os.Stdout, http.HandlerFunc(han.DeleteUserHandler))).Methods("DELETE", "OPTIONS")

	apiRouter.PathPrefix("/").Handler(log(os.Stdout, http.HandlerFunc(han.NotFoundHandler)))

	router.PathPrefix("/").Handler(log(os.Stdout, gziphandler.GzipHandler(http.HandlerFunc(webui.UIHandler))))
	return nil
}
