package mysql

import (
	"context"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
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
	p := &paste{
		conn: db,
		cfg:  cfg,
	}
	if err := p.migrateDB(); err != nil {
		return nil, errors.Wrap(err, "migrating DB")
	}
	return p, nil
}

type paste struct {
	conn *gorm.DB
	cfg  config.Default
}

func (p *paste) migrateDB() error {
	if err := p.conn.Debug().AutoMigrate(
		&models.Users{},
		&models.Paste{},
		&models.Teams{},
	).Error; err != nil {
		return err
	}
	if err := p.conn.Debug().Model(&models.Paste{}).AddForeignKey(
		"owner", "users(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	if err := p.conn.Debug().Model(&models.Teams{}).AddForeignKey(
		"owner", "users(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}

	if err := p.conn.Debug().Model(&models.TeamUsers{}).AddForeignKey(
		"users_id", "users(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	if err := p.conn.Debug().Model(&models.TeamUsers{}).AddForeignKey(
		"teams_id", "teams(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	if err := p.conn.Debug().Model(&models.PasteTeams{}).AddForeignKey(
		"paste_id", "pastes(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	if err := p.conn.Debug().Model(&models.PasteTeams{}).AddForeignKey(
		"teams_id", "teams(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	if err := p.conn.Debug().Model(&models.PasteUsers{}).AddForeignKey(
		"paste_id", "pastes(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	if err := p.conn.Debug().Model(&models.PasteUsers{}).AddForeignKey(
		"users_id", "users(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return errors.Wrap(err, "creating foreign key")
	}
	return nil
}

func (p *paste) getUser(userID int64) (models.Users, error) {
	// TODO: abstract this into a common interface
	var tmpUser models.Users
	q := p.conn.Preload("Teams").Preload("CreatedTeams").First(&tmpUser)
	if q.Error != nil {
		if q.RecordNotFound() {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (p *paste) canAccess(paste models.Paste, user models.Users, anonymous bool) bool {
	if paste.Public == true {
		return true
	}

	if anonymous == true {
		return false
	}

	if paste.Owner == user.ID {
		return true
	}

	for _, usr := range paste.Users {
		if usr.ID == user.ID {
			return true
		}
	}

	for _, team := range paste.Teams {
		for _, usrTeam := range user.Teams {
			if team.ID == usrTeam.ID {
				return true
			}
		}
	}
	return false
}

func (p *paste) getPaste(pasteID string, user models.Users, anonymous bool) (models.Paste, error) {
	var tmpPaste models.Paste
	q := p.conn.Preload("Teams").Preload("Users").Where("id = ?", pasteID).First(&tmpPaste)
	if q.Error != nil {
		if q.RecordNotFound() {
			return models.Paste{}, gErrors.ErrNotFound
		}
		return models.Paste{}, errors.Wrap(q.Error, "fetching paste from database")
	}
	if canAccess := p.canAccess(tmpPaste, user, anonymous); !canAccess {
		return models.Paste{}, gErrors.ErrUnauthorized
	}
	return tmpPaste, nil
}

func (p *paste) sqlToCommonPaste(modelPaste models.Paste) params.Paste {
	paste := params.Paste{
		ID:        modelPaste.ID,
		Data:      string(modelPaste.Data),
		Language:  modelPaste.Language,
		Name:      modelPaste.Name,
		Public:    modelPaste.Public,
		CreatedAt: modelPaste.CreatedAt,
	}
	if modelPaste.Expires != nil {
		paste.Expires = *modelPaste.Expires
	}
	return paste
}

func (p *paste) Create(ctx context.Context, data, title, language string, expires *time.Time, isPublic bool) (paste params.Paste, err error) {
	pasteID, err := util.GetRandomString(24)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "getting random string")
	}
	if auth.IsAnonymous(ctx) || auth.IsEnabled(ctx) == false {
		return params.Paste{}, gErrors.ErrUnauthorized
	}
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching user")
	}

	newPaste := models.Paste{
		ID:        pasteID,
		Owner:     user.ID,
		CreatedAt: time.Now(),
		Data:      []byte(data),
		Expires:   expires,
		Language:  language,
		Public:    isPublic,
		Name:      title,
	}
	q := p.conn.Create(&newPaste)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "creating paste")
	}
	return p.sqlToCommonPaste(newPaste), nil
}

func (p *paste) get(ctx context.Context, pasteID string) (models.Paste, error) {
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return models.Paste{}, errors.Wrap(err, "fetching user from DB")
	}
	pst, err := p.getPaste(pasteID, user, auth.IsAnonymous(ctx))
	if err != nil {
		return models.Paste{}, errors.Wrap(err, "fetching paste")
	}
	return pst, nil
}

func (p *paste) Get(ctx context.Context, pasteID string) (paste params.Paste, err error) {
	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching paste")
	}
	return p.sqlToCommonPaste(pst), nil
}

func (p *paste) List(ctx context.Context) ([]params.Paste, error) {
	return nil, nil
}

func (p *paste) Delete(ctx context.Context, pasteID string) error {
	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return errors.Wrap(err, "fetching paste")
	}
	if pst.ID == "" {
		return nil
	}
	q := p.conn.Delete(&pst)
	if q.Error != nil && !q.RecordNotFound() {
		return errors.Wrap(q.Error, "deleting paste")
	}
	return nil
}

func (p *paste) ShareWithUser(ctx context.Context, pasteID string, userID int64) error {
	return nil
}

func (p *paste) UnshareWithUser(ctx context.Context, pasteID string, userID int64) error {
	return nil
}

func (p *paste) ShareWithTeam(ctx context.Context, pasteID string, teamID int64) error {
	return nil
}

func (p *paste) UnshareWithTeam(ctx context.Context, pasteID string, teamID int64) error {
	return nil
}

func (p *paste) SetPrivacy(ctx context.Context, pasteID string, public bool) error {
	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return errors.Wrap(err, "fetching paste")
	}
	pst.Public = public
	q := p.conn.Save(&pst)
	if q.Error != nil {
		return errors.Wrap(q.Error, "saving paste to DB")
	}
	return nil
}
