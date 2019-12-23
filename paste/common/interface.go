package common

import (
	"context"
	"time"

	"gopherbin/params"
)

// Paster is the interface for pastes
type Paster interface {
	Create(ctx context.Context, data, title, language string, expires *time.Time, isPublic bool) (paste params.Paste, err error)
	Get(ctx context.Context, pasteID string) (paste params.Paste, err error)
	List(ctx context.Context) (paste []params.Paste, err error)
	Delete(ctx context.Context, pasteID string) error
	ShareWithUser(ctx context.Context, pasteID string, userID int64) error
	UnshareWithUser(ctx context.Context, pasteID string, userID int64) error
	ShareWithTeam(ctx context.Context, pasteID string, teamID int64) error
	UnshareWithTeam(ctx context.Context, pasteID string, teamID int64) error
	SetPrivacy(ctx context.Context, pasteID string, public bool) error
}
