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

package models

import (
	"time"

	"gorm.io/datatypes"
)

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
	ID          int64  `gorm:"primary_key"`
	PasteID     string `gorm:"type:varchar(32);unique_index"`
	Data        []byte `gorm:"type:longblob"`
	Language    string `gorm:"type:varchar(64)"`
	Name        string
	Description string
	Metadata    datatypes.JSON
	Owner       int64
	CreatedAt   time.Time
	Expires     *time.Time `gorm:"index:expires"`
	Public      bool
	Encrypted   bool
	Teams       []Teams `gorm:"many2many:paste_teams;"`
	Users       []Users `gorm:"many2many:paste_users;"`
}

// Users represents a user entry in the database
type Users struct {
	ID           int64 `gorm:"primary_key"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	FullName     string   `gorm:"type:varchar(254)"`
	Email        string   `gorm:"type:varchar(254);unique;index:idx_email"`
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

// JWTBacklist is a JWT token blacklist
type JWTBacklist struct {
	TokenID    string `gorm:"primary_key;type:varchar(16)"`
	Expiration int64  `gorm:"index:expire"`
}
