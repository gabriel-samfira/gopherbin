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

package sql

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"

	"gorm.io/gorm"

	"github.com/pkg/errors"
)

// NewPaster returns a SQL backed paste implementation
func NewPaster(dbCfg config.Database) (common.Paster, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}

	p := &paste{
		conn: db,
		teamMgr: &teamManager{
			conn: db,
		},
	}
	if err := p.migrateDB(); err != nil {
		return nil, errors.Wrap(err, "migrating DB")
	}
	return p, nil
}

type paste struct {
	conn    *gorm.DB
	teamMgr common.TeamManager
}

func (p *paste) migrateDB() error {
	if err := p.conn.AutoMigrate(
		&models.Users{},
		&models.Paste{},
		&models.Teams{},
		&models.JWTBacklist{},
	); err != nil {
		return err
	}

	return nil
}

func (p *paste) getUserFromContext(ctx context.Context) (models.Users, error) {
	if auth.IsAnonymous(ctx) || !auth.IsEnabled(ctx) {
		return models.Users{}, gErrors.ErrUnauthorized
	}
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return models.Users{}, errors.Wrap(err, "fetching user")
	}
	return user, nil
}

func (p *paste) getUser(userID uint) (models.Users, error) {
	// TODO: abstract this into a common interface
	var tmpUser models.Users
	q := p.conn.Preload("MemberOf").Where("id = ?", userID).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (p *paste) getUserByUsernameOrEmail(userID string) (models.Users, error) {
	isEmail := util.IsValidEmail(userID)
	var tmpUser models.Users
	queryString := "username = ?"
	if isEmail {
		queryString = "email = ?"
	}

	q := p.conn.Preload("MemberOf").Where(queryString, userID).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
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
		CreatedAt:   modelPaste.CreatedAt,
		Expires:     modelPaste.Expires,
		CreatedBy:   modelPaste.Owner.FullName,
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
	isPublic bool, team string,
	metadata map[string]string) (paste params.Paste, err error) {

	pasteID, err := util.GetRandomString(24)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "getting random string")
	}

	user, err := p.getUserFromContext(ctx)
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
		Owner:       user,
		CreatedAt:   time.Now(),
		Data:        data,
		Expires:     expires,
		Language:    language,
		Public:      isPublic,
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

func (p *paste) canAccess(paste models.Paste, user models.Users) bool {
	if paste.Public {
		return true
	}

	// The user is the owner of the team
	if paste.Owner.ID == user.ID {
		return true
	}

	// This paste belongs to a team, and the user
	// is the owner of the team.
	if paste.Team.OwnerID == user.ID {
		return true
	}

	// Check if the paste is shared with the user.
	for _, usr := range paste.Users {
		if usr.ID == user.ID {
			return true
		}
	}

	// Check if the paste belongs to a team that the user
	// is a member of.
	for _, team := range user.MemberOf {
		if team.ID == paste.Team.ID {
			return true
		}
	}

	return false
}

func (p *paste) GetPublicPaste(ctx context.Context, pasteID string) (params.Paste, error) {
	var tmpPaste models.Paste
	now := time.Now()
	q := p.conn.Where(
		"paste_id = ? and (expires is NULL or expires >= ?) and public = ?", pasteID, now, true).First(&tmpPaste)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return params.Paste{}, gErrors.ErrNotFound
		}
		return params.Paste{}, errors.Wrap(q.Error, "fetching paste from database")
	}
	return p.sqlToCommonPaste(tmpPaste, false), nil
}

func (p *paste) getPaste(pasteID string, user models.Users) (models.Paste, error) {
	var tmpPaste models.Paste
	now := time.Now()
	q := p.conn.Preload("Users").Preload("Owner").Where(
		"paste_id = ? and (expires is NULL or expires >= ?)", pasteID, now).First(&tmpPaste)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Paste{}, gErrors.ErrNotFound
		}
		return models.Paste{}, errors.Wrap(q.Error, "fetching paste from database")
	}
	if canAccess := p.canAccess(tmpPaste, user); !canAccess {
		return models.Paste{}, gErrors.ErrNotFound
	}
	return tmpPaste, nil
}

func (p *paste) get(ctx context.Context, pasteID string) (models.Paste, error) {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return models.Paste{}, errors.Wrap(err, "fetching user from DB")
	}
	pst, err := p.getPaste(pasteID, user)
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
	if q.Error != nil && !errors.Is(q.Error, gorm.ErrRecordNotFound) {
		return errors.Wrap(q.Error, "deleting paste")
	}
	return nil
}

func (p *paste) List(ctx context.Context, page int64, results int64) (paste params.PasteListResult, err error) {
	user, err := p.getUserFromContext(ctx)
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
	q := p.conn.Select(
		"id, paste_id, language, name, description, metadata, owner_id as owner, created_at, expires, public, substr(`data`, 1, 512) as data",
	).Where("owner_id = ? and (expires is NULL or expires >= ?)", user.ID, now).Order("id desc")

	cntQ := q.Model(&models.Paste{}).Count(&cnt)
	if cntQ.Error != nil {
		return params.PasteListResult{}, errors.Wrap(cntQ.Error, "counting results")
	}

	resQ := q.Offset(int(startFrom)).Limit(int(results)).Find(&pasteResults)
	if resQ.Error != nil {
		if errors.Is(resQ.Error, gorm.ErrRecordNotFound) {
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
		Pastes:     asParams,
		TotalPages: totalPages,
		Page:       page,
	}, nil
}

func (p *paste) ShareWithUser(ctx context.Context, pasteID string, userID string) (params.TeamMember, error) {
	ctxUserID := auth.UserID(ctx)

	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching paste")
	}
	if pst.Owner.ID != ctxUserID {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrUnauthorized, "sharing foreign paste")
	}

	if pst.Team.ID != 0 {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrBadRequest, "sharing team paste")
	}

	targetUser, err := p.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "finding user")
	}

	if err := p.conn.Model(&pst).Association("Users").Append(&targetUser); err != nil {
		return params.TeamMember{}, errors.Wrap(err, "sharing with user")
	}
	return sqlUserToTeamMember(targetUser), nil
}

func (p *paste) UnshareWithUser(ctx context.Context, pasteID string, userID string) error {
	ctxUserID := auth.UserID(ctx)

	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return errors.Wrap(err, "fetching paste")
	}
	if pst.Owner.ID != ctxUserID {
		return errors.Wrap(gErrors.ErrUnauthorized, "unsharing foreign paste")
	}

	targetUser, err := p.getUserByUsernameOrEmail(userID)
	if err != nil {
		return errors.Wrap(err, "finding user")
	}

	if err := p.conn.Model(&pst).Association("Users").Delete(targetUser); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Wrap(err, "unsharing with user")
	}
	return nil
}

func (p *paste) ListShares(ctx context.Context, pasteID string) (params.PasteShareListResponse, error) {
	ctxUserID := auth.UserID(ctx)

	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return params.PasteShareListResponse{}, errors.Wrap(err, "fetching paste")
	}
	if pst.Owner.ID != ctxUserID {
		return params.PasteShareListResponse{}, errors.Wrap(gErrors.ErrUnauthorized, "listing shares of foreign paste")
	}

	var shares []models.Users
	if err := p.conn.Model(&pst).Association("Users").Find(&shares); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return params.PasteShareListResponse{}, nil
		}
		return params.PasteShareListResponse{}, errors.Wrap(err, "unsharing with user")
	}

	ret := make([]params.TeamMember, len(shares))
	for idx, val := range shares {
		ret[idx] = sqlUserToTeamMember(val)
	}
	return params.PasteShareListResponse{
		Users: ret,
	}, nil
}

func (p *paste) SetPrivacy(ctx context.Context, pasteID string, public bool) (params.Paste, error) {
	pst, err := p.get(ctx, pasteID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching paste")
	}
	pst.Public = public
	q := p.conn.Save(&pst)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "saving paste to DB")
	}
	return p.sqlToCommonPaste(pst, true), nil
}
