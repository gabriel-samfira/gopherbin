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

package apiserver

import (
	"context"
	"fmt"
	"gopherbin/paste"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"gopherbin/admin"
	"gopherbin/apiserver/controllers"
	"gopherbin/apiserver/routers"
	"gopherbin/auth"
	"gopherbin/config"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// apiPathPrefix is the mount point of every API endpoint. The request body
// cap applies only under this prefix; everything else (the web UI) is left
// alone.
const apiPathPrefix = "/api/v1"

// maxBodySizeMiddleware caps the size of request bodies served under the API
// prefix. It must run per request because http.MaxBytesReader needs the
// ResponseWriter of the request whose body it limits. When the limit is hit
// the wrapped body starts returning *http.MaxBytesError, which makes body
// decoding in the handlers fail; the shared error mapper turns that into a
// 413 response, so no handler changes are required.
func maxBodySizeMiddleware(next http.Handler, maxBodySize int64) http.Handler {
	// Defense in depth: a validated config always carries a positive
	// limit (config.APIServer.Validate applies the default), but never
	// build an uncapped chain if handed a zeroed config.
	if maxBodySize <= 0 {
		maxBodySize = config.DefaultMaxBodySize
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPathPrefix) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		}
		next.ServeHTTP(w, r)
	})
}

// contentSecurityPolicy is the fixed CSP shipped on every response. The app
// serves the SvelteKit SPA and the JSON API from the same origin and keeps
// the JWT in localStorage, so script execution is the main XSS risk:
// script-src stays strict ('self' only — the bundle ships external module
// scripts and needs no unsafe-inline/unsafe-eval). style-src must allow
// 'unsafe-inline' because Svelte 5 injects component <style> elements at
// runtime and CodeMirror 6 builds style sheets programmatically. img-src and
// font-src allow data: URLs for inline icons/fonts emitted by the bundle.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

// securityHeadersMiddleware sets a fixed set of security response headers on
// every response the server produces: API JSON, SPA assets and error pages
// alike. The headers are Set on w.Header() before delegating, so they are in
// place before the first WriteHeader fires; handlers that overwrite their own
// Content-Type etc. are unaffected. As the outermost wrapper in
// newServerHandler it also covers the paths that never reach a controller:
// mux's default 404, CORS-rejected requests, 413s from the body cap. Setting
// them on 204/101 responses is harmless and keeps the rule uniform.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		next.ServeHTTP(w, r)
	})
}

// newServerHandler assembles the HTTP handler chain for the API server.
//
// Order (outermost first): security headers, then CORS, then the body-size
// cap. Headers go outermost so every response — including CORS-rejected and
// cap-triggered ones — carries them. The body-size cap is applied inside the
// (optional) CORS wrapper so that error responses triggered by the cap still
// carry CORS headers. The CORS wrapper is only installed when explicit
// origins are configured: with an empty cors_origins list gorilla/handlers
// would fall back to "Access-Control-Allow-Origin: *", so we fail closed and
// emit no CORS headers at all instead.
func newServerHandler(router *mux.Router, cfg *config.Config) http.Handler {
	var handler http.Handler = maxBodySizeMiddleware(router, cfg.APIServer.MaxBodySize)

	if len(cfg.APIServer.CORSOrigins) > 0 {
		allowedOrigins := handlers.AllowedOrigins(cfg.APIServer.CORSOrigins)
		methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})
		// Configured origins are trusted clients of the API, so they may
		// also confirm limited-access views.
		headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization", controllers.ConsumeAccessHeader})
		handler = handlers.CORS(methodsOk, headersOk, allowedOrigins)(handler)
	}
	return securityHeadersMiddleware(handler)
}

// APIServer is the API server worker
type APIServer struct {
	listener    net.Listener
	srv         *http.Server
	sessCleanup chan struct{}
}

// Start starts the API server
func (h *APIServer) Start() error {
	go func() {
		if err := h.srv.Serve(h.listener); err != nil {
			log.Fatal(err)
		}
	}()
	return nil
}

// Stop stops the APi server
func (h *APIServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown web server: %q", err)
	}
	close(h.sessCleanup)
	return nil
}

// GetAPIServer returns a new API server
func GetAPIServer(cfg *config.Config) (*APIServer, error) {
	paster, err := paste.NewPaster(cfg.Database)
	if err != nil {
		return nil, errors.Wrap(err, "initializing paster")
	}

	teamMgr, err := paste.NewTeamManager(cfg.Database)
	if err != nil {
		return nil, errors.Wrap(err, "initializing team manager")
	}

	userMgr, err := admin.GetUserManager(cfg.Database)
	if err != nil {
		return nil, errors.Wrap(err, "getting user manager")
	}

	trustedProxies, err := cfg.APIServer.TrustedProxyNets()
	if err != nil {
		return nil, errors.Wrap(err, "parsing trusted proxies")
	}
	apiHandler := controllers.NewAPIController(paster, teamMgr, userMgr, cfg.APIServer.JWTAuth, trustedProxies)

	jwtMiddleware, err := auth.NewjwtMiddleware(userMgr, cfg.APIServer.JWTAuth)
	if err != nil {
		return nil, errors.Wrap(err, "initializing jwt middleware")
	}

	initMiddleware, err := auth.NewInitRequiredMiddleware(userMgr)
	if err != nil {
		return nil, errors.Wrap(err, "initializing init required middleware")
	}
	router := mux.NewRouter()
	corwMw := mux.CORSMethodMiddleware(router)

	if err := routers.AddAPIURLs(router, apiHandler, jwtMiddleware, initMiddleware); err != nil {
		return nil, errors.Wrap(err, "setting API urls")
	}

	router.Use(corwMw)

	srv := &http.Server{
		Handler: newServerHandler(router, cfg),
		// Transport level hardening. ReadHeaderTimeout bounds
		// slowloris-style header stalls; the generous read/write
		// timeouts still allow large pastes (capped at
		// cfg.APIServer.MaxBodySize) over slow links. No endpoint
		// streams responses (handlers write full bodies, no
		// http.Flusher / io.Copy use), so WriteTimeout cannot cut
		// off an in-flight stream.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if cfg.APIServer.UseTLS {
		tlsCfg, err := cfg.APIServer.TLSConfig.TLSConfig()
		if err != nil {
			return nil, errors.Wrap(err, "getting TLS config")
		}
		srv.TLSConfig = tlsCfg
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.APIServer.Bind, cfg.APIServer.Port))
	if err != nil {
		return nil, err
	}
	return &APIServer{
		srv:      srv,
		listener: listener,
	}, nil
}
