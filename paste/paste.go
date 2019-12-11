package paste

import (
	"context"
	"gopherbin/auth"
	"gopherbin/config"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/pkg/errors"
)

/*
func (c *Collector) MigrateDatabase() error {
	db, err := c.GetDBConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Debug().AutoMigrate(
		&models.Block{},
		&models.Tx{},
		&models.Output{},
		&models.Address{},
	).Error; err != nil {
		return err
	}

	if err := db.Debug().Model(&models.Tx{}).AddForeignKey(
		"blk_id", "blocks(height)",
		"CASCADE", "CASCADE").Error; err != nil {

		return err
	}
	if err := db.Debug().Model(&models.Output{}).AddForeignKey(
		"included_in_tx", "txes(tx_id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return err
	}
	if err := db.Debug().Model(&models.Output{}).AddForeignKey(
		"spent_with_tx", "txes(tx_id)",
		"SET NULL", "CASCADE").Error; err != nil {

		return err
	}
	if err := db.Debug().Model(&models.Output{}).AddForeignKey(
		"spent_with_block", "blocks(height)",
		"SET NULL", "CASCADE").Error; err != nil {

		return err
	}
	return nil
}
*/

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

		return err
	}
	if err := p.conn.Debug().Model(&models.Teams{}).AddForeignKey(
		"owner", "users(id)",
		"CASCADE", "CASCADE").Error; err != nil {

		return err
	}
	return nil
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

func (p *paste) getPaste(pasteID string, user models.Users) (models.Paste, error) {
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

func (p *paste) sqlToCommonPaste(modelPaste models.Paste) params.Paste {
	return params.Paste{
		ID:        modelPaste.ID,
		Data:      string(modelPaste.Data),
		Name:      modelPaste.Name,
		Expires:   modelPaste.Expires,
		Public:    modelPaste.Public,
		CreatedAt: modelPaste.CreatedAt,
	}
}

func (p *paste) Create(ctx context.Context, data string, expires time.Time, isPublic bool, title string) (paste params.Paste, err error) {
	pasteID, err := util.GetRandomString(24)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "getting random string")
	}
	if auth.IsAnonymous(ctx) || auth.IsEnabled(ctx) == false {
		return params.Paste{}, auth.ErrUnauthorized
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
		Public:    isPublic,
		Name:      title,
	}
	q := p.conn.Create(&newPaste)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "creating paste")
	}
	return p.sqlToCommonPaste(newPaste), nil
}

func (p *paste) Get(ctx context.Context, pasteID string) (paste params.Paste, err error) {
	return params.Paste{}, nil
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
