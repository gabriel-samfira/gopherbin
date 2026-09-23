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
	"fmt"
	"math"
	"strings"
	"time"

	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pkg/errors"
)

// NewPaster returns a SQL backed paste implementation
func NewPaster(dbCfg config.Database) (common.Paster, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}

	p := &paste{
		conn:      db,
		dbBackend: dbCfg.DbBackend,
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
	conn      *gorm.DB
	dbBackend config.DBBackendType
	teamMgr   *teamManager
}

func (p *paste) migrateDB() error {
	if err := p.conn.AutoMigrate(
		&models.Users{},
		&models.Paste{},
		&models.Teams{},
		&models.TeamUser{},
		&models.Label{},
		&models.JWTBacklist{},
	); err != nil {
		return err
	}

	// Setup full-text search based on database backend
	switch p.dbBackend {
	case config.SQLiteBackend:
		// Create FTS5 virtual table for SQLite full-text search
		var count int64
		if err := p.conn.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='pastes_fts'").Scan(&count).Error; err != nil {
			return errors.Wrap(err, "checking for FTS5 table existence")
		}

		if count == 0 {
			// Create FTS5 virtual table
			if err := p.conn.Exec(`
				CREATE VIRTUAL TABLE IF NOT EXISTS pastes_fts USING fts5(
					paste_id UNINDEXED,
					name,
					data,
					content=pastes,
					content_rowid=id
				)
			`).Error; err != nil {
				return errors.Wrap(err, "creating FTS5 virtual table")
			}

			// Create triggers to keep FTS table in sync
			// Drop triggers if they exist (for idempotency)
			p.conn.Exec("DROP TRIGGER IF EXISTS pastes_ai")
			p.conn.Exec("DROP TRIGGER IF EXISTS pastes_ad")
			p.conn.Exec("DROP TRIGGER IF EXISTS pastes_au")

			if err := p.conn.Exec(`
				CREATE TRIGGER pastes_ai AFTER INSERT ON pastes BEGIN
					INSERT INTO pastes_fts(rowid, paste_id, name, data)
					VALUES (new.id, new.paste_id, new.name, new.data);
				END
			`).Error; err != nil {
				return errors.Wrap(err, "creating FTS5 insert trigger")
			}

			if err := p.conn.Exec(`
				CREATE TRIGGER pastes_ad AFTER DELETE ON pastes BEGIN
					INSERT INTO pastes_fts(pastes_fts, rowid, paste_id, name, data)
					VALUES('delete', old.id, old.paste_id, old.name, old.data);
				END
			`).Error; err != nil {
				return errors.Wrap(err, "creating FTS5 delete trigger")
			}

			if err := p.conn.Exec(`
				CREATE TRIGGER pastes_au AFTER UPDATE ON pastes BEGIN
					INSERT INTO pastes_fts(pastes_fts, rowid, paste_id, name, data)
					VALUES('delete', old.id, old.paste_id, old.name, old.data);
					INSERT INTO pastes_fts(rowid, paste_id, name, data)
					VALUES (new.id, new.paste_id, new.name, new.data);
				END
			`).Error; err != nil {
				return errors.Wrap(err, "creating FTS5 update trigger")
			}

			// Populate existing data (only if there are pastes)
			var pasteCount int64
			if err := p.conn.Model(&models.Paste{}).Count(&pasteCount).Error; err != nil {
				return errors.Wrap(err, "counting existing pastes")
			}

			if pasteCount > 0 {
				if err := p.conn.Exec(`
					INSERT INTO pastes_fts(rowid, paste_id, name, data)
					SELECT id, paste_id, name, data FROM pastes
				`).Error; err != nil {
					return errors.Wrap(err, "populating FTS5 table with existing data")
				}
			}
		}

	case config.MySQLBackend:
		// Create FULLTEXT indexes for MySQL
		// Check if indexes already exist
		var indexCount int64
		if err := p.conn.Raw(`
			SELECT COUNT(*)
			FROM information_schema.STATISTICS
			WHERE table_schema = DATABASE()
			AND table_name = 'pastes'
			AND index_name = 'idx_pastes_fulltext'
		`).Scan(&indexCount).Error; err != nil {
			return errors.Wrap(err, "checking for MySQL FULLTEXT index existence")
		}

		if indexCount == 0 {
			// Create FULLTEXT index on name and data columns
			// Note: This may take time on large tables
			if err := p.conn.Exec(`
				ALTER TABLE pastes
				ADD FULLTEXT INDEX idx_pastes_fulltext (name, data)
			`).Error; err != nil {
				// Log warning but don't fail - LIKE search will still work
				// FULLTEXT requires InnoDB in MySQL 5.6+ or MyISAM
				fmt.Printf("Warning: Failed to create FULLTEXT index (will use LIKE search): %v\n", err)
			}
		}
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

// incrementAndMaybeDestroy increments the access counter and, if the paste has
// reached its access limit, hard-deletes it. Must be called inside a transaction.
func (p *paste) incrementAndMaybeDestroy(tx *gorm.DB, pst *models.Paste) error {
	if err := tx.Model(pst).UpdateColumn(
		"access_count", gorm.Expr("access_count + 1"),
	).Error; err != nil {
		return errors.Wrap(err, "incrementing access count")
	}
	pst.AccessCount++
	if pst.AccessCount >= *pst.MaxAccesses {
		if err := tx.Unscoped().Delete(pst).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Wrap(err, "deleting exhausted paste")
		}
	}
	return nil
}

// sqlToCommonPaste renders a paste for a viewer. viewerID 0 means anonymous:
// personal labels are then omitted, and they are also hidden from anyone who is
// not the paste owner.
func (p *paste) sqlToCommonPaste(modelPaste models.Paste, withPreview bool, viewerID uint) params.Paste {
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
		MaxAccesses: modelPaste.MaxAccesses,
		AccessCount: modelPaste.AccessCount,
		CreatedBy:   modelPaste.Owner.FullName,
		Owner:       modelPaste.Owner.Username,
		OwnerID:     modelPaste.OwnerID,
		Team:        modelPaste.Team.Name,
		Metadata:    metadata,
	}
	for _, label := range modelPaste.Labels {
		if label.TeamID != nil {
			paste.Labels = append(paste.Labels, params.PasteLabel{Name: label.Name, Color: label.Color, Scope: "team", Team: modelPaste.Team.Name})
		} else if viewerID != 0 && viewerID == modelPaste.OwnerID {
			paste.Labels = append(paste.Labels, params.PasteLabel{Name: label.Name, Color: label.Color, Scope: "personal"})
		}
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
	metadata map[string]string,
	maxAccesses *int,
	labels []string) (paste params.Paste, err error) {

	pasteID, err := util.GetRandomString(24)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "getting random string")
	}

	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching user")
	}
	if len(data) == 0 || len(title) == 0 {
		return params.Paste{}, gErrors.ErrBadRequest
	}

	var teamID *uint
	if team != "" {
		teamModel, err := p.teamMgr.get(team)
		if err != nil {
			return params.Paste{}, errors.Wrap(err, "fetching team")
		}
		if !p.teamMgr.canShareToTeam(teamModel, user) {
			return params.Paste{}, errors.Wrap(gErrors.ErrUnauthorized, "creating paste for foreign team")
		}
		teamID = &teamModel.ID
		// Team pastes are never public: access is governed by team membership.
		isPublic = false
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
		MaxAccesses: maxAccesses,
		TeamID:      teamID,
	}
	cleanLabels, err := dedupeLabels(labels)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "validating labels")
	}

	q := p.conn.Create(&newPaste)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "creating paste")
	}
	if len(cleanLabels) > 0 {
		labelOwnerID, labelTeamID := user.ID, uint(0)
		if teamID != nil {
			labelOwnerID, labelTeamID = 0, *teamID
		}
		err = p.conn.Transaction(func(tx *gorm.DB) error {
			resolved, err := resolveOrCreateLabels(tx, cleanLabels, labelOwnerID, labelTeamID)
			if err != nil {
				return err
			}
			return tx.Model(&newPaste).Association("Labels").Replace(resolved)
		})
		if err != nil {
			return params.Paste{}, errors.Wrap(err, "attaching labels")
		}
	}
	if newPaste.TeamID != nil {
		teamModel, err := p.teamMgr.get(team)
		if err != nil {
			return params.Paste{}, errors.Wrap(err, "fetching team")
		}
		newPaste.Team = teamModel
	}
	return p.sqlToCommonPaste(newPaste, false, user.ID), nil
}

func (p *paste) canAccess(paste models.Paste, user models.Users) bool {
	if paste.Public {
		return true
	}

	// The user is the owner of the paste
	if paste.Owner.ID == user.ID {
		return true
	}

	// This paste belongs to a team, and the user
	// is the owner of the team.
	if paste.TeamID != nil && paste.Team.OwnerID == user.ID {
		return true
	}

	// Check if the paste is shared with the user.
	for _, usr := range paste.Users {
		if usr.ID == user.ID {
			return true
		}
	}

	// Check if the paste belongs to a team that the user has joined
	// (pending invitees do not get access).
	if paste.TeamID != nil && p.teamMgr.isActiveMember(*paste.TeamID, user.ID) {
		return true
	}

	return false
}

// canManage returns true if the user may mutate (delete, change privacy of,
// transfer) the paste. Only the owner of the paste, or the owner of the team
// that owns the paste may manage it.
func (p *paste) canManage(paste models.Paste, user models.Users) bool {
	if paste.OwnerID == user.ID {
		return true
	}
	if paste.TeamID != nil && paste.Team.OwnerID == user.ID {
		return true
	}
	return false
}

// loadPaste fetches a paste by pasteID with all relations needed for
// authorization decisions. Unlike getPaste it does not increment the access
// counter, so it can be used by mutation endpoints without side effects.
func (p *paste) loadPaste(pasteID string) (models.Paste, error) {
	var tmpPaste models.Paste
	q := p.conn.Preload("Owner").Preload("Users").Preload("Team").Preload("Team.Owner").Preload("Labels").
		Where("paste_id = ? and (expires is NULL or expires >= ?)", pasteID, time.Now()).First(&tmpPaste)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Paste{}, gErrors.ErrNotFound
		}
		return models.Paste{}, errors.Wrap(q.Error, "fetching paste from database")
	}
	return tmpPaste, nil
}

func (p *paste) GetPublicPaste(ctx context.Context, pasteID string) (params.Paste, error) {
	var tmpPaste models.Paste
	err := p.conn.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Labels").Preload("Team").Where(
			"paste_id = ? and (expires is NULL or expires >= ?) and public = ?", pasteID, now, true).First(&tmpPaste)
		if q.Error != nil {
			if errors.Is(q.Error, gorm.ErrRecordNotFound) {
				return gErrors.ErrNotFound
			}
			return errors.Wrap(q.Error, "fetching paste from database")
		}
		if tmpPaste.MaxAccesses != nil {
			return p.incrementAndMaybeDestroy(tx, &tmpPaste)
		}
		return nil
	})
	if err != nil {
		return params.Paste{}, err
	}
	return p.sqlToCommonPaste(tmpPaste, false, 0), nil
}

func (p *paste) getPaste(pasteID string, user models.Users) (models.Paste, error) {
	var tmpPaste models.Paste
	err := p.conn.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Users").Preload("Owner").Preload("Team").Preload("Labels").Where(
			"paste_id = ? and (expires is NULL or expires >= ?)", pasteID, now).First(&tmpPaste)
		if q.Error != nil {
			if errors.Is(q.Error, gorm.ErrRecordNotFound) {
				return gErrors.ErrNotFound
			}
			return errors.Wrap(q.Error, "fetching paste from database")
		}
		if canAccess := p.canAccess(tmpPaste, user); !canAccess {
			return gErrors.ErrNotFound
		}
		if tmpPaste.MaxAccesses != nil {
			return p.incrementAndMaybeDestroy(tx, &tmpPaste)
		}
		return nil
	})
	if err != nil {
		return models.Paste{}, err
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
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching user from DB")
	}
	pst, err := p.getPaste(pasteID, user)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching paste")
	}
	return p.sqlToCommonPaste(pst, false, user.ID), nil
}

// scopeClause returns a SQL condition (and its bind values) limiting results
// to the pastes visible to the given user, according to the requested scope:
//   - ScopeMine:   pastes the user owns
//   - ScopeShared: pastes shared with the user directly, or visible through a
//     team the user is a member or owner of
//   - ScopeAll:    the union of the above
func (p *paste) scopeClause(user models.Users, scope string) (string, []interface{}) {
	sharedCond := `(owner_id != ? AND (EXISTS (SELECT 1 FROM paste_users WHERE paste_users.paste_id = pastes.id AND paste_users.users_id = ?) OR (team_id IS NOT NULL AND (team_id IN (SELECT teams_id FROM team_users WHERE team_users.users_id = ? AND team_users.status = 'active') OR team_id IN (SELECT id FROM teams WHERE teams.owner_id = ?)))))`

	switch scope {
	case common.ScopeMine:
		return "owner_id = ?", []interface{}{user.ID}
	case common.ScopeShared:
		return sharedCond, []interface{}{user.ID, user.ID, user.ID, user.ID}
	default: // ScopeAll
		cond := `(owner_id = ? OR EXISTS (SELECT 1 FROM paste_users WHERE paste_users.paste_id = pastes.id AND paste_users.users_id = ?) OR (team_id IS NOT NULL AND (team_id IN (SELECT teams_id FROM team_users WHERE team_users.users_id = ? AND team_users.status = 'active') OR team_id IN (SELECT id FROM teams WHERE teams.owner_id = ?))))`
		return cond, []interface{}{user.ID, user.ID, user.ID, user.ID}
	}
}

// viewerTeamIDs returns the IDs of teams whose labels the viewer may use or
// see: teams they actively belong to, plus teams they own.
func (p *paste) viewerTeamIDs(user models.Users) []uint {
	var ids []uint
	p.conn.Model(&models.TeamUser{}).
		Where("users_id = ? AND status = ?", user.ID, models.TeamMembershipActive).
		Pluck("teams_id", &ids)
	var owned []uint
	p.conn.Model(&models.Teams{}).Where("owner_id = ?", user.ID).Pluck("id", &owned)
	return append(ids, owned...)
}

// labelFilter builds AND-joined EXISTS conditions: every requested label name
// must be attached to the paste, either as the viewer's own personal label or
// as a label of one of the viewer's teams. Same-named labels belonging to
// foreign teams are not matched.
func (p *paste) labelFilter(user models.Users, names []string) (string, []interface{}) {
	if len(names) == 0 {
		return "", nil
	}
	teamIDs := p.viewerTeamIDs(user)
	conds := make([]string, 0, len(names))
	args := make([]interface{}, 0, len(names)*(1+len(teamIDs)))
	for _, name := range names {
		if len(teamIDs) > 0 {
			placeholders := strings.Repeat("?, ", len(teamIDs)-1) + "?"
			conds = append(conds, `(EXISTS (SELECT 1 FROM paste_labels pl JOIN labels l ON l.id = pl.label_id
				WHERE pl.paste_id = pastes.id AND l.name = ? AND ((l.team_id IS NULL AND l.owner_user_id = ?) OR l.team_id IN (`+placeholders+`))))`)
			args = append(args, name, user.ID)
			for _, id := range teamIDs {
				args = append(args, id)
			}
		} else {
			conds = append(conds, `(EXISTS (SELECT 1 FROM paste_labels pl JOIN labels l ON l.id = pl.label_id
				WHERE pl.paste_id = pastes.id AND l.name = ? AND l.team_id IS NULL AND l.owner_user_id = ?))`)
			args = append(args, name, user.ID)
		}
	}
	return strings.Join(conds, " AND "), args
}

// teamFilter limits results to pastes of one named team. The scope clause
// still applies, so teams the viewer cannot see simply match nothing.
func (p *paste) teamFilter(team string) (string, []interface{}) {
	if team == "" {
		return "", nil
	}
	return "(team_id IN (SELECT id FROM teams WHERE name = ?))", []interface{}{team}
}

// mergeConds joins optional WHERE fragments with AND.
func mergeConds(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, "("+part+")")
		}
	}
	return strings.Join(kept, " AND ")
}

func (p *paste) Search(ctx context.Context, query string, page int64, results int64, scope string, labels []string, team string) (params.PasteListResult, error) {
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

	scopeCond, scopeArgs := p.scopeClause(user, scope)

	// Build full-text search query based on database backend
	var q *gorm.DB
	searchPattern := "%" + query + "%"

	switch p.dbBackend {
	case config.MySQLBackend:
		// MySQL: Try to use FULLTEXT search if index exists, fallback to LIKE
		// Check if FULLTEXT index exists
		var indexCount int64
		p.conn.Raw(`
			SELECT COUNT(*)
			FROM information_schema.STATISTICS
			WHERE table_schema = DATABASE()
			AND table_name = 'pastes'
			AND index_name = 'idx_pastes_fulltext'
		`).Scan(&indexCount)

		if indexCount > 0 {
			// Use FULLTEXT search with MATCH...AGAINST
			// IN BOOLEAN MODE allows for more flexible searching
			q = p.conn.Select(
				"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, substr(`data`, 1, 512) as data",
			).Where(
				scopeCond+" AND MATCH(name, `data`) AGAINST(? IN BOOLEAN MODE) AND (expires IS NULL OR expires >= ?)",
				append(scopeArgs, query, now)...,
			).Order("id desc")
		} else {
			// Fallback to LIKE search
			q = p.conn.Select(
				"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, substr(`data`, 1, 512) as data",
			).Where(
				scopeCond+" AND (name LIKE ? OR `data` LIKE ?) AND (expires IS NULL OR expires >= ?)",
				append(scopeArgs, searchPattern, searchPattern, now)...,
			).Order("id desc")
		}

	case config.SQLiteBackend:
		// SQLite: Use FTS5 for full-text search
		// Join with FTS table and use MATCH for efficient full-text search
		q = p.conn.Table("pastes").
			Select(
				"pastes.id, pastes.paste_id, pastes.language, pastes.name, pastes.description, pastes.metadata, pastes.owner_id, pastes.team_id, pastes.created_at, pastes.expires, pastes.public, substr(pastes.`data`, 1, 512) as data",
			).
			Joins("INNER JOIN pastes_fts ON pastes.id = pastes_fts.rowid").
			Where("pastes_fts MATCH ? AND "+scopeCond+" AND (pastes.expires IS NULL OR pastes.expires >= ?)", append([]interface{}{query}, append(scopeArgs, now)...)...).
			Order("pastes.id desc")

	default:
		// Default fallback: search only in name
		q = p.conn.Select(
			"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, substr(`data`, 1, 512) as data",
		).Where(scopeCond+" and name LIKE ? and (expires is NULL or expires >= ?)", append(scopeArgs, searchPattern, now)...).Order("id desc")
	}

	cleanLabels, err := dedupeLabels(labels)
	if err != nil {
		return params.PasteListResult{}, errors.Wrap(err, "validating label filter")
	}
	if labelCond, labelArgs := p.labelFilter(user, cleanLabels); labelCond != "" {
		q = q.Where(labelCond, labelArgs...)
	}
	if teamCond, teamArgs := p.teamFilter(team); teamCond != "" {
		q = q.Where(teamCond, teamArgs...)
	}

	cntQ := q.Model(&models.Paste{}).Count(&cnt)
	if cntQ.Error != nil {
		return params.PasteListResult{}, errors.Wrap(cntQ.Error, "counting results")
	}

	resQ := q.Preload("Owner").Preload("Team").Preload("Labels").Offset(int(startFrom)).Limit(int(results)).Find(&pasteResults)
	if resQ.Error != nil {
		if errors.Is(resQ.Error, gorm.ErrRecordNotFound) {
			return params.PasteListResult{}, gErrors.ErrNotFound
		}
		return params.PasteListResult{}, errors.Wrap(resQ.Error, "fetching pastes from database")
	}

	asParams := make([]params.Paste, len(pasteResults))
	for idx, val := range pasteResults {
		asParams[idx] = p.sqlToCommonPaste(val, true, user.ID)
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

func (p *paste) Delete(ctx context.Context, pasteID string) error {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user")
	}
	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return errors.Wrap(err, "fetching paste")
	}
	if !p.canManage(pst, user) {
		return gErrors.ErrUnauthorized
	}
	if pst.PasteID == "" {
		return nil
	}
	err = p.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&pst).Association("Users").Clear(); err != nil {
			return errors.Wrap(err, "clearing paste shares")
		}
		if err := tx.Model(&pst).Association("Labels").Clear(); err != nil {
			return errors.Wrap(err, "clearing paste labels")
		}
		if err := tx.Unscoped().Delete(&pst).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Wrap(err, "deleting paste")
		}
		return nil
	})
	return err
}

func (p *paste) List(ctx context.Context, page int64, results int64, scope string, labels []string, team string) (paste params.PasteListResult, err error) {
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

	scopeCond, scopeArgs := p.scopeClause(user, scope)
	teamCond, teamArgs := p.teamFilter(team)
	cleanLabels, err := dedupeLabels(labels)
	if err != nil {
		return params.PasteListResult{}, errors.Wrap(err, "validating label filter")
	}
	labelCond, labelArgs := p.labelFilter(user, cleanLabels)

	// List will return only a small preview of the paste data (first 512 bytes).
	cond := mergeConds(scopeCond, teamCond, labelCond)
	args := append(append(append([]interface{}{}, scopeArgs...), teamArgs...), labelArgs...)
	q := p.conn.Select(
		"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, substr(`data`, 1, 512) as data",
	).Where(cond+" and (expires is NULL or expires >= ?)", append(args, now)...).Order("id desc")

	cntQ := q.Model(&models.Paste{}).Count(&cnt)
	if cntQ.Error != nil {
		return params.PasteListResult{}, errors.Wrap(cntQ.Error, "counting results")
	}

	resQ := q.Preload("Owner").Preload("Team").Preload("Labels").Offset(int(startFrom)).Limit(int(results)).Find(&pasteResults)
	if resQ.Error != nil {
		if errors.Is(resQ.Error, gorm.ErrRecordNotFound) {
			return params.PasteListResult{}, gErrors.ErrNotFound
		}
		return params.PasteListResult{}, errors.Wrap(resQ.Error, "fetching pastes from database")
	}

	asParams := make([]params.Paste, len(pasteResults))
	for idx, val := range pasteResults {
		asParams[idx] = p.sqlToCommonPaste(val, true, user.ID)
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
	ctxUser, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching user")
	}

	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching paste")
	}
	if !p.canAccess(pst, ctxUser) {
		return params.TeamMember{}, gErrors.ErrNotFound
	}
	if pst.OwnerID != ctxUser.ID {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrUnauthorized, "sharing foreign paste")
	}

	if pst.TeamID != nil {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrBadRequest, "team pastes are shared with the whole team")
	}

	targetUser, err := p.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "finding user")
	}
	if targetUser.ID == pst.OwnerID {
		return params.TeamMember{}, gErrors.NewBadRequestError("cannot share a paste with its owner")
	}

	if err := p.conn.Model(&pst).Association("Users").Append(&targetUser); err != nil {
		return params.TeamMember{}, errors.Wrap(err, "sharing with user")
	}
	return sqlUserToTeamMember(targetUser), nil
}

func (p *paste) UnshareWithUser(ctx context.Context, pasteID string, userID string) error {
	ctxUser, err := p.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user")
	}

	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return errors.Wrap(err, "fetching paste")
	}
	if pst.OwnerID != ctxUser.ID {
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
	ctxUser, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.PasteShareListResponse{}, errors.Wrap(err, "fetching user")
	}

	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return params.PasteShareListResponse{}, errors.Wrap(err, "fetching paste")
	}
	if !p.canManage(pst, ctxUser) {
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
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching user")
	}
	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching paste")
	}
	if !p.canManage(pst, user) {
		return params.Paste{}, gErrors.ErrUnauthorized
	}
	if pst.TeamID != nil && public {
		return params.Paste{}, gErrors.NewBadRequestError("team pastes cannot be made public")
	}
	pst.Public = public
	q := p.conn.Save(&pst)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "saving paste to DB")
	}
	return p.sqlToCommonPaste(pst, false, user.ID), nil
}

// TransferOwnership transfers a paste to another user. Team pastes can only be
// transferred within the team.
func (p *paste) TransferOwnership(ctx context.Context, pasteID string, userID string) (params.Paste, error) {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching user")
	}
	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching paste")
	}
	if !p.canManage(pst, user) {
		return params.Paste{}, gErrors.ErrUnauthorized
	}

	targetUser, err := p.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "finding target user")
	}
	if targetUser.ID == pst.OwnerID {
		return params.Paste{}, gErrors.NewBadRequestError("paste is already owned by this user")
	}
	if pst.TeamID != nil && !p.teamMgr.isMember(pst.Team, targetUser) {
		return params.Paste{}, gErrors.NewBadRequestError("team pastes can only be transferred to team members")
	}

	pst.OwnerID = targetUser.ID
	pst.Owner = targetUser
	q := p.conn.Save(&pst)
	if q.Error != nil {
		return params.Paste{}, errors.Wrap(q.Error, "saving paste to DB")
	}
	return p.sqlToCommonPaste(pst, false, user.ID), nil
}

// SetLabels replaces the labels attached to a paste. Personal pastes accept
// the owner's personal labels; team pastes accept labels of the paste's
// team. Labels are created on first use.
func (p *paste) SetLabels(ctx context.Context, pasteID string, names []string) (params.Paste, error) {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching user")
	}
	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "fetching paste")
	}
	if !p.canManage(pst, user) {
		return params.Paste{}, gErrors.ErrUnauthorized
	}
	clean, err := dedupeLabels(names)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "validating labels")
	}

	labelOwnerID, labelTeamID := user.ID, uint(0)
	if pst.TeamID != nil {
		labelOwnerID, labelTeamID = 0, *pst.TeamID
	}
	if err := p.conn.Transaction(func(tx *gorm.DB) error {
		resolved, err := resolveOrCreateLabels(tx, clean, labelOwnerID, labelTeamID)
		if err != nil {
			return err
		}
		return tx.Model(&pst).Association("Labels").Replace(resolved)
	}); err != nil {
		return params.Paste{}, errors.Wrap(err, "setting paste labels")
	}
	pst.Labels = nil
	if err := p.conn.Model(&pst).Association("Labels").Find(&pst.Labels); err != nil {
		return params.Paste{}, errors.Wrap(err, "reloading paste labels")
	}
	return p.sqlToCommonPaste(pst, false, user.ID), nil
}

// ListLabels returns the label vocabulary visible to the viewer: their own
// personal labels and the labels of teams they belong to or own.
func (p *paste) ListLabels(ctx context.Context) (params.LabelVocabulary, error) {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.LabelVocabulary{}, errors.Wrap(err, "fetching user")
	}
	out := params.LabelVocabulary{Personal: []string{}, Teams: []params.TeamLabelGroup{}, Colors: map[string]string{}}

	var personal []models.Label
	if err := p.conn.Where("owner_user_id = ? AND team_id IS NULL", user.ID).Order("name ASC").Find(&personal).Error; err != nil {
		return params.LabelVocabulary{}, errors.Wrap(err, "fetching personal labels")
	}
	for _, l := range personal {
		out.Personal = append(out.Personal, l.Name)
		if l.Color != "" {
			out.Colors[l.Name] = l.Color
		}
	}

	teamIDs := p.viewerTeamIDs(user)
	if len(teamIDs) == 0 {
		return out, nil
	}
	var teams []models.Teams
	if err := p.conn.Where("id IN ?", teamIDs).Order("name ASC").Find(&teams).Error; err != nil {
		return params.LabelVocabulary{}, errors.Wrap(err, "fetching viewer teams")
	}
	teamNames := map[uint]string{}
	for _, tm := range teams {
		teamNames[tm.ID] = tm.Name
	}
	var teamLabels []models.Label
	if err := p.conn.Where("team_id IN ?", teamIDs).Order("name ASC").Find(&teamLabels).Error; err != nil {
		return params.LabelVocabulary{}, errors.Wrap(err, "fetching team labels")
	}
	grouped := map[string]*params.TeamLabelGroup{}
	order := []string{}
	for _, l := range teamLabels {
		name := teamNames[*l.TeamID]
		group, ok := grouped[name]
		if !ok {
			group = &params.TeamLabelGroup{Team: name, Labels: []string{}}
			grouped[name] = group
			order = append(order, name)
		}
		group.Labels = append(group.Labels, l.Name)
		if l.Color != "" {
			out.Colors[fmt.Sprintf("%s:%s", name, l.Name)] = l.Color
			if _, exists := out.Colors[l.Name]; !exists {
				out.Colors[l.Name] = l.Color
			}
		}
	}
	for _, name := range order {
		out.Teams = append(out.Teams, *grouped[name])
	}
	return out, nil
}
