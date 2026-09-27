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
	"regexp"
	"strings"
	"time"
	"unicode"

	zxcvbn "github.com/nbutton23/zxcvbn-go"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

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
	IsAdmin      *bool   `json:"is_admin,omitempty"`
	Username     *string `json:"username,omitempty"`
	Password     *string `json:"password,omitempty"`
	FullName     *string `json:"full_name,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	Email        *string `json:"email,omitempty"`
	Discoverable *bool   `json:"discoverable,omitempty"`
	// CurrentPassword is the account's existing password. It is required
	// only when a user sets a new Password on their own account; optional
	// so that admin-initiated resets of other users keep working. It is a
	// raw existing secret, so it is deliberately not strength-checked.
	CurrentPassword string `json:"current_password,omitempty"`
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

// NewPasteParams is the payload for creating a paste. Data is the paste
// content (base64 encoded in JSON). Labels accept bare label names as well
// as label objects. Setting Team creates a team paste, which is always
// private.
type NewPasteParams struct {
	Data        []byte            `json:"data"`
	Name        string            `json:"name"`
	Language    string            `json:"language,omitempty"`
	Description string            `json:"description,omitempty"`
	Expires     *time.Time        `json:"expires,omitempty"`
	MaxAccesses *int              `json:"max_accesses,omitempty"`
	Public      bool              `json:"public,omitempty"`
	Team        string            `json:"team,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Labels      []PasteLabel      `json:"labels,omitempty"`
}

// UpdatePasteParams is the payload we can send to update a paste.
type UpdatePasteParams struct {
	Public bool `json:"public"`
}

// NewTeamParams holds information needed to create a new team.
type NewTeamParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Validate checks the team name and description.
func (n NewTeamParams) Validate() error {
	if err := ValidateTeamName(n.Name); err != nil {
		return err
	}
	return validateTeamDescription(n.Description)
}

// reservedTeamNames collide with fixed routes under /teams/.
var reservedTeamNames = map[string]bool{"invites": true, "transfers": true}

// ValidateTeamName checks a team name. Names are used verbatim as a path
// segment (/teams/{name}), so they are limited to letters, digits, spaces,
// dots, dashes and underscores (no "/", "?", "#", "%" or dot segments),
// must start with a letter or digit, and must not shadow a fixed route.
func ValidateTeamName(name string) error {
	runes := []rune(name)
	if len(runes) == 0 || len(runes) > 32 {
		return errors.NewBadRequestError("team name must be 1-32 characters")
	}
	if !unicode.IsLetter(runes[0]) && !unicode.IsDigit(runes[0]) {
		return errors.NewBadRequestError("team name must start with a letter or digit")
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" ._-", r) {
			return errors.NewBadRequestError("team name may only contain letters, digits, spaces, dots, dashes and underscores")
		}
	}
	if strings.TrimSpace(name) != name {
		return errors.NewBadRequestError("team name must not end with a space")
	}
	if reservedTeamNames[strings.ToLower(name)] {
		return errors.NewBadRequestError("%q is a reserved team name", name)
	}
	return nil
}

func validateTeamDescription(description string) error {
	if len([]rune(description)) > 254 {
		return errors.NewBadRequestError("team description must be at most 254 characters")
	}
	return nil
}

// UpdateTeamParams holds the mutable attributes of a team. Both fields are
// optional; only the ones that are set are applied.
type UpdateTeamParams struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// Validate rejects invalid names and oversized descriptions.
func (u UpdateTeamParams) Validate() error {
	if u.Name != nil {
		if err := ValidateTeamName(*u.Name); err != nil {
			return err
		}
	}
	if u.Description != nil {
		return validateTeamDescription(*u.Description)
	}
	return nil
}

// TeamLabelsParams replaces the full set of team-scoped labels.
type TeamLabelsParams struct {
	Labels []string `json:"labels"`
}

// UpdateLabelParams renames and/or recolors a single label. A color of ""
// resets it to the automatic palette color.
type UpdateLabelParams struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// Validate checks the optional fields.
func (u UpdateLabelParams) Validate() error {
	if u.Name == nil && u.Color == nil {
		return errors.NewBadRequestError("nothing to update")
	}
	if u.Color != nil && *u.Color != "" && !hexColorRe.MatchString(*u.Color) {
		return errors.NewBadRequestError("color must be a #rrggbb hex value")
	}
	return nil
}

// PasteLabelsParams replaces the full set of labels attached to a paste.
type PasteLabelsParams struct {
	Labels []string `json:"labels"`
}

// MeSettingsParams is the payload for the authenticated user's own settings.
type MeSettingsParams struct {
	Discoverable *bool `json:"discoverable,omitempty"`
}

// TeamMemberParams invites a user to a team, optionally with a role
// (defaults to member).
type TeamMemberParams struct {
	UserID string `json:"userID"`
	Role   string `json:"role,omitempty"`
}

// Validate checks the optional role.
func (t TeamMemberParams) Validate() error {
	return validateTeamRole(t.Role, true)
}

// SetTeamMemberRoleParams assigns a role to an existing member.
type SetTeamMemberRoleParams struct {
	Role string `json:"role"`
}

// Validate checks the role.
func (s SetTeamMemberRoleParams) Validate() error {
	return validateTeamRole(s.Role, false)
}

// TeamTransferParams names the member ownership should transfer to.
type TeamTransferParams struct {
	UserID string `json:"userID"`
}

func validateTeamRole(role string, allowEmpty bool) error {
	switch role {
	case "":
		if allowEmpty {
			return nil
		}
		return errors.NewBadRequestError("role is required")
	case "admin", "member", "viewer":
		return nil
	}
	return errors.NewBadRequestError("invalid role: use admin, member or viewer")
}

// UserActionRequest is the payload describing a user ID. This user ID
// can be either a username or an email address and is used to add a team
// member or share a paste with a user.
type UserActionRequest struct {
	UserID string `json:"userID"`
}
