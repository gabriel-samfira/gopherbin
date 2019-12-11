package mysql

import (
	"context"
	"fmt"
	"time"

	"gopherbin/admin/common"
	"gopherbin/auth"
	"gopherbin/config"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/util"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
)

// NewUserManager returns a new *UserManager
func NewUserManager(dbCfg config.Database, defCfg config.Default) (common.UserManager, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}
	return &userManager{
		conn: db,
		cfg:  defCfg,
	}, nil
}

// UserManager defined functions that handle the
// creation and updating of users
type userManager struct {
	conn *gorm.DB
	cfg  config.Default
}

func (u *userManager) newUserParamsToSQL(user params.Users) (models.Users, error) {
	if err := util.ValidateNewUser(user); err != nil {
		return models.Users{}, errors.Wrap(err, "validating user info")
	}
	// When creating a new user only 3 fields are ever used:
	// Email, FullName and Password. The ID is generated from the email
	// address, and the rest of the fields should be set by an administrator
	// or the superuser.
	hashedPassword, err := util.PaswsordToBcrypt(user.Password)
	if err != nil {
		return models.Users{}, errors.Wrap(err, "hashing password")
	}
	id, err := util.HashString(user.Email)
	if err != nil {
		return models.Users{}, errors.Wrap(err, "hashing the email address")
	}
	newUser := models.Users{
		ID:          int64(id),
		Email:       user.Email,
		FullName:    user.FullName,
		Password:    hashedPassword,
		CreatedAt:   time.Now(),
		IsAdmin:     false,
		IsSuperUser: false,
		Enabled:     false,
	}
	return newUser, nil
}

func (u *userManager) sqlUserToParams(user models.Users) params.Users {
	return params.Users{
		ID:          user.ID,
		FullName:    user.FullName,
		Email:       user.Email,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Enabled:     user.Enabled,
		IsAdmin:     user.IsAdmin,
		IsSuperUser: user.IsSuperUser,
	}
}

func (u *userManager) Create(ctx context.Context, user params.Users) (params.Users, error) {
	if u.cfg.RegistrationOpen == false && auth.IsAdmin(ctx) == false {
		return params.Users{}, auth.ErrUnauthorized
	}
	newUser, err := u.newUserParamsToSQL(user)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user object")
	}
	tmpUser, err := u.getUser(newUser.ID)
	if err != nil && err != auth.ErrNotFound {
		return params.Users{}, errors.Wrap(err, "fetching user")
	}

	if err != auth.ErrNotFound {
		return params.Users{}, fmt.Errorf("the email address %s is already in use", newUser.Email)
	}

	err = u.conn.Create(&newUser).Error
	if err != nil {
		return params.Users{}, errors.Wrap(err, "creating new user")
	}
	return u.sqlUserToParams(tmpUser), nil
}

func (u *userManager) Update(ctx context.Context, userID int64, update params.UpdateUserPayload) (params.Users, error) {
	tmpUser, err := u.getUser(userID)
	if err != nil {
		return params.Users{}, errors.Wrap(err, "fetching user")
	}
	isAdmin := auth.IsAdmin(ctx)
	isSuper := auth.IsSuperUser(ctx)
	user := auth.UserID(ctx)
	if user == 0 {
		return params.Users{}, auth.ErrUnauthorized
	}
	// Only superusers may create administrators
	if update.IsAdmin != nil && isSuper == false {
		return params.Users{}, auth.ErrUnauthorized
	}
	// A user may update their own info, or an admin may
	// update another user's info.
	if userID != user && isAdmin == false {
		return params.Users{}, auth.ErrUnauthorized
	}

	if update.IsAdmin != nil {
		tmpUser.IsAdmin = *update.IsAdmin
	}

	if update.Password != nil {
		hashed, err := util.PaswsordToBcrypt(*update.Password)
		if err != nil {
			return params.Users{}, errors.Wrap(err, "updating password")
		}
		tmpUser.Password = hashed
	}

	if update.FullName != nil {
		if len(*update.FullName) == 0 {
			return params.Users{}, fmt.Errorf("name may not be empty")
		}
		tmpUser.FullName = *update.FullName
	}
	q := u.conn.Save(&tmpUser)
	if q.Error != nil {
		return params.Users{}, errors.Wrap(q.Error, "saving user to database")
	}
	return u.sqlUserToParams(tmpUser), nil
}

func (u *userManager) getUser(userID int64) (models.Users, error) {
	var tmpUser models.Users
	q := u.conn.First(&tmpUser)
	if q.Error != nil {
		if q.RecordNotFound() {
			return models.Users{}, auth.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (u *userManager) setEnabledFlag(userID int64, enabled bool) error {
	usr, err := u.getUser(userID)
	if err != nil {
		return errors.Wrap(err, "fetching user from db")
	}
	usr.Enabled = enabled
	err = u.conn.Save(&usr).Error
	if err != nil {
		return errors.Wrap(err, "saving user to database")
	}
	return nil
}

func (u *userManager) Enable(ctx context.Context, userID int64) error {
	isAdmin := auth.IsAdmin(ctx)
	if isAdmin == false {
		return auth.ErrUnauthorized
	}
	return u.setEnabledFlag(userID, true)
}

func (u *userManager) Disable(ctx context.Context, userID int64) error {
	isAdmin := auth.IsAdmin(ctx)
	if isAdmin == false {
		return auth.ErrUnauthorized
	}
	return u.setEnabledFlag(userID, false)
}

func (u *userManager) Delete(ctx context.Context, userID int64) error {
	isAdmin := auth.IsAdmin(ctx)
	if isAdmin == false {
		return auth.ErrUnauthorized
	}
	usr, err := u.getUser(userID)
	if err != nil {
		return errors.Wrap(err, "fetching user from db")
	}
	q := u.conn.Delete(&usr)
	if q.Error != nil {
		return errors.Wrap(q.Error, "deleting user")
	}
	return nil
}
