// Copyright 2019 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"

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
		&models.JWTBacklist{},
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
	q := p.conn.Preload("Teams").Preload("CreatedTeams").Where("id = ?", userID).First(&tmpUser)
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
	now := time.Now()
	q := p.conn.Preload("Teams").Preload("Users").Where(
		"paste_id = ? and (expires is NULL or expires >= ?)", pasteID, now).First(&tmpPaste)
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

func (p *paste) sqlToCommonPaste(modelPaste models.Paste, withPreview bool) params.Paste {
	metadata := make(map[string]string)
	if modelPaste.Metadata != nil {
		err := json.Unmarshal(modelPaste.Metadata, &metadata)
		if err != nil {
			metadata = nil
		}
	}

	paste := params.Paste{
		ID:          modelPaste.ID,
		PasteID:     modelPaste.PasteID,
		Language:    modelPaste.Language,
		Name:        modelPaste.Name,
		Description: modelPaste.Description,
		Public:      modelPaste.Public,
		Encrypted:   modelPaste.Encrypted,
		CreatedAt:   modelPaste.CreatedAt,
		Expires:     modelPaste.Expires,
		Metadata:    metadata,
	}
	if withPreview {
		paste.Preview = modelPaste.Data
	} else {
		paste.Data = modelPaste.Data
	}
	return paste
}

func (p *paste) Create(
	ctx context.Context, data []byte,
	title, language, description string,
	expires *time.Time,
	isPublic, encrypted bool,
	metadata map[string]string) (paste params.Paste, err error) {

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
	if len(data) == 0 || len(title) == 0 {
		// TODO: create some custom error types
		return params.Paste{}, gErrors.ErrBadRequest
	}

	var encodedMetadata []byte
	if metadata != nil {
		encodedMetadata, err = json.Marshal(metadata)
		if err != nil {
			return params.Paste{}, errors.Wrap(err, "encoding metadata")
		}
	}

	newPaste := models.Paste{
		PasteID:     pasteID,
		Owner:       user.ID,
		CreatedAt:   time.Now(),
		Data:        data,
		Expires:     expires,
		Language:    language,
		Public:      isPublic,
		Encrypted:   encrypted,
		Name:        title,
		Description: description,
		Metadata:    encodedMetadata,
	}
	q := p.conn.Create(&newPaste)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "creating paste")
	}
	return p.sqlToCommonPaste(newPaste, false), nil
}

func (p *paste) get(ctx context.Context, pasteID string) (models.Paste, error) {
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return models.Paste{}, errors.Wrap(err, fmt.Sprintf("fetching user %v from DB", userID))
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
	return p.sqlToCommonPaste(pst, false), nil
}

func (p *paste) Delete(ctx context.Context, pasteID string) error {
	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return errors.Wrap(err, "fetching paste")
	}
	if pst.PasteID == "" {
		return nil
	}
	q := p.conn.Delete(&pst)
	if q.Error != nil && !q.RecordNotFound() {
		return errors.Wrap(q.Error, "deleting paste")
	}
	return nil
}

func (p *paste) List(ctx context.Context, page int64, results int64) (paste params.PasteListResult, err error) {
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return params.PasteListResult{}, errors.Wrap(err, "fetching user from DB")
	}
	if page == 0 {
		page = 1
	}
	if results == 0 {
		results = 1
	}
	var pasteResults []models.Paste
	var cnt int64
	now := time.Now()
	startFrom := (page - 1) * results
	// List will return only a small preview of the paste data (first 512 bytes).
	q := p.conn.Select("id, paste_id, language, name, description, metadata, owner, created_at, expires, public, LEFT(`data`, 512) as data").Where(
		"owner = ? and (expires is NULL or expires >= ?)",
		user.ID, now).Order("id desc")

	cntQ := q.Debug().Model(&models.Paste{}).Count(&cnt)
	if cntQ.Error != nil {
		return params.PasteListResult{}, errors.Wrap(cntQ.Error, "counting results")
	}

	resQ := q.Debug().Offset(startFrom).Limit(results).Find(&pasteResults)
	if resQ.Error != nil {
		if resQ.RecordNotFound() {
			return params.PasteListResult{}, gErrors.ErrNotFound
		}
		return params.PasteListResult{}, errors.Wrap(resQ.Error, "fetching pastes from database")
	}
	asParams := make([]params.Paste, len(pasteResults))
	for idx, val := range pasteResults {
		asParams[idx] = p.sqlToCommonPaste(val, true)
	}
	totalPages := int64(math.Ceil(float64(cnt) / float64(results)))
	if totalPages == 0 {
		totalPages = 1
	}

	if totalPages < page {
		page = totalPages
	}
	return params.PasteListResult{
		Pastes: asParams,
		// Total:      cnt,
		TotalPages: totalPages,
		Page:       page,
	}, nil
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
