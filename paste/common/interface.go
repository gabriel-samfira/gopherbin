package common

import (
	"time"

	"gopherbin/models"
)

// Paster is the interface for pastes
type Paster interface {
	Create(owner models.Users, data []byte, expires time.Time, isPublic bool) (id string, err error)
	Delete(id string) error
	ShareWithUser(id string, user string) error
	ShareWithTeam(id string, team string) error
	SetPrivacy(id string, private bool) error
}
