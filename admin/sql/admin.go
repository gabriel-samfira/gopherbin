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
	"fmt"
	"math"
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
		Email:       user.Email,
		Username:    user.Username,
		FullName:    user.FullName,
		Password:    hashedPassword,
		CreatedAt:   time.Now(),
		IsAdmin:     user.IsAdmin,
		IsSuperUser: false,
		Enabled:     user.Enabled,
	}
	return newUser, nil
}

func (u *userManager) sqlUserToParams(user models.Users) params.Users {
	return params.Users{
		ID:          user.ID,
		FullName:    user.FullName,
		Email:       user.Email,
		Username:    user.Username,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Enabled:     user.Enabled,
		IsAdmin:     user.IsAdmin,
		IsSuperUser: user.IsSuperUser,
	}
}

func (u *userManager) Authenticate(ctx context.Context, info params.PasswordLoginParams) (context.Context, error) {
	if info.Username == "" {
		return ctx, gErrors.ErrUnauthorized
	}

	if info.Password == "" {
		return ctx, gErrors.ErrUnauthorized
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
			return ctx, gErrors.NewUnauthorizedError("invalid username or password")
		}
		return ctx, err
	}
	if !modelUser.Enabled {
		return ctx, gErrors.NewUnauthorizedError("user is disabled")
	}
	// If the user has an empty password saved in the
	// database, it is implicitly disabled. This should not happen,
	// but an extra check can't hurt.
	if modelUser.Password == "" {
		return ctx, gErrors.ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(modelUser.Password), []byte(info.Password)); err != nil {
		return ctx, gErrors.ErrUnauthorized
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

	err = u.conn.Create(&newUser).Error
	if err != nil {
		return params.Users{}, errors.Wrap(err, "creating new user")
	}
	return u.sqlUserToParams(newUser), nil
}

// CreateSuperUser creates a new super user. This function should never be called
// from an API handler.
func (u *userManager) CreateSuperUser(user params.NewUserParams) (params.Users, error) {
	if u.HasSuperUser() {
		return params.Users{}, fmt.Errorf("super user already exists")
	}
	newUser, err := u.newUserParamsToSQL(user)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user object")
	}
	newUser.IsSuperUser = true
	newUser.IsAdmin = true
	newUser.Enabled = true

	err = u.conn.Create(&newUser).Error
	if err != nil {
		return params.Users{}, errors.Wrap(err, "creating new user")
	}
	return u.sqlUserToParams(newUser), nil
}

func (u *userManager) Get(ctx context.Context, userID int64) (params.Users, error) {
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

func (u *userManager) Update(ctx context.Context, userID int64, update params.UpdateUserPayload) (params.Users, error) {
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

	// Only superusers may create administrators
	if update.IsAdmin != nil {
		if isSuper {
			tmpUser.IsAdmin = *update.IsAdmin
		} else {
			// return meaningful error. Whould we just ignore the request?
			return params.Users{}, gErrors.NewUnauthorizedError("you are not authorized to perform this action")
		}
	}

	if update.Password != nil {
		hashed, err := util.PaswsordToBcrypt(*update.Password)
		if err != nil {
			return params.Users{}, errors.Wrap(err, "updating password")
		}
		tmpUser.Password = hashed
	}

	if update.Email != nil && *update.Email != tmpUser.Email {
		_, err = u.getUserByEmail(*update.Email)
		if err != nil {
			if err != gErrors.ErrNotFound {
				return params.Users{}, errors.Wrap(err, "updating email")
			}
			tmpUser.Email = *update.Email
		} else {
			return params.Users{}, gErrors.NewDuplicateUserError("email address already in use")
		}
	}

	if update.FullName != nil {
		tmpUser.FullName = *update.FullName
	}

	if update.Enabled != nil {
		if userID == user {
			return params.Users{}, gErrors.NewBadRequestError("you may not enable/disable your own account")
		}
		tmpUser.Enabled = *update.Enabled
	}

	if update.Username != nil {
		if tmpUser.Username != "" {
			return params.Users{}, gErrors.NewBadRequestError("username is already set")
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
	// TODO: When we update the user for any reason, it will invalidate
	// all login tokens. Add a separate field as witness instead of UpdatedAt,
	// which will only update when the password is reset, or when any other
	// operation that should invalidate a token, happens.
	tmpUser.UpdatedAt = time.Now()
	q := u.conn.Save(&tmpUser)
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

func (u *userManager) getUser(userID int64) (models.Users, error) {
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

func (u *userManager) setEnabledFlag(userID int64, enabled bool) error {
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

func (u *userManager) Enable(ctx context.Context, userID int64) error {
	isAdmin := auth.IsAdmin(ctx)
	if !isAdmin {
		return gErrors.ErrUnauthorized
	}
	return u.setEnabledFlag(userID, true)
}

func (u *userManager) Disable(ctx context.Context, userID int64) error {
	isAdmin := auth.IsAdmin(ctx)
	if !isAdmin {
		return gErrors.ErrUnauthorized
	}
	return u.setEnabledFlag(userID, false)
}

func (u *userManager) Delete(ctx context.Context, userID int64) error {
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
	q := u.conn.Delete(&usr)
	if q.Error != nil {
		return errors.Wrap(q.Error, "deleting user")
	}
	return nil
}
