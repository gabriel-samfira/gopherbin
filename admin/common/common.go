package common

import (
	"context"

	"gopherbin/params"
)

// UserManager defines an interface for user management
type UserManager interface {
	Create(ctx context.Context, user params.NewUserParams) (params.Users, error)
	Get(ctx context.Context, userID int64) (params.Users, error)
	Update(ctx context.Context, userID int64, update params.UpdateUserPayload) (params.Users, error)
	Enable(ctx context.Context, userID int64) error
	Disable(ctx context.Context, userID int64) error
	Delete(ctx context.Context, userID int64) error
	Authenticate(ctx context.Context, info params.PasswordLoginParams) (context.Context, error)
}
