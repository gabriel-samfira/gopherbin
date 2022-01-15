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
	// StrictSlashes generates an extra request. There is no good way to match both cases
	// where you have a trailing slash and one where you don't.

	// Teams handlers
	// Remove team member
	apiRouter.Handle("/teams/{teamName}/members/{member}", log(os.Stdout, http.HandlerFunc(han.RemoveTeamMemberHandler))).Methods("DELETE", "OPTIONS")
	apiRouter.Handle("/teams/{teamName}/members/{member}/", log(os.Stdout, http.HandlerFunc(han.RemoveTeamMemberHandler))).Methods("DELETE", "OPTIONS")
	// Add team member
	apiRouter.Handle("/teams/{teamName}/members", log(os.Stdout, http.HandlerFunc(han.AddTeamMemberHandler))).Methods("POST", "OPTIONS")
	apiRouter.Handle("/teams/{teamName}/members/", log(os.Stdout, http.HandlerFunc(han.AddTeamMemberHandler))).Methods("POST", "OPTIONS")
	// List team members
	apiRouter.Handle("/teams/{teamName}/members", log(os.Stdout, http.HandlerFunc(han.ListTeamMembersHandler))).Methods("POST", "OPTIONS")
	apiRouter.Handle("/teams/{teamName}/members/", log(os.Stdout, http.HandlerFunc(han.ListTeamMembersHandler))).Methods("POST", "OPTIONS")
	// Get team
	apiRouter.Handle("/teams/{teamName}", log(os.Stdout, http.HandlerFunc(han.GetTeamHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/teams/{teamName}/", log(os.Stdout, http.HandlerFunc(han.GetTeamHandler))).Methods("GET", "OPTIONS")
	// Delete team
	apiRouter.Handle("/teams/{teamName}", log(os.Stdout, http.HandlerFunc(han.DeleteTeamHandler))).Methods("DELETE", "OPTIONS")
	apiRouter.Handle("/teams/{teamName}/", log(os.Stdout, http.HandlerFunc(han.DeleteTeamHandler))).Methods("DELETE", "OPTIONS")
	// List teams
	apiRouter.Handle("/teams", log(os.Stdout, http.HandlerFunc(han.ListTeamsHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/teams/", log(os.Stdout, http.HandlerFunc(han.ListTeamsHandler))).Methods("GET", "OPTIONS")
	// Create teams
	apiRouter.Handle("/teams", log(os.Stdout, http.HandlerFunc(han.NewTeamHandler))).Methods("POST", "OPTIONS")
	apiRouter.Handle("/teams/", log(os.Stdout, http.HandlerFunc(han.NewTeamHandler))).Methods("POST", "OPTIONS")

	// Paste handlers
	// Unshare paste
	apiRouter.Handle("/paste/{pasteID}/sharing/{userID}", log(os.Stdout, http.HandlerFunc(han.UnsharePasteHandler))).Methods("DELETE", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/sharing/{userID}/", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("DELETE", "OPTIONS")
	// Share paste
	apiRouter.Handle("/paste/{pasteID}/sharing", log(os.Stdout, http.HandlerFunc(han.SharePasteHandler))).Methods("POST", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/sharing/", log(os.Stdout, http.HandlerFunc(han.SharePasteHandler))).Methods("POST", "OPTIONS")
	// List shares
	apiRouter.Handle("/paste/{pasteID}/sharing", log(os.Stdout, http.HandlerFunc(han.ListSharesHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/sharing/", log(os.Stdout, http.HandlerFunc(han.ListSharesHandler))).Methods("GET", "OPTIONS")
	// Get paste
	apiRouter.Handle("/paste/{pasteID}", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/", log(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET", "OPTIONS")
	// Download paste
	apiRouter.Handle("/paste/{pasteID}/download", log(os.Stdout, http.HandlerFunc(han.PasteDownloadHandler))).Methods("GET", "OPTIONS")
	apiRouter.Handle("/paste/{pasteID}/download/", log(os.Stdout, http.HandlerFunc(han.PasteDownloadHandler))).Methods("GET", "OPTIONS")
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
