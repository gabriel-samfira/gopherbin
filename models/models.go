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

// Paste represents a pastebin entry in the database
type Paste struct {
	ID          uint   `gorm:"primarykey"`
	PasteID     string `gorm:"type:varchar(32);uniqueIndex"`
	Data        []byte `gorm:"type:longblob"`
	Language    string `gorm:"type:varchar(64)"`
	Name        string
	Description string
	Metadata    datatypes.JSON
	OwnerID     uint
	Owner       Users `gorm:"foreignKey:OwnerID"`
	CreatedAt   time.Time
	Expires     *time.Time `gorm:"index:expires"`
	Public      bool
	TeamID      uint
	Team        Teams   `gorm:"foreignKey:TeamID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Users       []Users `gorm:"many2many:paste_users;"`
}

// Users represents a user entry in the database
type Users struct {
	ID          uint `gorm:"primarykey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Username    string  `gorm:"index;unique;varchar(64)"`
	FullName    string  `gorm:"type:varchar(254)"`
	Email       string  `gorm:"type:varchar(254);unique;index:idx_email"`
	Teams       []Teams `gorm:"foreignKey:Owner"`
	Password    string  `gorm:"type:varchar(60)"`
	IsAdmin     bool
	IsSuperUser bool
	Enabled     bool
}

// Teams represents a team of users
type Teams struct {
	ID      uint     `gorm:"primarykey"`
	Name    string   `gorm:"type:varchar(32);uniqueIndex"`
	Owner   uint     `gorm:"index"`
	Members []*Users `gorm:"many2many:team_users;"`
}

// JWTBacklist is a JWT token blacklist
type JWTBacklist struct {
	TokenID    string `gorm:"primarykey;type:varchar(16)"`
	Expiration int64  `gorm:"index:expire"`
}
