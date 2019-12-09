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
	Create(ownerID string, data []byte, expires time.Time, isPublic bool) (paste Paste, err error)
	Get(ownerID string, pasteID string) (paste Paste, err error)
	Delete(id string) error
	ShareWithUser(id string, user string) error
	ShareWithTeam(id string, team string) error
	SetPrivacy(id string, private bool) error
}
