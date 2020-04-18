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
	"encoding/json"
	"fmt"
	"gopherbin/config"
	"net/http"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"

	adminCommon "gopherbin/admin/common"
	"gopherbin/apiserver/responses"
	gErrors "gopherbin/errors"
)

// JWTClaims holds JWT claims
type JWTClaims struct {
	UserID    int64  `json:"user"`
	UpdatedAt string `json:"updated_at"`
	TokenID   string `json:"token_id"`
	jwt.StandardClaims
}

// jwtMiddleware is the authentication middleware
// used with gorilla
type jwtMiddleware struct {
	manager adminCommon.UserManager
	cfg     config.JWTAuth
}

// NewjwtMiddleware returns a populated jwtMiddleware
func NewjwtMiddleware(manager adminCommon.UserManager, cfg config.JWTAuth) (Middleware, error) {
	return &jwtMiddleware{
		manager: manager,
		cfg:     cfg,
	}, nil
}

func (amw *jwtMiddleware) claimsToContext(ctx context.Context, claims *JWTClaims) (context.Context, error) {
	if claims == nil {
		return ctx, gErrors.ErrUnauthorized
	}

	if claims.UserID == 0 {
		// Anonymous
		return ctx, nil
	}

	adminCtx := GetAdminContext()
	userInfo, err := amw.manager.Get(adminCtx, claims.UserID)
	if err != nil {
		return ctx, err
	}
	ctx = PopulateContext(ctx, userInfo)

	if claims.UpdatedAt != UpdatedAt(ctx) {
		return ctx, fmt.Errorf("Invalid token")
	}
	return ctx, nil
}

func invalidAuthResponse(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(
		responses.APIErrorResponse{
			Error:   "Authentication failed",
			Details: "Invalid authentication token",
		})
}

// Middleware implements the middleware interface
func (amw *jwtMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Log error details when authentication fails
		if amw.manager.HasSuperUser() == false {
			w.WriteHeader(http.StatusConflict)
			w.Header().Add("Content-Type", "application/json")
			json.NewEncoder(w).Encode(responses.InitializationRequired)
			return
		}
		ctx := r.Context()
		authorizationHeader := r.Header.Get("authorization")
		if authorizationHeader == "" {
			invalidAuthResponse(w)
			return
		}

		bearerToken := strings.Split(authorizationHeader, " ")
		if len(bearerToken) != 2 {
			invalidAuthResponse(w)
			return
		}

		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(bearerToken[1], claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Invalid signing method")
			}
			return []byte(amw.cfg.Secret), nil
		})

		if err != nil {
			invalidAuthResponse(w)
			return
		}

		if token.Valid != true {
			invalidAuthResponse(w)
			return
		}

		ctx, err = amw.claimsToContext(ctx, claims)
		if err != nil {
			invalidAuthResponse(w)
			return
		}
		if IsEnabled(ctx) == false || IsAnonymous(ctx) {
			invalidAuthResponse(w)
			return
		}

		if err := amw.manager.ValidateToken(claims.TokenID); err != nil {
			invalidAuthResponse(w)
			return
		}
		ctx = SetJWTClaim(ctx, *claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
