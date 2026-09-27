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

// suspendSQLiteForeignKeys disables foreign key enforcement on the SQLite
// connection pool and returns a function which re-enables it. The pool is
// pinned to a single connection so the PRAGMA (which is per-connection)
// applies to every statement issued while the migration runs.
func suspendSQLiteForeignKeys(conn *gorm.DB) (func(), error) {
	sqlDB, err := conn.DB()
	if err != nil {
		return nil, err
	}
	prev := sqlDB.Stats().MaxOpenConnections
	sqlDB.SetMaxOpenConns(1)
	if err := conn.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		sqlDB.SetMaxOpenConns(prev)
		return nil, err
	}
	return func() {
		if err := conn.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
			fmt.Printf("warning: could not re-enable SQLite foreign keys: %v\n", err)
		}
		sqlDB.SetMaxOpenConns(prev)
	}, nil
}

func (p *paste) migrateDB() error {
	// The SQLite driver adds new foreign key constraints by rebuilding tables
	// with DROP + recreate. With enforcement enabled on the DSN
	// (_foreign_keys=ON), the DROP cascades into dependent rows (pastes and
	// team_users reference teams with ON DELETE CASCADE), silently destroying
	// data during startup migration. Suspend FK enforcement for the migration.
	if p.conn.Dialector.Name() == "sqlite" {
		restore, err := suspendSQLiteForeignKeys(p.conn)
		if err != nil {
			return errors.Wrap(err, "suspending foreign keys for migration")
		}
		defer restore()
	}

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
	q := p.conn.Where("id = ?", userID).First(&tmpUser)
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

	q := p.conn.Where(queryString, userID).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

// consumeAccess atomically consumes one of the paste's remaining accesses.
// The read-modify-write it replaces was unprotected in practice: the sqlite
// driver drops SELECT ... FOR UPDATE (vendor gorm.io/driver/sqlite, "FOR"
// clause builder), so concurrent viewers could interleave and over-serve a
// access-limited paste. The counter is bumped with a single conditional
// UPDATE whose predicate the database evaluates against the current row,
// which is race-free on both sqlite and MySQL.
//
// The UPDATE stays the FIRST statement of its transaction: on SQLite (WAL)
// a write that upgrades a transaction already holding a read snapshot can
// abort immediately with SQLITE_BUSY instead of waiting, so starting the
// transaction with the write keeps that window as small as SQLite allows.
// The remaining aborts are transient by nature and are retried by the
// caller via transactionRetryingOnBusy.
//
// A budget of less than 1 behaves as exactly one remaining view: the legacy
// path incremented unconditionally and destroyed when count >= max, so a
// paste created with max_accesses <= 0 still served its content once before
// destruction; the CASE preserves that observable behavior.
//
// It returns true when a slot was taken, and false when the UPDATE matched
// nothing: either the paste has no access budget (or no longer exists),
// which the caller's follow-up read resolves, or a concurrent viewer just
// exhausted the budget. Must be called inside a transaction.
func (p *paste) consumeAccess(tx *gorm.DB, pasteID string) (bool, error) {
	res := tx.Model(&models.Paste{}).
		Where("paste_id = ? AND max_accesses IS NOT NULL AND access_count < CASE WHEN max_accesses < 1 THEN 1 ELSE max_accesses END", pasteID).
		UpdateColumn("access_count", gorm.Expr("access_count + 1"))
	if res.Error != nil {
		return false, errors.Wrap(res.Error, "consuming access")
	}
	return res.RowsAffected == 1, nil
}

// isTransientLockError reports the SQLite driver messages that signal
// transient write contention (SQLITE_BUSY, including the BUSY_SNAPSHOT
// variant returned without consulting the busy timeout, and SQLITE_LOCKED).
// Message matching is used because the sqlite driver is only an indirect
// dependency; MySQL never produces these strings, so retrying on them is
// inert there.
func isTransientLockError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked")
}

// transactionRetryingOnBusy runs fn inside a database transaction, retrying
// the whole unit of work a bounded number of times when SQLite aborts it
// with transient lock contention. The retry is safe for the access-counter
// transaction: an attempt that failed before committing consumed nothing,
// and the next attempt re-evaluates the conditional UPDATE against the
// freshly committed state, so no sequence of attempts can ever serve more
// viewers than the budget allows.
func (p *paste) transactionRetryingOnBusy(fn func(tx *gorm.DB) error) error {
	const maxAttempts = 24
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = p.conn.Transaction(fn)
		if err == nil || !isTransientLockError(err) {
			return err
		}
		// Linear backoff; the contending transactions are single-statement
		// writes that drain in microseconds, so this stays far below a
		// second even with a long queue of viewers.
		time.Sleep(time.Duration(attempt+1) * 500 * time.Microsecond)
	}
	return errors.Wrap(err, "after retrying on lock contention")
}

// destroyExhaustedPaste hard-deletes a paste whose access budget is spent.
// The viewer whose access exhausted the budget is served from the copy
// loaded before the delete, matching the pre-existing destroy-at-limit
// behavior. Must be called inside a transaction.
func (p *paste) destroyExhaustedPaste(tx *gorm.DB, pst *models.Paste) error {
	if err := tx.Unscoped().Delete(pst).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrap(err, "deleting exhausted paste")
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
		// A preview is content served without consuming an access, so
		// access-limited pastes only preview for their owner; for short
		// pastes the preview is the whole content.
		if modelPaste.MaxAccesses == nil || (viewerID != 0 && viewerID == modelPaste.OwnerID) {
			paste.Preview = modelPaste.Data
		}
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

	var teamModel models.Teams
	var teamID *uint
	if team != "" {
		teamModel, err = p.teamMgr.get(team)
		if err != nil {
			return params.Paste{}, errors.Wrap(err, "fetching team")
		}
		if !p.teamMgr.canShareToTeam(teamModel, user) {
			// The team row was loaded successfully: a 401 here would let any
			// logged-in user enumerate team names through paste creation.
			return params.Paste{}, errors.Wrap(gErrors.ErrNotFound, "creating paste for foreign team")
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

	// Only the foreign keys are set: assigning the Owner/Team structs would
	// make GORM upsert them, together with whatever associations they carry.
	newPaste := models.Paste{
		PasteID:     pasteID,
		OwnerID:     user.ID,
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

	err = p.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&newPaste).Error; err != nil {
			return errors.Wrap(err, "creating paste")
		}
		if len(cleanLabels) == 0 {
			return nil
		}
		labelOwnerID, labelTeamID := user.ID, uint(0)
		if teamID != nil {
			labelOwnerID, labelTeamID = 0, *teamID
		}
		resolved, err := resolveOrCreateLabels(tx, cleanLabels, labelOwnerID, labelTeamID)
		if err != nil {
			return err
		}
		if err := tx.Model(&newPaste).Association("Labels").Replace(resolved); err != nil {
			return errors.Wrap(err, "attaching labels")
		}
		return nil
	})
	if err != nil {
		return params.Paste{}, err
	}
	newPaste.Owner = user
	newPaste.Team = teamModel
	return p.sqlToCommonPaste(newPaste, false, user.ID), nil
}

// canAccess reports whether the user may read the paste. Team pastes are
// governed by team membership alone: the team owner and accepted members
// (not pending invitees) may read them, and authorship does not outlive
// membership, so a member who leaves or is removed loses access to the team
// pastes they created. The paste's Team must be loaded.
func (p *paste) canAccess(paste models.Paste, user models.Users) bool {
	if paste.Public {
		return true
	}

	if paste.TeamID != nil {
		return paste.Team.OwnerID == user.ID || p.teamMgr.isActiveMember(*paste.TeamID, user.ID)
	}

	// The user is the owner of the paste
	if paste.OwnerID == user.ID {
		return true
	}

	// Check if the paste is shared with the user.
	for _, usr := range paste.Users {
		if usr.ID == user.ID {
			return true
		}
	}

	return false
}

// canManage returns true if the user may mutate (delete, change privacy of,
// transfer, label) the paste: the owner of a personal paste; for team pastes
// the team owner, or the paste owner while still an accepted team member.
// The paste's Team must be loaded.
func (p *paste) canManage(paste models.Paste, user models.Users) bool {
	if paste.TeamID != nil {
		if paste.Team.OwnerID == user.ID {
			return true
		}
		return paste.OwnerID == user.ID && p.teamMgr.isActiveMember(*paste.TeamID, user.ID)
	}
	return paste.OwnerID == user.ID
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

// PeekMaxAccesses returns the access budget configured for a paste without
// consuming one. A nil result means the paste has no max_accesses limit.
// Only pastes the caller could read are reported: public pastes when
// publicOnly is set (the anonymous route), otherwise pastes the user in ctx
// may access. Anything else is ErrNotFound, so the budget gate never tells
// an inaccessible paste apart from a missing one. It never touches the
// access counter.
func (p *paste) PeekMaxAccesses(ctx context.Context, pasteID string, publicOnly bool) (*int, error) {
	if publicOnly {
		var row models.Paste
		q := p.conn.Select("max_accesses").
			Where("paste_id = ? and (expires is NULL or expires >= ?) and public = ?", pasteID, time.Now(), true).
			First(&row)
		if q.Error != nil {
			if errors.Is(q.Error, gorm.ErrRecordNotFound) {
				return nil, gErrors.ErrNotFound
			}
			return nil, errors.Wrap(q.Error, "peeking paste access budget")
		}
		return row.MaxAccesses, nil
	}
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching user")
	}
	pst, err := p.loadPaste(pasteID)
	if err != nil {
		return nil, errors.Wrap(err, "peeking paste access budget")
	}
	if !p.canAccess(pst, user) {
		return nil, gErrors.ErrNotFound
	}
	return pst.MaxAccesses, nil
}

func (p *paste) GetPublicPaste(ctx context.Context, pasteID string) (params.Paste, error) {
	var tmpPaste models.Paste
	err := p.transactionRetryingOnBusy(func(tx *gorm.DB) error {
		tmpPaste = models.Paste{}
		now := time.Now()
		// The conditional counter UPDATE runs first (see consumeAccess for
		// why the order matters); the plain read afterwards sees our own
		// increment. It needs no FOR UPDATE lock: the serve decision is
		// made by the UPDATE predicate, not by row locking, and the sqlite
		// driver would drop the lock clause anyway.
		consumed, err := p.consumeAccess(tx, pasteID)
		if err != nil {
			return err
		}
		q := tx.Preload("Labels").Preload("Team").Where(
			"paste_id = ? and (expires is NULL or expires >= ?) and public = ?", pasteID, now, true).First(&tmpPaste)
		if q.Error != nil {
			if errors.Is(q.Error, gorm.ErrRecordNotFound) {
				return gErrors.ErrNotFound
			}
			return errors.Wrap(q.Error, "fetching paste from database")
		}
		if tmpPaste.MaxAccesses != nil {
			if !consumed {
				// The paste has a budget but the UPDATE took no slot: a
				// concurrent viewer just exhausted it. Serve nothing,
				// exactly as if the destroyed row had already vanished.
				return gErrors.ErrNotFound
			}
			if tmpPaste.AccessCount >= *tmpPaste.MaxAccesses {
				return p.destroyExhaustedPaste(tx, &tmpPaste)
			}
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
	err := p.transactionRetryingOnBusy(func(tx *gorm.DB) error {
		tmpPaste = models.Paste{}
		now := time.Now()
		// Counter UPDATE first; see GetPublicPaste for the rationale.
		consumed, err := p.consumeAccess(tx, pasteID)
		if err != nil {
			return err
		}
		q := tx.Preload("Users").Preload("Owner").Preload("Team").Preload("Labels").Where(
			"paste_id = ? and (expires is NULL or expires >= ?)", pasteID, now).First(&tmpPaste)
		if q.Error != nil {
			if errors.Is(q.Error, gorm.ErrRecordNotFound) {
				return gErrors.ErrNotFound
			}
			return errors.Wrap(q.Error, "fetching paste from database")
		}
		if canAccess := p.canAccess(tmpPaste, user); !canAccess {
			// Rolling the transaction back also undoes the increment above,
			// so an inaccessible paste never consumes an access, as before.
			return gErrors.ErrNotFound
		}
		if tmpPaste.MaxAccesses != nil {
			if !consumed {
				// Budget exhausted by a concurrent viewer: gone.
				return gErrors.ErrNotFound
			}
			if tmpPaste.AccessCount >= *tmpPaste.MaxAccesses {
				return p.destroyExhaustedPaste(tx, &tmpPaste)
			}
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
//   - ScopeMine:   pastes the user owns (team pastes only while the user can
//     still see the team, mirroring canAccess)
//   - ScopeShared: pastes of others shared with the user directly, or visible
//     through a team the user is an accepted member or owner of
//   - ScopeAll:    the union of the above
func (p *paste) scopeClause(user models.Users, scope string) (string, []interface{}) {
	const teamVisible = `(team_id IS NOT NULL AND (team_id IN (SELECT teams_id FROM team_users WHERE team_users.users_id = ? AND team_users.status = 'active') OR team_id IN (SELECT id FROM teams WHERE teams.owner_id = ?)))`
	const sharedDirect = `(team_id IS NULL AND EXISTS (SELECT 1 FROM paste_users WHERE paste_users.paste_id = pastes.id AND paste_users.users_id = ?))`

	switch scope {
	case common.ScopeMine:
		return "(owner_id = ? AND (team_id IS NULL OR " + teamVisible + "))",
			[]interface{}{user.ID, user.ID, user.ID}
	case common.ScopeShared:
		return "(owner_id != ? AND (" + sharedDirect + " OR " + teamVisible + "))",
			[]interface{}{user.ID, user.ID, user.ID, user.ID}
	default: // ScopeAll
		return "((owner_id = ? AND team_id IS NULL) OR " + sharedDirect + " OR " + teamVisible + ")",
			[]interface{}{user.ID, user.ID, user.ID, user.ID}
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

// sanitizeFTSQuery turns free-text search input into a safe SQLite FTS5
// query expression. The value bound to `pastes_fts MATCH ?` is parsed by
// the FTS5 query parser, where characters such as `"`, `*`, `(`, `)`, `:`,
// `^`, `-`, `{`, `}` carry syntax meaning (phrases, prefixes, column
// filters, negation, grouping) and unbalanced input aborts the statement
// with a parse error that surfaces as an HTTP 500. Each whitespace-
// separated token is stripped of those characters; the surviving tokens are
// wrapped as double-quoted string phrases (any internal quote doubled,
// defensively, though stripping leaves none) and joined with spaces, which
// FTS5 treats as an implicit AND. Tokens emptied by the stripping are
// dropped, so pure-syntax input yields the empty query rather than a
// parser error, and column/wildcard/boolean injection is impossible.
func sanitizeFTSQuery(query string) string {
	syntax := strings.NewReplacer(`"`, ``, `*`, ``, `(`, ``, `)`, ``, `:`, ``, `^`, ``, `-`, ``, `{`, ``, `}`, ``)
	var phrases []string
	for _, token := range strings.Fields(query) {
		token = syntax.Replace(token)
		if token == "" {
			continue
		}
		phrases = append(phrases, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
	}
	return strings.Join(phrases, " ")
}

// stripMySQLBooleanOperators removes the operators that MySQL's
// MATCH ... AGAINST(... IN BOOLEAN MODE) reserves as search syntax: +
// (required), - (excluded), * (prefix wildcard), " (phrase) and grouping
// parens. Raw user text bound into boolean mode would otherwise let these
// characters silently change the result set or trigger a parse error;
// stripping them degrades the input to plain terms, which is what the
// search box intends.
func stripMySQLBooleanOperators(query string) string {
	return strings.NewReplacer(`+`, ` `, `-`, ` `, `*`, ` `, `"`, ` `, `(`, ` `, `)`, ` `).Replace(query)
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
	// User text is never bound raw into a LIKE pattern: %, _ and \ are LIKE
	// syntax and would let input such as "%%" match every row. Escaping
	// happens once here; every LIKE below declares the escape character.
	searchPattern := "%" + util.EscapeLike(query) + "%"

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
			// IN BOOLEAN MODE allows for more flexible searching. The
			// bound query is parsed as boolean-mode *syntax* (+required,
			// -excluded, *prefix, "phrase", grouping parens), so raw user
			// text could change result semantics or hit a parse error;
			// bind the operator-stripped form so it searches as plain terms.
			q = p.conn.Select(
				"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, max_accesses, access_count, substr(`data`, 1, 512) as data",
			).Where(
				scopeCond+" AND MATCH(name, `data`) AGAINST(? IN BOOLEAN MODE) AND (expires IS NULL OR expires >= ?)",
				append(scopeArgs, stripMySQLBooleanOperators(query), now)...,
			).Order("id desc")
		} else {
			// Fallback to LIKE search
			q = p.conn.Select(
				"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, max_accesses, access_count, substr(`data`, 1, 512) as data",
			).Where(
				scopeCond+" AND (name LIKE ? "+util.LikeEscape+" OR `data` LIKE ? "+util.LikeEscape+") AND (expires IS NULL OR expires >= ?)",
				append(scopeArgs, searchPattern, searchPattern, now)...,
			).Order("id desc")
		}

	case config.SQLiteBackend:
		// SQLite: Use FTS5 for full-text search.
		// Join with FTS table and use MATCH for efficient full-text search.
		// The bound value is an FTS5 *query expression*, so raw user input
		// must be sanitized first (see sanitizeFTSQuery); the scope clause
		// still ANDs visibility on top of the text match.
		ftsQuery := sanitizeFTSQuery(query)
		if ftsQuery == "" {
			// Input consisted solely of FTS5 syntax characters; no token
			// can match it. Answer with an empty page instead of handing
			// the FTS5 parser an empty expression.
			if page > 1 {
				page = 1
			}
			return params.PasteListResult{Pastes: []params.Paste{}, TotalPages: 1, Page: page}, nil
		}
		q = p.conn.Table("pastes").
			Select(
				"pastes.id, pastes.paste_id, pastes.language, pastes.name, pastes.description, pastes.metadata, pastes.owner_id, pastes.team_id, pastes.created_at, pastes.expires, pastes.public, pastes.max_accesses, pastes.access_count, substr(pastes.`data`, 1, 512) as data",
			).
			Joins("INNER JOIN pastes_fts ON pastes.id = pastes_fts.rowid").
			Where("pastes_fts MATCH ? AND "+scopeCond+" AND (pastes.expires IS NULL OR pastes.expires >= ?)", append([]interface{}{ftsQuery}, append(scopeArgs, now)...)...).
			Order("pastes.id desc")

	default:
		// Default fallback: search only in name
		q = p.conn.Select(
			"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, max_accesses, access_count, substr(`data`, 1, 512) as data",
		).Where(scopeCond+" and name LIKE ? "+util.LikeEscape+" and (expires is NULL or expires >= ?)", append(scopeArgs, searchPattern, now)...).Order("id desc")
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
		// Post-load canManage denial: answered with the 404 sentinel so an
		// existing but foreign paste is indistinguishable from a missing one.
		return gErrors.ErrNotFound
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
		"id, paste_id, language, name, description, metadata, owner_id, team_id, created_at, expires, public, max_accesses, access_count, substr(`data`, 1, 512) as data",
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
		// Post-load denial on the sharing path: 404, matching the uniform
		// "foreign paste" answer used by unshare and list-shares.
		return params.TeamMember{}, errors.Wrap(gErrors.ErrNotFound, "sharing foreign paste")
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

	// Insert the join row alone: appending through the association would
	// also upsert the target's users row, re-creating an account deleted
	// in the meantime.
	share := pasteShare{PasteID: pst.ID, UsersID: targetUser.ID}
	if err := p.conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&share).Error; err != nil {
		return params.TeamMember{}, errors.Wrap(err, "sharing with user")
	}
	return sqlUserToTeamMember(targetUser), nil
}

// pasteShare is a row of the paste_users join table behind Paste.Users.
type pasteShare struct {
	PasteID uint `gorm:"column:paste_id;primaryKey"`
	UsersID uint `gorm:"column:users_id;primaryKey"`
}

// TableName pins the table name to the one used by the many2many association.
func (pasteShare) TableName() string { return "paste_users" }

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
		// Post-load denial: this check runs before any canAccess gate, so a
		// 401 here was a direct paste-ID existence oracle. Deny with 404.
		return errors.Wrap(gErrors.ErrNotFound, "unsharing foreign paste")
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
		// Post-load canManage denial: answered with the 404 sentinel.
		return params.PasteShareListResponse{}, errors.Wrap(gErrors.ErrNotFound, "listing shares of foreign paste")
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
		// Post-load canManage denial: answered with the 404 sentinel.
		return params.Paste{}, gErrors.ErrNotFound
	}
	if pst.TeamID != nil && public {
		return params.Paste{}, gErrors.NewBadRequestError("team pastes cannot be made public")
	}
	// A targeted update, not Save: Save would write back every column of the
	// copy loaded above (undoing a concurrent access-count increment) and
	// re-insert the preloaded share and label join rows.
	if err := p.conn.Model(&models.Paste{}).Where("id = ?", pst.ID).Update("public", public).Error; err != nil {
		return params.Paste{}, errors.Wrap(err, "saving paste to DB")
	}
	pst.Public = public
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
		// Post-load canManage denial: answered with the 404 sentinel.
		return params.Paste{}, gErrors.ErrNotFound
	}

	targetUser, err := p.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.Paste{}, errors.Wrap(err, "finding target user")
	}
	if targetUser.ID == pst.OwnerID {
		return params.Paste{}, gErrors.NewBadRequestError("paste is already owned by this user")
	}
	if pst.TeamID != nil && !p.teamMgr.canShareToTeam(pst.Team, targetUser) {
		// Viewers are read-only members: they cannot author team pastes,
		// so they cannot be handed one either.
		return params.Paste{}, gErrors.NewBadRequestError("team pastes can only be transferred to team members who can create pastes")
	}

	err = p.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Paste{}).Where("id = ?", pst.ID).Update("owner_id", targetUser.ID).Error; err != nil {
			return errors.Wrap(err, "saving paste to DB")
		}
		if pst.TeamID != nil {
			return nil
		}
		// Personal labels belong to the previous owner's vocabulary: they
		// would show up (names and all) on the new owner's copy, and keep
		// matching the previous owner's label filters.
		if err := tx.Exec(
			"DELETE FROM paste_labels WHERE paste_id = ? AND label_id IN (SELECT id FROM labels WHERE team_id IS NULL)",
			pst.ID).Error; err != nil {
			return errors.Wrap(err, "dropping previous owner's labels")
		}
		// The new owner no longer needs a share of their own paste.
		if err := tx.Exec("DELETE FROM paste_users WHERE paste_id = ? AND users_id = ?", pst.ID, targetUser.ID).Error; err != nil {
			return errors.Wrap(err, "dropping new owner's share")
		}
		return nil
	})
	if err != nil {
		return params.Paste{}, err
	}

	pst.OwnerID = targetUser.ID
	pst.Owner = targetUser
	if pst.TeamID == nil {
		pst.Labels = nil
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
		// Post-load canManage denial: answered with the 404 sentinel.
		return params.Paste{}, gErrors.ErrNotFound
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
