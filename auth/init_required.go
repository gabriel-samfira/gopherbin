package auth

import (
	"encoding/json"
	"net/http"

	adminCommon "gopherbin/admin/common"
	"gopherbin/apiserver/responses"
)


// NewjwtMiddleware returns a populated jwtMiddleware
func NewInitRequiredMiddleware(manager adminCommon.UserManager) (Middleware, error) {
	return &initRequired{
		manager: manager,
	}, nil
}


type initRequired struct {
	manager adminCommon.UserManager
}

// Middleware implements the middleware interface
func (i *initRequired) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Log error details when authentication fails
		if i.manager.HasSuperUser() == false {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(responses.InitializationRequired)
			return
		}
		ctx := r.Context()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
