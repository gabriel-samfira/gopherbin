package common

import (
	"context"

	"gopherbin/params"
)

// DBAuthenticator defines a database authenticator. The database
// authenticator will take a username and a password, and will perform
// the needed authentication against a configured database.
// In the future we may allow other types of authentication.
type DBAuthenticator interface {
	Login(ctx context.Context, info params.PasswordLoginParams) (context.Context, error)
	Logout(ctx context.Context) error
}
