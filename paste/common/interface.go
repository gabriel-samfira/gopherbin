package common

import (
	"context"
	"time"
)

// UserManager defines an interface for user management
type UserManager interface {
	Create(ctx context.Context, user Users) (Users, error)
	Update(ctx context.Context, userID int64, update UpdateUserPayload) (Users, error)
	Enable(ctx context.Context, userID int64) error
	Disable(ctx context.Context, userID int64) error
	Delete(ctx context.Context, userID int64) error
}

// Paster is the interface for pastes
type Paster interface {
	Create(ctx context.Context, data string, expires time.Time, isPublic bool, title string) (paste Paste, err error)
	Get(ctx context.Context, pasteID string) (paste Paste, err error)
	Delete(ctx context.Context, pasteID string) error
	ShareWithUser(ctx context.Context, pasteID string, userID int64) error
	ShareWithTeam(ctx context.Context, pasteID string, teamID int64) error
	SetPrivacy(ctx context.Context, pasteID string, private bool) error
}
