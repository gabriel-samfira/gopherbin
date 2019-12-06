package models

// Paste represents a pastebin entry in the database
type Paste struct {
	ID      string `gorm:"type:varchar(32);primary_key"`
	Data    []byte `gorm:"type:longblob"`
	Owner   Users
	Expires int64
	Public  bool
	Teams []*Teams `gorm:"many2many:paste_teams;"`
	Users []*Users `gorm:"many2many:paste_users;"`
}

// Users represents a user entry in the database
type Users struct {
	ID       int64    `gorm:"primary_key"`
	FullName string   `gorm:"type:varchar(254)"`
	Email    string   `gorm:"type:varchar(254)"`
	Teams    []*Teams `gorm:"many2many:team_users;"`
}

// Teams represents a team of users
type Teams struct {
	ID    int64  `gorm:"primary_key"`
	Name  string `gorm:"type:varchar(32)"`
	Owner Users
	Users []*Users `gorm:"many2many:team_users;"`
}
