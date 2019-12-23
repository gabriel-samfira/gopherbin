package routers

import (
	"net/http"
	"os"

	"github.com/gobuffalo/packr/v2"
	gorillaHandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"gopherbin/apiserver/controllers"
)

var assetsBox = packr.New("assets", "../../templates/assets")

// GetRouter returns a new paste router
func GetRouter(han *controllers.PasteController) (*mux.Router, error) {
	router := mux.NewRouter()
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(assetsBox))).Methods("GET")
	router.Handle("/{login:login\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.LoginHandler))).Methods("GET", "POST")
	router.Handle("/{logout:logout\\/?}", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.LogoutHandler))).Methods("GET")
	router.Handle("/", gorillaHandlers.LoggingHandler(os.Stdout, http.HandlerFunc(han.IndexHandler))).Methods("GET", "POST")

	return router, nil
}
