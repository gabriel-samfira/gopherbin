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

func addWebUIRoutes(router *mux.Router) (*mux.Router, error) {
	return nil, nil
}

// GetAPIServer returns a new API server
func GetAPIServer(cfg *config.Config) (*APIServer, error) {
	paster, err := paste.NewPaster(cfg.Database, cfg.Default)
	if err != nil {
		return nil, errors.Wrap(err, "initializing paster")
	}

	userMgr, err := admin.GetUserManager(cfg.Database, cfg.Default)
	if err != nil {
		return nil, errors.Wrap(err, "getting user manager")
	}

	apiHandler := controllers.NewAPIController(paster, userMgr, cfg.APIServer.JWTAuth)

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
	allowedOrigins := handlers.AllowedOrigins(cfg.APIServer.CORSOrigins)
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})
	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"})

	srv := &http.Server{
		Handler: handlers.CORS(methodsOk, headersOk, allowedOrigins)(router),
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
