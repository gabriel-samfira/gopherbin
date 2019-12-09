package apiserver

// import (
// 	"context"
// 	"fmt"
// 	"log"
// 	"net"
// 	"net/http"
// 	"time"

// 	"gopherbin/apiserver/controllers"
// 	"gopherbin/apiserver/routers"
// 	"gopherbin/config"

// 	"github.com/pkg/errors"
// )

// type APIServer struct {
// 	listener net.Listener
// 	srv      *http.Server
// }

// func (h *APIServer) Start() error {
// 	go func() {
// 		if err := h.srv.Serve(h.listener); err != nil {
// 			log.Fatal(err)
// 		}
// 	}()
// 	return nil
// }

// func (h *APIServer) Stop() error {
// 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 	defer cancel()
// 	if err := h.srv.Shutdown(ctx); err != nil {
// 		return fmt.Errorf("failed to shutdown web server: %q", err)
// 	}

// 	return nil
// }

// func GetAPIServer(cfg config.APIServer) (*APIServer, error) {
// 	pasteHandler := controllers.NewPasteHandler(cfg)
// 	router, err := routers.GetRouter(cfg, pasteHandler)
// 	if err != nil {
// 		return nil, errors.Wrap(err, "getting router")
// 	}
// 	srv := &http.Server{
// 		Handler: router,
// 	}
// 	if cfg.UseTLS {
// 		tlsCfg, err := cfg.TLSConfig.TLSConfig()
// 		if err != nil {
// 			return nil, errors.Wrap(err, "getting TLS config")
// 		}
// 		srv.TLSConfig = tlsCfg
// 	}
// 	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port))
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &APIServer{
// 		srv:      srv,
// 		listener: listener,
// 	}, nil
// }
