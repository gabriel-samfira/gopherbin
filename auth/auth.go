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
	"time"

	"gopherbin/params"
)

type contextFlags string

const (
	isAdminKey     contextFlags = "is_admin"
	isSuperUserKey contextFlags = "is_super"
	fullNameKey    contextFlags = "full_name"
	// UpdatedAtFlag sets the timestamp when the user was
	// updated in the context
	UpdatedAtFlag contextFlags = "updated_at"
	// UserIDFlag is the User ID flag we set in the context
	UserIDFlag    contextFlags = "user_id"
	isEnabledFlag contextFlags = "is_enabled"
	jwtTokenFlag  contextFlags = "jwt_token"
)

// PopulateContext sets the appropriate fields in the context, based on
// the user object
func PopulateContext(ctx context.Context, user params.Users) context.Context {
	ctx = SetUserID(ctx, user.ID)
	ctx = SetAdmin(ctx, user.IsAdmin)
	ctx = SetSuperUser(ctx, user.IsSuperUser)
	ctx = SetIsEnabled(ctx, user.Enabled)
	ctx = SetUpdatedAt(ctx, user.UpdatedAt)
	ctx = SetFullName(ctx, user.FullName)
	return ctx
}

// SetFullName sets the user full name in the context
func SetFullName(ctx context.Context, fullName string) context.Context {
	return context.WithValue(ctx, fullNameKey, fullName)
}

// FullName returns the full name from context
func FullName(ctx context.Context) string {
	name := ctx.Value(fullNameKey)
	if name == nil {
		return ""
	}
	return name.(string)
}

// SetUpdatedAt sets the update stamp for a user in the context
func SetUpdatedAt(ctx context.Context, tm time.Time) context.Context {
	return context.WithValue(ctx, UpdatedAtFlag, tm.String())
}

// UpdatedAt retrieves the updated_at flag
func UpdatedAt(ctx context.Context) string {
	updated := ctx.Value(UpdatedAtFlag)
	if updated == nil {
		return ""
	}
	return updated.(string)
}

// SetJWTClaim will set the JWT claim in the context
func SetJWTClaim(ctx context.Context, claim JWTClaims) context.Context {
	return context.WithValue(ctx, jwtTokenFlag, claim)
}

// JWTClaim returns the JWT claim saved in the context
func JWTClaim(ctx context.Context) JWTClaims {
	jwtClaim := ctx.Value(jwtTokenFlag)
	if jwtClaim == nil {
		return JWTClaims{}
	}
	return jwtClaim.(JWTClaims)
}

// SetIsEnabled sets a flag indicating if account is enabled
func SetIsEnabled(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, isEnabledFlag, enabled)
}

// IsEnabled returns the a boolean indicating if the enabled flag is
// set and is true or false
func IsEnabled(ctx context.Context) bool {
	elem := ctx.Value(isEnabledFlag)
	if elem == nil {
		return false
	}
	return elem.(bool)
}

// SetAdmin sets the isAdmin flag on the context
func SetAdmin(ctx context.Context, isAdmin bool) context.Context {
	return context.WithValue(ctx, isAdminKey, isAdmin)
}

// IsAdmin returns a boolean indicating whether
// or not the context belongs to a logged in user
// and if that context has the admin flag set
func IsAdmin(ctx context.Context) bool {
	elem := ctx.Value(isAdminKey)
	if elem == nil {
		return false
	}
	return elem.(bool)
}

// SetSuperUser sets the isAdmin flag on the context
func SetSuperUser(ctx context.Context, isSuper bool) context.Context {
	return context.WithValue(ctx, isSuperUserKey, isSuper)
}

// IsSuperUser returns a boolean indicating whether
// or not the context belongs to a logged in user
// and if that context has the superuser flag set
func IsSuperUser(ctx context.Context) bool {
	elem := ctx.Value(isSuperUserKey)
	if elem == nil {
		return false
	}
	return elem.(bool)
}

// SetUserID sets the userID in the context
func SetUserID(ctx context.Context, userID uint) context.Context {
	return context.WithValue(ctx, UserIDFlag, userID)
}

// UserID returns the userID from the context
func UserID(ctx context.Context) uint {
	userID := ctx.Value(UserIDFlag)
	if userID == nil {
		return 0
	}
	return userID.(uint)
}

// IsAnonymous indicates whether or not a context belongs to an
// anonymous user
func IsAnonymous(ctx context.Context) bool {
	return UserID(ctx) == 0
}

// GetAdminContext will return an admin context. This can be used internally
// when fetching users.
func GetAdminContext() context.Context {
	ctx := context.Background()
	ctx = SetUserID(ctx, 0)
	ctx = SetAdmin(ctx, true)
	ctx = SetSuperUser(ctx, false)
	ctx = SetIsEnabled(ctx, true)
	return ctx
}
