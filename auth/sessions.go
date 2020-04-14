package auth

import (
	"context"
	"fmt"
	"net/http"

	adminCommon "gopherbin/admin/common"
	gErrors "gopherbin/errors"

	"github.com/gorilla/sessions"
	"github.com/juju/loggo"
)

var log = loggo.GetLogger("gopherbin.auth")

// NewSessionAuthMiddleware returns a new session based auth middleware
func NewSessionAuthMiddleware(sess sessions.Store, manager adminCommon.UserManager) (Middleware, error) {
	return &sessionAuthMiddleware{
		session: sess,
		manager: manager,
	}, nil
}

type sessionAuthMiddleware struct {
	session sessions.Store
	manager adminCommon.UserManager
}

func (amw *sessionAuthMiddleware) sessionToContext(ctx context.Context, sess *sessions.Session) (context.Context, error) {
	if sess == nil {
		return ctx, gErrors.ErrUnauthorized
	}
	userID, ok := sess.Values["user_id"]
	if !ok {
		// Anonymous
		return ctx, nil
	}
	rev, _ := sess.Values["updated_at"]
	ctx = SetUserID(ctx, userID.(int64))
	userInfo, err := amw.manager.Get(ctx, userID.(int64))
	if err != nil {
		return ctx, err
	}
	if rev != userInfo.UpdatedAt.String() {
		return ctx, gErrors.ErrInvalidSession
	}
	return PopulateContext(ctx, userInfo), nil
}

// Middleware function, which will be called for each request
func (amw *sessionAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if amw.manager.HasSuperUser() == false {
			http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
			return
		}
		sess, err := amw.session.Get(r, SessionTokenName)
		if err != nil {
			log.Errorf("failed to get session: %v", err)
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		loginWithNext := fmt.Sprintf("/auth/login?next=%s", r.URL.Path)
		ctx, err := amw.sessionToContext(r.Context(), sess)
		if err != nil {
			if err == gErrors.ErrInvalidSession {
				sess.Options.MaxAge = -1
				sess.Save(r, w)
			}
			log.Errorf("failed to convert session to ctx: %v", err)
			http.Redirect(w, r, loginWithNext, http.StatusSeeOther)
			return
		}

		if IsAnonymous(ctx) {
			http.Redirect(w, r.WithContext(ctx), loginWithNext, http.StatusSeeOther)
			return
		}

		if IsEnabled(ctx) == false {
			log.Errorf("User is not enabled")
			http.Redirect(w, r.WithContext(ctx), loginWithNext, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
