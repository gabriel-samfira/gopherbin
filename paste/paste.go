package paste

import (
	"context"
	"gopherbin/auth"
	"gopherbin/config"
	"gopherbin/models"
	"gopherbin/paste/common"
	"gopherbin/util"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/pkg/errors"
)

// NewPaster returns a SQL backed paste implementation
func NewPaster(dbCfg config.Database, cfg config.Default) (common.Paster, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}
	return &paste{
		conn: db,
		cfg:  cfg,
	}, nil
}

type paste struct {
	conn *gorm.DB
	cfg  config.Default
}

func (p *paste) getUser(userID int64) (models.Users, error) {
	// TODO: abstract this into a common interface
	var tmpUser models.Users
	q := p.conn.First(&tmpUser)
	if q.Error != nil {
		if q.RecordNotFound() {
			return models.Users{}, auth.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (p *paste) getPaste(pasteID string, userID int64) (models.Paste, error) {
	// TODO: get teams for userID
	var tmpPaste models.Paste
	// q := p.conn.First(&tmpPaste)
	// if q.Error != nil {
	// 	if q.RecordNotFound() {
	// 		return models.Paste{}, auth.ErrNotFound
	// 	}
	// 	return models.Paste{}, errors.Wrap(q.Error, "fetching paste from database")
	// }
	return tmpPaste, nil
}

func (p *paste) sqlToCommonPaste(modelPaste models.Paste) common.Paste {
	return common.Paste{
		ID:        modelPaste.ID,
		Data:      string(modelPaste.Data),
		Name:      modelPaste.Name,
		Expires:   modelPaste.Expires,
		Public:    modelPaste.Public,
		CreatedAt: modelPaste.CreatedAt,
	}
}

func (p *paste) Create(ctx context.Context, data string, expires time.Time, isPublic bool, title string) (paste common.Paste, err error) {
	pasteID, err := util.GetRandomString(24)
	if err != nil {
		return common.Paste{}, errors.Wrap(err, "getting random string")
	}
	if auth.IsAnonymous(ctx) || auth.IsEnabled(ctx) == false {
		return common.Paste{}, auth.ErrUnauthorized
	}
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return common.Paste{}, errors.Wrap(err, "fetching user")
	}
	newPaste := models.Paste{
		ID:        pasteID,
		Owner:     user,
		CreatedAt: time.Now(),
		Data:      []byte(data),
		Public:    isPublic,
		Name:      title,
	}
	q := p.conn.Create(&newPaste)
	if q.Error != nil {
		return common.Paste{}, errors.Wrap(q.Error, "creating paste")
	}
	return p.sqlToCommonPaste(newPaste), nil
}

func (p *paste) Get(ctx context.Context, pasteID string) (paste common.Paste, err error) {
	return common.Paste{}, nil
}

func (p *paste) Delete(ctx context.Context, pasteID string) error {
	return nil
}

func (p *paste) ShareWithUser(ctx context.Context, pasteID string, userID int64) error {
	return nil
}

func (p *paste) ShareWithTeam(ctx context.Context, pasteID string, teamID int64) error {
	return nil
}

func (p *paste) SetPrivacy(ctx context.Context, pasteID string, private bool) error {
	return nil
}
