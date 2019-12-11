package auth

import (
	"context"

	"gopherbin/params"
)

type contextFlags string

const (
	isAdminKey     contextFlags = "is_admin"
	isSuperUserKey contextFlags = "is_super"
	userIDFlag     contextFlags = "user_id"
	isEnabledFlag  contextFlags = "is_enabled"
)

// PopulateContext sets the appropriate fields in the context, based on
// the user object
func PopulateContext(ctx context.Context, user params.Users) context.Context {
	ctx = SetUserID(ctx, user.ID)
	ctx = SetAdmin(ctx, user.IsAdmin)
	ctx = SetSuperUser(ctx, user.IsSuperUser)
	ctx = SetIsEnabled(ctx, user.Enabled)
	return ctx
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
func SetUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDFlag, userID)
}

// UserID returns the userID from the context
func UserID(ctx context.Context) int64 {
	userID := ctx.Value(userIDFlag)
	if userID == nil {
		return 0
	}
	return userID.(int64)
}

// IsAnonymous indicates whether or not a context belongs to an
// anonymous user
func IsAnonymous(ctx context.Context) bool {
	return UserID(ctx) == 0
}
