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
	"math"
	"strings"
	"time"

	"gopherbin/admin/common"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/util"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// NewUserManager returns a new *UserManager
func NewUserManager(dbCfg config.Database) (common.UserManager, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}
	return &userManager{
		conn: db,
	}, nil
}

// UserManager defined functions that handle the
// creation and updating of users
type userManager struct {
	conn *gorm.DB
}

func (u *userManager) HasSuperUser() bool {
	var tmpUser models.Users
	q := u.conn.Where("is_super_user = ?", true).First(&tmpUser)
	if q.Error != nil || tmpUser.ID == 0 {
		return false
	}
	return true
}

func (u *userManager) newUserParamsToSQL(user params.NewUserParams) (models.Users, error) {
	if err := user.Validate(); err != nil {
		return models.Users{}, gErrors.NewBadRequestError("error validating parameters: %s", err)
	}
	// When creating a new user only 3 fields are ever used:
	// Email, FullName and Password. The ID is generated from the email
	// address, and the rest of the fields should be set by an administrator
	// or the superuser.
	hashedPassword, err := util.PaswsordToBcrypt(user.Password)
	if err != nil {
		return models.Users{}, errors.Wrap(err, "hashing password")
	}
	newUser := models.Users{
		Email:        user.Email,
		Username:     user.Username,
		FullName:     user.FullName,
		Password:     hashedPassword,
		CreatedAt:    time.Now(),
		IsAdmin:      user.IsAdmin,
		IsSuperUser:  false,
		Enabled:      user.Enabled,
		Discoverable: true,
	}
	return newUser, nil
}

func (u *userManager) sqlUserToParams(user models.Users) params.Users {
	return params.Users{
		ID:           user.ID,
		FullName:     user.FullName,
		Email:        user.Email,
		Username:     user.Username,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Enabled:      user.Enabled,
		IsAdmin:      user.IsAdmin,
		IsSuperUser:  user.IsSuperUser,
		Discoverable: user.Discoverable,
	}
}

// invalidCredentialsMsg is returned by every failed Authenticate path so that
// clients cannot enumerate accounts (unknown user, disabled account, wrong
// password) from the response body.
const invalidCredentialsMsg = "invalid username or password"

func (u *userManager) Authenticate(ctx context.Context, info params.PasswordLoginParams) (context.Context, error) {
	// All rejection paths return this identical error. The file has no
	// logger to record the real reason server-side.
	unauthorized := gErrors.NewUnauthorizedError(invalidCredentialsMsg)

	if info.Username == "" {
		return ctx, unauthorized
	}

	if info.Password == "" {
		return ctx, unauthorized
	}

	isEmail := util.IsValidEmail(info.Username)
	var modelUser models.Users
	var err error
	if isEmail {
		modelUser, err = u.getUserByEmail(info.Username)
	} else {
		modelUser, err = u.getUserByUsername(info.Username)
	}

	if err != nil {
		if err == gErrors.ErrNotFound {
			// Burn a comparable amount of CPU to a real bcrypt
			// verification, so "no such account" cannot be told apart
			// from "wrong password" by response timing.
			if dummyHash, hashErr := util.PaswsordToBcrypt("x"); hashErr == nil {
				_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(info.Password))
			}
			return ctx, unauthorized
		}
		return ctx, err
	}
	// The password comparison runs before the Enabled check so that a
	// disabled account takes exactly as long as an active one. An empty
	// stored hash always fails this comparison, which implicitly disables
	// such accounts (this should not happen, but an extra check can't hurt).
	if err := bcrypt.CompareHashAndPassword([]byte(modelUser.Password), []byte(info.Password)); err != nil {
		return ctx, unauthorized
	}
	if !modelUser.Enabled {
		return ctx, unauthorized
	}
	userParams := u.sqlUserToParams(modelUser)
	return auth.PopulateContext(ctx, userParams), nil
}

func (u *userManager) Create(ctx context.Context, user params.NewUserParams) (params.Users, error) {
	if !auth.IsAdmin(ctx) {
		return params.Users{}, gErrors.ErrUnauthorized
	}

	if user.IsAdmin && !auth.IsSuperUser(ctx) {
		return params.Users{}, gErrors.ErrUnauthorized
	}

	if user.FullName == "" || len(user.FullName) > 255 {
		return params.Users{}, gErrors.NewBadRequestError("invalid full name")
	}

	if user.Email == "" || !util.IsValidEmail(user.Email) {
		return params.Users{}, gErrors.NewBadRequestError("invalid email")
	}

	if user.Username == "" || !util.IsAlphanumeric(user.Username) {
		return params.Users{}, gErrors.NewBadRequestError("invalid username")
	}

	newUser, err := u.newUserParamsToSQL(user)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user object")
	}
	_, err = u.getUserByEmail(newUser.Email)
	if err != nil {
		if err != gErrors.ErrNotFound {
			return params.Users{}, errors.Wrap(err, "fetching user")
		}
	} else {
		return params.Users{}, gErrors.ErrDuplicateEntity
	}

	_, err = u.getUserByUsername(newUser.Username)
	if err != nil {
		if err != gErrors.ErrNotFound {
			return params.Users{}, errors.Wrap(err, "fetching user")
		}
	} else {
		return params.Users{}, gErrors.ErrDuplicateEntity
	}

	err = u.conn.Create(&newUser).Error
	if err != nil {
		return params.Users{}, errors.Wrap(err, "creating new user")
	}
	return u.sqlUserToParams(newUser), nil
}

// errSuperUserInsertSkipped is an internal sentinel returned from the
// CreateSuperUser transaction when the conditional INSERT matched no rows,
// i.e. a concurrent caller created the super user first.
var errSuperUserInsertSkipped = errors.New("super user already exists")

// CreateSuperUser creates a new super user. This function should never be called
// from an API handler.
func (u *userManager) CreateSuperUser(user params.NewUserParams) (params.Users, error) {
	// Fast path only; this check is itself racy. The real guarantee is the
	// conditional INSERT below, which runs inside a transaction.
	if u.HasSuperUser() {
		return params.Users{}, gErrors.NewConflictError("super user already exists")
	}
	newUser, err := u.newUserParamsToSQL(user)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user object")
	}
	newUser.IsSuperUser = true
	newUser.IsAdmin = true
	newUser.Enabled = true

	// SQLite serializes writers, and the NOT EXISTS guard is evaluated by
	// the INSERT statement itself while the write lock is held, so check
	// and act are atomic: of N concurrent first-run callers at most one
	// INSERT affects a row. The retry loop only exists so that a caller
	// which lost the race (or hit a transient SQLITE_BUSY) settles on
	// "super user already exists" instead of a raw driver error.
	var created models.Users
	const maxAttempts = 10
	var txErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if u.HasSuperUser() {
				return params.Users{}, gErrors.NewConflictError("super user already exists")
			}
			time.Sleep(time.Duration(attempt) * 20 * time.Millisecond)
		}
		txErr = u.conn.Transaction(func(tx *gorm.DB) error {
			now := time.Now()
			// Columns are listed explicitly because GORM's regular
			// Create would auto-populate created_at/updated_at; on the
			// raw path they must be set by hand.
			q := tx.Exec(`INSERT INTO users (created_at, updated_at, username, full_name, email, password, is_admin, is_super_user, enabled, discoverable)
	SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
	WHERE NOT EXISTS (SELECT 1 FROM users WHERE is_super_user = ?)`,
				now, now,
				newUser.Username, newUser.FullName, newUser.Email, newUser.Password,
				newUser.IsAdmin, newUser.IsSuperUser, newUser.Enabled, newUser.Discoverable,
				true)
			if q.Error != nil {
				return errors.Wrap(q.Error, "conditional super user insert")
			}
			if q.RowsAffected == 0 {
				return errSuperUserInsertSkipped
			}
			// Still holding the write lock: verify that exactly one
			// super user exists and fetch the row just inserted.
			var count int64
			if err := tx.Model(&models.Users{}).Where("is_super_user = ?", true).Count(&count).Error; err != nil {
				return errors.Wrap(err, "counting super users")
			}
			if count != 1 {
				return errors.Errorf("expected exactly one super user after insert, found %d", count)
			}
			if err := tx.Where("is_super_user = ?", true).First(&created).Error; err != nil {
				return errors.Wrap(err, "fetching created super user")
			}
			return nil
		})
		if txErr == nil {
			return u.sqlUserToParams(created), nil
		}
		if errors.Is(txErr, errSuperUserInsertSkipped) {
			return params.Users{}, gErrors.NewConflictError("super user already exists")
		}
	}
	return params.Users{}, errors.Wrap(txErr, "creating new user")
}

func (u *userManager) Get(ctx context.Context, userID uint) (params.Users, error) {
	user := auth.UserID(ctx)
	if user != userID && !auth.IsAdmin(ctx) {
		return params.Users{}, gErrors.ErrUnauthorized
	}
	modelUser, err := u.getUser(userID)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user form DB")
	}
	return u.sqlUserToParams(modelUser), nil
}

func (u *userManager) List(ctx context.Context, page int64, results int64) (paste params.UserListResult, err error) {
	if !auth.IsAdmin(ctx) {
		return params.UserListResult{}, gErrors.ErrUnauthorized
	}

	if page == 0 {
		page = 1
	}
	if results == 0 {
		results = 1
	}

	var userResults []models.Users
	var cnt int64
	startFrom := (page - 1) * results

	cntQ := u.conn.Model(&models.Users{}).Count(&cnt)
	if cntQ.Error != nil {
		return params.UserListResult{}, errors.Wrap(cntQ.Error, "counting results")
	}

	resQ := u.conn.Offset(int(startFrom)).Limit(int(results)).Find(&userResults)
	if resQ.Error != nil {
		if errors.Is(resQ.Error, gorm.ErrRecordNotFound) {
			return params.UserListResult{}, gErrors.ErrNotFound
		}
		return params.UserListResult{}, errors.Wrap(resQ.Error, "fetching pastes from database")
	}
	asParams := make([]params.Users, len(userResults))
	for idx, val := range userResults {
		asParams[idx] = u.sqlUserToParams(val)
	}
	totalPages := int64(math.Ceil(float64(cnt) / float64(results)))
	if totalPages == 0 {
		totalPages = 1
	}
	return params.UserListResult{
		TotalPages: totalPages,
		Users:      asParams,
	}, nil
}

func (u *userManager) Update(ctx context.Context, userID uint, update params.UpdateUserPayload) (params.Users, error) {
	if err := update.Validate(); err != nil {
		return params.Users{}, errors.Wrap(err, "validating params")
	}

	tmpUser, err := u.getUser(userID)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user")
	}
	isAdmin := auth.IsAdmin(ctx)
	isSuper := auth.IsSuperUser(ctx)
	user := auth.UserID(ctx)
	if user == 0 {
		return params.Users{}, gErrors.ErrUnauthorized
	}

	// A user may update their own info, or an admin may
	// update another user's info.
	if userID != user && !isAdmin {
		return params.Users{}, gErrors.ErrUnauthorized
	}

	// Superuser accounts may only be modified by a superuser (or by
	// themselves), otherwise a plain admin could reset their password.
	if tmpUser.IsSuperUser && userID != user && !isSuper {
		return params.Users{}, gErrors.NewUnauthorizedError("only a superuser may modify the superuser account")
	}

	// Only superusers may create administrators
	if update.IsAdmin != nil {
		if isSuper {
			tmpUser.IsAdmin = *update.IsAdmin
		} else {
			return params.Users{}, gErrors.NewUnauthorizedError("you are not authorized to perform this action")
		}
	}

	if update.Password != nil {
		// A user changing their own password must prove knowledge of the
		// current one: a stolen session token alone must not be enough to
		// take over the account by setting a new password. Admin-initiated
		// resets of other users (userID != caller) never carry the current
		// password by design, so they are exempt.
		if userID == user {
			if update.CurrentPassword == "" {
				return params.Users{}, gErrors.NewUnauthorizedError("current password is incorrect")
			}
			if err := bcrypt.CompareHashAndPassword([]byte(tmpUser.Password), []byte(update.CurrentPassword)); err != nil {
				return params.Users{}, gErrors.NewUnauthorizedError("current password is incorrect")
			}
		}
		hashed, err := util.PaswsordToBcrypt(*update.Password)
		if err != nil {
			return params.Users{}, errors.Wrap(err, "updating password")
		}
		tmpUser.Password = hashed
	}

	if update.Email != nil && *update.Email != tmpUser.Email {
		_, err = u.getUserByEmail(*update.Email)
		if err != nil {
			if !errors.Is(err, gErrors.ErrNotFound) {
				return params.Users{}, errors.Wrap(err, "updating email")
			}
			tmpUser.Email = *update.Email
		} else {
			return params.Users{}, gErrors.NewDuplicateUserError("email address already in use")
		}
	}

	if update.FullName != nil && *update.FullName != tmpUser.FullName {
		tmpUser.FullName = *update.FullName
	}

	if update.Enabled != nil && *update.Enabled != tmpUser.Enabled {
		if userID == user {
			return params.Users{}, gErrors.NewBadRequestError("you may not enable/disable your own account")
		}
		tmpUser.Enabled = *update.Enabled
	}

	if update.Discoverable != nil {
		tmpUser.Discoverable = *update.Discoverable
	}

	if update.Username != nil {
		if *update.Username != tmpUser.Username {
			if tmpUser.Username != "" {
				return params.Users{}, gErrors.NewBadRequestError("username is already set")
			}
			if !util.IsAlphanumeric(*update.Username) {
				return params.Users{}, gErrors.NewBadRequestError("username must be alphanumeric")
			}
			_, err := u.getUserByUsername(*update.Username)
			if err != nil {
				if !errors.Is(err, gErrors.ErrNotFound) {
					return params.Users{}, errors.Wrap(err, "looking up user")
				}
			} else {
				return params.Users{}, errors.Wrap(gErrors.ErrDuplicateEntity, "updating username")
			}
			tmpUser.Username = *update.Username
		}
	}
	// Updating sensitive attributes invalidates all login tokens (the JWT
	// carries UpdatedAt). Preference-only changes (discoverable) must not
	// sign the user out of their own sessions.
	sensitive := update.Password != nil || update.Email != nil || update.FullName != nil ||
		update.Enabled != nil || update.Username != nil || update.IsAdmin != nil
	if sensitive {
		tmpUser.UpdatedAt = time.Now()
	}
	save := u.conn
	if !sensitive {
		// GORM's Save refreshes UpdatedAt automatically; preference-only
		// changes must leave it untouched so issued tokens stay valid.
		save = save.Omit("UpdatedAt")
	}
	q := save.Save(&tmpUser)
	if q.Error != nil {
		return params.Users{}, errors.Wrap(q.Error, "saving user to database")
	}
	return u.sqlUserToParams(tmpUser), nil
}

func (u *userManager) getUserByEmail(email string) (models.Users, error) {
	var tmpUser models.Users
	q := u.conn.Where("email = ?", email).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (u *userManager) getUserByUsername(username string) (models.Users, error) {
	var tmpUser models.Users
	q := u.conn.Where("username = ?", username).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (u *userManager) getUser(userID uint) (models.Users, error) {
	var tmpUser models.Users
	q := u.conn.Where("id = ?", userID).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (u *userManager) ValidateToken(tokenID string) error {
	if tokenID == "" {
		return gErrors.ErrUnauthorized
	}

	var token models.JWTBacklist
	q := u.conn.Where("token_id = ?", tokenID).First(&token)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Wrap(q.Error, "checking token blacklist")
	}
	return gErrors.ErrUnauthorized
}

func (u *userManager) BlacklistToken(tokenID string, expiration int64) error {
	token := models.JWTBacklist{
		TokenID:    tokenID,
		Expiration: expiration,
	}
	err := u.conn.Create(&token).Error
	if err != nil {
		return errors.Wrap(err, "updating blacklist")
	}
	return nil
}

func (u *userManager) CleanTokens() error {
	now := time.Now().Unix()
	err := u.conn.Where("expiration < ?", now).Delete(models.JWTBacklist{}).Error
	if err != nil {
		return errors.Wrap(err, "pruning tokens")
	}
	return nil
}

func (u *userManager) setEnabledFlag(userID uint, enabled bool) error {
	usr, err := u.getUser(userID)
	if err != nil {
		return errors.Wrap(err, "fetching user from db")
	}
	usr.Enabled = enabled
	usr.UpdatedAt = time.Now()
	err = u.conn.Save(&usr).Error
	if err != nil {
		return errors.Wrap(err, "saving user to database")
	}
	return nil
}

func (u *userManager) Enable(ctx context.Context, userID uint) error {
	if !auth.IsAdmin(ctx) {
		return gErrors.ErrUnauthorized
	}
	if err := u.ensureNotSuperUser(userID, auth.IsSuperUser(ctx)); err != nil {
		return err
	}
	return u.setEnabledFlag(userID, true)
}

func (u *userManager) Disable(ctx context.Context, userID uint) error {
	if !auth.IsAdmin(ctx) {
		return gErrors.ErrUnauthorized
	}
	if err := u.ensureNotSuperUser(userID, auth.IsSuperUser(ctx)); err != nil {
		return err
	}
	return u.setEnabledFlag(userID, false)
}

// ensureNotSuperUser rejects changes to a superuser account unless the actor
// is a superuser.
func (u *userManager) ensureNotSuperUser(userID uint, actorIsSuper bool) error {
	if actorIsSuper {
		return nil
	}
	usr, err := u.getUser(userID)
	if err != nil {
		return errors.Wrap(err, "fetching user from db")
	}
	if usr.IsSuperUser {
		return gErrors.NewUnauthorizedError("only a superuser may modify the superuser account")
	}
	return nil
}

func (u *userManager) Delete(ctx context.Context, userID uint) error {
	isAdmin := auth.IsAdmin(ctx)
	if !isAdmin {
		return gErrors.ErrUnauthorized
	}
	isSuperUser := auth.IsSuperUser(ctx)
	currentUserID := auth.UserID(ctx)
	if userID == currentUserID {
		return gErrors.NewConflictError("you may not delete your own account")
	}

	usr, err := u.getUser(userID)
	if err != nil {
		return errors.Wrap(err, "fetching user from db")
	}
	if usr.IsSuperUser {
		return gErrors.NewUnauthorizedError("the superuser may not be deleted")
	}

	if usr.IsAdmin && !isSuperUser {
		return gErrors.NewUnauthorizedError("only a superuser may delete an admin")
	}

	// A user who still owns teams cannot be deleted: deleting them would
	// either orphan or silently destroy those teams and their pastes.
	var ownedTeams int64
	if err := u.conn.Model(&models.Teams{}).Where("owner_id = ?", userID).Count(&ownedTeams).Error; err != nil {
		return errors.Wrap(err, "counting owned teams")
	}
	if ownedTeams > 0 {
		return gErrors.NewConflictError("this user still owns one or more teams; delete those teams first")
	}

	q := u.conn.Delete(&usr)
	if q.Error != nil {
		return errors.Wrap(q.Error, "deleting user")
	}
	return nil
}

// SearchUsers returns enabled, discoverable users matching the query for the
// team-invite type-ahead. Matches are username prefix, full-name substring or
// email prefix. Users who opted out of discovery are never listed; they can
// still be invited by exact username/email. If excludeTeam is set, members
// (active or pending) of that team and the caller are filtered out.
func (u *userManager) SearchUsers(ctx context.Context, query string, excludeTeam string) ([]params.UserSearchResult, error) {
	viewer := auth.UserID(ctx)
	if viewer == 0 {
		return nil, gErrors.ErrUnauthorized
	}
	q := strings.TrimSpace(query)
	if len(q) < 2 {
		return []params.UserSearchResult{}, nil
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)

	tx := u.conn.Model(&models.Users{}).
		Where("enabled = ? AND discoverable = ?", true, true).
		Where("id <> ?", viewer).
		Where(
			u.conn.Where("username LIKE ? ESCAPE '\\'", escaped+"%").
				Or("full_name LIKE ? ESCAPE '\\'", "%"+escaped+"%").
				Or("email LIKE ? ESCAPE '\\'", escaped+"%"),
		)

	if excludeTeam != "" {
		var team models.Teams
		if err := u.conn.Where("name = ?", excludeTeam).First(&team).Error; err == nil {
			sub := u.conn.Table("team_users").
				Select("users_id").
				Where("teams_id = ?", team.ID)
			tx = tx.Where("id NOT IN (?)", sub)
		}
	}

	var found []models.Users
	if err := tx.Order("username ASC").Limit(8).Find(&found).Error; err != nil {
		return nil, errors.Wrap(err, "searching users")
	}

	out := make([]params.UserSearchResult, 0, len(found))
	for i := range found {
		out = append(out, params.UserSearchResult{
			ID:       found[i].ID,
			Username: found[i].Username,
			FullName: found[i].FullName,
		})
	}
	return out, nil
}
