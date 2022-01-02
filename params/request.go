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
	"gopherbin/errors"
	"gopherbin/util"

	zxcvbn "github.com/nbutton23/zxcvbn-go"
)

// NewUserParams holds the needed information to create
// a new user
type NewUserParams struct {
	Email    string `json:"email"`
	Username string `json:"username"`
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

	if !util.IsAlphanumeric(u.Username) {
		return fmt.Errorf("invalid username %s", u.Username)
	}

	if len(u.FullName) == 0 || len(u.FullName) > 255 {
		return fmt.Errorf("full name may not be empty")
	}
	return nil
}

// UpdateUserPayload defines fields that may be updated
// on a user entry
type UpdateUserPayload struct {
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
	Email    *string `json:"email,omitempty"`
}

// Validate validates the object in order to determine
// if the minimum required fields have proper values (email
// is valid, password is of a decent strength etc).
func (u UpdateUserPayload) Validate() error {
	if u.Password != nil {
		passwordStenght := zxcvbn.PasswordStrength(*u.Password, nil)
		if passwordStenght.Score < 4 {
			return errors.NewBadRequestError("the password is too weak, please use a stronger password")
		}
	}

	if u.FullName != nil {
		if len(*u.FullName) == 0 || len(*u.FullName) > 255 {
			return errors.NewBadRequestError("invalid full name")
		}
	}
	return nil
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

// UpdatePasteParams is the payload we can send to update a paste.
type UpdatePasteParams struct {
	Public bool `json:"public"`
}

// NewTeamParams holds information needed to create a new team.
type NewTeamParams struct {
	Name string `json:"name"`
}

// AddTeamMemberRequest is the payload needed to add a new team member.
// Either username or email can be used. Username takes precedence.
type AddTeamMemberRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}
