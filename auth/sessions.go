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

func (smw *sessionAuthMiddleware) sessionToContext(ctx context.Context, sess *sessions.Session) (context.Context, error) {
	if sess == nil {
		return ctx, gErrors.ErrUnauthorized
	}
	userID, ok := sess.Values["user_id"]
	if !ok {
		// Anonymous
		return ctx, nil
	}
	rev, _ := sess.Values["updated_at"]
	adminCtx := GetAdminContext()
	userInfo, err := smw.manager.Get(adminCtx, userID.(int64))
	if err != nil {
		return ctx, err
	}

	ctx = PopulateContext(ctx, userInfo)
	if rev != UpdatedAt(ctx) {
		return ctx, gErrors.ErrInvalidSession
	}
	return ctx, nil
}

// Middleware function, which will be called for each request
func (smw *sessionAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if smw.manager.HasSuperUser() == false {
			http.Redirect(w, r, "/firstrun", http.StatusSeeOther)
			return
		}
		sess, err := smw.session.Get(r, SessionTokenName)
		if err != nil {
			log.Errorf("failed to get session: %v", err)
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		loginWithNext := fmt.Sprintf("/auth/login?next=%s", r.URL.Path)
		ctx, err := smw.sessionToContext(r.Context(), sess)
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
