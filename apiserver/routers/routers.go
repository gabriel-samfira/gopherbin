package routers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gorillaHandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"gopherbin/apiserver/controllers"
	"gopherbin/auth"
	"gopherbin/templates"
)

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

// GetRouter returns a new paste router
func GetRouter(han *controllers.PasteController, authMiddleware auth.Middleware) (*mux.Router, error) {
	router := mux.NewRouter()

	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", maxAge(http.FileServer(templates.AssetsBox)))).Methods("GET")
	router.Handle("/{login:login\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.LoginHandler))).Methods("GET", "POST")
	router.Handle("/{logout:logout\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.LogoutHandler))).Methods("GET")
	router.Handle("/", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.IndexHandler))).Methods("GET", "POST")
	router.Handle("/firstrun", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.FirstRunHandler))).Methods("GET")
	router.Handle("/{p:p\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.PasteListHandler))).Methods("GET")
	router.Handle("/p/{pasteID}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.PasteViewHandler))).Methods("GET")
	router.Handle("/p/{pasteID}/{delete:delete\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.DeletePasteHandler))).Methods("DELETE")
	router.Handle("/admin/{users:users\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.UserListHandler))).Methods("GET")
	router.Handle("/admin/{new-user:new-user\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.NewUserHandler))).Methods("GET", "POST")

	router.Use(authMiddleware.Middleware)
	return router, nil
}
