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
	Owner       Users `gorm:"foreignKey:OwnerID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt   time.Time
	Expires     *time.Time `gorm:"index:expires"`
	Public      bool
	MaxAccesses *int `gorm:"default:NULL"`
	AccessCount int  `gorm:"default:0"`
	TeamID      *uint
	Team        Teams   `gorm:"foreignKey:TeamID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Users       []Users `gorm:"many2many:paste_users;constraint:OnDelete:CASCADE"`
	Labels      []Label `gorm:"many2many:paste_labels;constraint:OnDelete:CASCADE"`
}

// Label is a user- or team-scoped tag that can be attached to pastes.
// Exactly one of OwnerUserID / TeamID is set: personal labels belong to a
// user, team labels to a team. Names are stored case-folded; uniqueness is
// enforced per scope in the application layer (get-or-create).
type Label struct {
	ID          uint   `gorm:"primarykey"`
	Name        string `gorm:"size:64"`
	Color       string `gorm:"size:16;default:''"`
	OwnerUserID *uint  `gorm:"index:idx_label_scope"`
	TeamID      *uint  `gorm:"index:idx_label_scope"`
}

// Users represents a user entry in the database
type Users struct {
	ID          uint `gorm:"primarykey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Username    string  `gorm:"uniqueIndex;type:varchar(64)"`
	FullName    string  `gorm:"type:varchar(254)"`
	Email       string  `gorm:"type:varchar(254);unique;index:idx_email"`
	MemberOf    []Teams `gorm:"many2many:team_users;constraint:OnDelete:CASCADE"`
	Password    string  `gorm:"type:varchar(60)"`
	IsAdmin     bool
	IsSuperUser bool
	Enabled     bool
	// Discoverable controls whether the account may appear in team-invite
	// type-ahead search. Opted-out users can still be invited by exact
	// username or email.
	Discoverable bool `gorm:"default:true"`
}

// Teams represents a team of users
type Teams struct {
	ID          uint   `gorm:"primarykey"`
	Name        string `gorm:"type:varchar(32);uniqueIndex"`
	Description string `gorm:"size:254;default:''"`
	OwnerID     uint
	Owner       Users    `gorm:"foreignKey:OwnerID"`
	Members     []*Users `gorm:"many2many:team_users;constraint:OnDelete:CASCADE"`
	Labels      []Label  `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	// TransferToUserID points at the member who has been asked to take over
	// ownership; the transfer completes only when they accept.
	TransferToUserID *uint  `gorm:"index"`
	TransferTo       *Users `gorm:"foreignKey:TransferToUserID"`
}

// Membership status values for the team_users join table
const (
	TeamMembershipPending = "pending"
	TeamMembershipActive  = "active"
	// RoleOwner is not a status stored in the database; it is the derived
	// relation a viewer has to a team they own.
	RoleOwner = "owner"
	// Stored member roles (TeamUser.Role). Owner is not stored here.
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"
)

// TeamUser is the explicit join model for the team_users table that GORM's
// many2many association maps to. It carries the membership status (invited
// members are "pending" until they accept) and the audit trail of who added
// (or invited) each member. Columns are pinned to the names GORM derives for
// the many2many join so existing databases migrate in place.
type TeamUser struct {
	TeamID    uint   `gorm:"column:teams_id;primaryKey"`
	UserID    uint   `gorm:"column:users_id;primaryKey"`
	Status    string `gorm:"size:16;default:active"`
	Role      string `gorm:"size:16;default:member"`
	AddedByID *uint  `gorm:"column:added_by_id"`
}

// TableName pins the table name to the one used by the many2many association.
func (TeamUser) TableName() string { return "team_users" }

// JWTBacklist is a JWT token blacklist
type JWTBacklist struct {
	TokenID    string `gorm:"primarykey;type:varchar(16)"`
	Expiration int64  `gorm:"index:expire"`
}
