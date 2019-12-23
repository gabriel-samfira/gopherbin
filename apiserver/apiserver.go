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
	"gopherbin/config"
	"gopherbin/util"

	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
	"github.com/wader/gormstore"
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

func initSessionStor(cfg *config.Config, quit chan struct{}) (sessions.Store, error) {
	db, err := util.NewDBConn(cfg.Database)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}
	store := gormstore.New(db, []byte(cfg.APIServer.SessionSecret))
	go store.PeriodicCleanup(1*time.Hour, quit)
	return store, nil
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

	sessQuit := make(chan struct{})
	sess, err := initSessionStor(cfg, sessQuit)

	if err != nil {
		return nil, errors.Wrap(err, "initializing session store")
	}
	pasteHandler := controllers.NewPasteController(paster, sess, userMgr)
	publicURL := []string{
		"/login",
		"/logout",
	}
	staticAssets := []string{
		"/static",
		"/firstrun",
	}
	authMiddleware, err := controllers.NewSessionAuthMiddleware(publicURL, staticAssets, sess, userMgr)
	if err != nil {
		return nil, errors.Wrap(err, "initializing auth middleware")
	}

	router, err := routers.GetRouter(pasteHandler, authMiddleware)
	if err != nil {
		return nil, errors.Wrap(err, "getting router")
	}
	srv := &http.Server{
		Handler: router,
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
		srv:         srv,
		listener:    listener,
		sessCleanup: sessQuit,
	}, nil
}
