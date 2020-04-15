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

package params

import (
	"fmt"
	"time"

	"gopherbin/errors"
	"gopherbin/util"

	zxcvbn "github.com/nbutton23/zxcvbn-go"
)

// Teams holds information about a team
type Teams struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Owner   Users   `json:"owner"`
	Members []Users `json:"members"`
}

// Users holds information about a particular user
type Users struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	FullName    string    `json:"full_name"`
	Password    string    `json:"password"`
	Enabled     bool      `json:"enabled"`
	IsAdmin     bool      `json:"is_admin"`
	IsSuperUser bool      `json:"is_superuser"`
}

// FormattedCreatedAt returns a DD-MM-YY formatted createdAt
// date
func (u Users) FormattedCreatedAt() string {
	return u.CreatedAt.Format("02-Jan-2006")
}

// FormattedUpdatedAt returns a DD-MM-YY formatted expiration
// date
func (u Users) FormattedUpdatedAt() string {
	return u.UpdatedAt.Format("02-Jan-2006")
}

// UserListResult holds results for a user list request
type UserListResult struct {
	TotalPages int64   `json:"total_pages"`
	Users      []Users `json:"users"`
}

// NewUserParams holds the needed information to create
// a new user
type NewUserParams struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
	Enabled  bool   `json:"enabled"`
}

// Validate validates the object in order to determine
// if the minimum required fields have proper values (email
// is valid, password is of a decent strength etc).
func (u NewUserParams) Validate() error {
	passwordStenght := zxcvbn.PasswordStrength(u.Password, nil)
	if passwordStenght.Score < 4 {
		return fmt.Errorf("the password is too weak, please use a stronger password")
	}
	if !util.IsValidEmail(u.Email) {
		return fmt.Errorf("invalid email address %s", u.Email)
	}

	if len(u.FullName) == 0 {
		return fmt.Errorf("full name may not be empty")
	}
	return nil
}

// UpdateUserPayload defines fields that may be updated
// on a user entry
type UpdateUserPayload struct {
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Password *string `json:"password,omitempty"`
	FullName *string `json:"full_name,omitempty"`
}

// Paste holds information about a paste
type Paste struct {
	ID        int64      `json:"id"`
	PasteID   string     `json:"paste_id"`
	Data      []byte     `json:"data,omitempty"`
	Language  string     `json:"language"`
	Name      string     `json:"name"`
	Expires   *time.Time `json:"expires,omitempty"`
	Public    bool       `json:"public"`
	CreatedAt time.Time  `json:"created_at"`
	Encrypted bool       `json:"encrypted"`
}

// FormattedCreatedAt returns a DD-MM-YY formatted createdAt
// date
func (p Paste) FormattedCreatedAt() string {
	return p.CreatedAt.Format("02-Jan-2006")
}

// FormattedExpires returns a DD-MM-YY formatted expiration
// date
func (p Paste) FormattedExpires() string {
	if p.Expires != nil {
		return p.Expires.Format("02-Jan-2006")
	}
	return ""
}

// PasteListResult holds results for a paste list request
type PasteListResult struct {
	// Total      int64   `json:"total"`
	TotalPages int64   `json:"total_pages"`
	Page       int64   `json:"page"`
	Pastes     []Paste `json:"pastes"`
}

// PasswordLoginParams holds information used during
// password authentication, that will be passed to a
// password login function
type PasswordLoginParams struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ID returns a xxhash (int64) of the username
func (p PasswordLoginParams) ID() int64 {
	if p.Username == "" {
		return 0
	}
	userID, err := util.HashString(p.Username)
	if err != nil {
		return 0
	}
	return int64(userID)
}

// Validate checks if the username and password are set
func (p PasswordLoginParams) Validate() error {
	if p.Username == "" || p.Password == "" {
		return errors.ErrUnauthorized
	}
	return nil
}

// JWTResponse holds the JWT token returned as a result of a
// successful auth
type JWTResponse struct {
	Token string `json:"token"`
}
