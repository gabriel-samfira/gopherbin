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
	"encoding/json"
	"time"
)

// Teams holds information about a team
type Teams struct {
	ID          uint         `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Owner       TeamMember   `json:"owner"`
	Members     []TeamMember `json:"members,omitempty"`
	// MyRole is the relation of the requesting user to this team: "owner",
	// the member role ("admin", "member", "viewer") or "pending" (invited,
	// not yet accepted).
	MyRole       string      `json:"my_role,omitempty"`
	Labels       []string    `json:"labels,omitempty"`
	LabelDetails []LabelInfo `json:"label_details,omitempty"`
	// TransferTo is set while a ownership transfer is awaiting acceptance.
	TransferTo        *TeamMember `json:"transfer_to,omitempty"`
	MyPendingTransfer bool        `json:"my_pending_transfer,omitempty"`
	Stats             *TeamStats  `json:"stats,omitempty"`
	CreatedAt         *time.Time  `json:"created_at,omitempty"`
}

// TeamTransferInfo is one ownership transfer awaiting the calling user's
// decision.
type TeamTransferInfo struct {
	TeamID   uint   `json:"team_id"`
	TeamName string `json:"team_name"`
	FromUser string `json:"from_user,omitempty"`
}

// TeamInviteInfo is one pending invitation of the calling user.
type TeamInviteInfo struct {
	TeamID    uint   `json:"team_id"`
	TeamName  string `json:"team_name"`
	InvitedBy string `json:"invited_by,omitempty"`
}

// TeamStats carries aggregate counters for a team page.
type TeamStats struct {
	Members      int `json:"members"`
	Pending      int `json:"pending"`
	Pastes       int `json:"pastes"`
	Contributors int `json:"contributors"`
}

// PasteLabel is a label attached to (or requested for) a paste. Scope is
// "personal" or "team"; Team is set for team-scoped labels.
type PasteLabel struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Scope string `json:"scope,omitempty"`
	Team  string `json:"team,omitempty"`
}

// LabelInfo is one entry of a label-management list (Settings page, team
// labels section). Usage is the number of pastes currently carrying it.
type LabelInfo struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Usage int64  `json:"usage"`
}

// UserSearchResult is one entry of the invite type-ahead. Emails are
// deliberately not returned.
type UserSearchResult struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
}

type TeamMember struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	FullName  string `json:"full_name"`
	Status    string `json:"status,omitempty"`
	Role      string `json:"role,omitempty"`
	AddedByID uint   `json:"added_by_id,omitempty"`
	AddedBy   string `json:"added_by,omitempty"`
}

// Users holds information about a particular user
type Users struct {
	ID           uint      `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	FullName     string    `json:"full_name"`
	Password     *string   `json:"-"`
	Enabled      bool      `json:"enabled"`
	IsAdmin      bool      `json:"is_admin"`
	IsSuperUser  bool      `json:"is_superuser"`
	Discoverable bool      `json:"discoverable"`
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

// Paste holds information about a paste
type Paste struct {
	ID          uint              `json:"id"`
	PasteID     string            `json:"paste_id"`
	Data        []byte            `json:"data,omitempty"`
	Preview     []byte            `json:"preview,omitempty"`
	Language    string            `json:"language"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Expires     *time.Time        `json:"expires,omitempty"`
	MaxAccesses *int              `json:"max_accesses,omitempty"`
	AccessCount int               `json:"access_count,omitempty"`
	Public      bool              `json:"public"`
	CreatedAt   time.Time         `json:"created_at"`
	CreatedBy   string            `json:"created_by"`
	Owner       string            `json:"owner"`
	OwnerID     uint              `json:"owner_id"`
	Team        string            `json:"team,omitempty"`
	Metadata    map[string]string `json:"metadata"`
	Labels      []PasteLabel      `json:"labels,omitempty"`
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
	TotalPages int64   `json:"total_pages"`
	Page       int64   `json:"page"`
	Pastes     []Paste `json:"pastes"`
}

// TeamListResult holds results for a team list request
type TeamListResult struct {
	TotalPages int64   `json:"total_pages"`
	Page       int64   `json:"page"`
	Teams      []Teams `json:"teams"`
}

type PasteShareListResponse struct {
	Users []TeamMember `json:"users"`
}

// JWTResponse holds the JWT token returned as a result of a
// successful auth
type JWTResponse struct {
	Token string `json:"token"`
}

// TeamLabelGroup is a team's set of labels.
type TeamLabelGroup struct {
	Team   string   `json:"team"`
	Labels []string `json:"labels"`
}

// LabelVocabulary is the set of labels a viewer can attach or filter by.
type LabelVocabulary struct {
	Personal []string         `json:"personal"`
	Teams    []TeamLabelGroup `json:"teams"`
	// Colors maps label name to a custom #rrggbb color across the whole
	// vocabulary, so any label badge can render consistently.
	Colors map[string]string `json:"colors"`
}

// UnmarshalJSON accepts either a bare label name string (the shape sent when
// creating or updating a paste) or the full {"name": ...} object returned by
// the API.
func (l *PasteLabel) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		l.Name = name
		return nil
	}
	type alias PasteLabel
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*l = PasteLabel(a)
	return nil
}
