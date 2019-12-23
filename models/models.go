package models

import "time"

// TeamUsers is used for many2many relation between
// the users model and the teams model. We define it here
// in order to be able to work around gorm inability
// to automatically create foreign key constraints during
// AutoMigrate.
type TeamUsers struct {
	UsersID int `gorm:"type:bigint(20);primary_key;auto_increment:false"`
	TeamsID int `gorm:"type:bigint(20);primary_key;auto_increment:false"`
}

// PasteTeams is used for many2many relation between
// the paste model and the teams model. We define it here
// in order to be able to work around gorm inability
// to automatically create foreign key constraints during
// AutoMigrate.
type PasteTeams struct {
	PasteID string `gorm:"primary_key;type:varchar(32)"`
	TeamsID int    `gorm:"primary_key;auto_increment:false;type:bigint(20)"`
}

// PasteUsers is used for many2many relation between
// the paste model and the users model. We define it here
// in order to be able to work around gorm inability
// to automatically create foreign key constraints during
// AutoMigrate.
type PasteUsers struct {
	PasteID string `gorm:"primary_key;type:varchar(32)"`
	UsersID int    `gorm:"primary_key;auto_increment:false;type:bigint(20)"`
}

// Paste represents a pastebin entry in the database
type Paste struct {
	ID        string `gorm:"type:varchar(32);primary_key"`
	Data      []byte `gorm:"type:longblob"`
	Language  string `gorm:"type:varchar(64)"`
	Name      string
	Owner     int64
	CreatedAt time.Time
	Expires   *time.Time
	Public    bool
	Teams     []Teams `gorm:"many2many:paste_teams;"`
	Users     []Users `gorm:"many2many:paste_users;"`
}

// Users represents a user entry in the database
type Users struct {
	ID           int64 `gorm:"primary_key"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	FullName     string   `gorm:"type:varchar(254)"`
	Email        string   `gorm:"type:varchar(254)"`
	Teams        []*Teams `gorm:"many2many:team_users;"`
	CreatedTeams []Teams  `gorm:"foreignkey:Owner"`
	Password     string   `gorm:"type:varchar(60)"`
	IsAdmin      bool
	IsSuperUser  bool
	Enabled      bool
}

// Teams represents a team of users
type Teams struct {
	ID      int64  `gorm:"primary_key"`
	Name    string `gorm:"type:varchar(32)"`
	Owner   int64
	Members []*Users `gorm:"many2many:team_users;"`
}
