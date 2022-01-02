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
	"time"
)

// Teams holds information about a team
type Teams struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Owner   TeamMember   `json:"owner"`
	Members []TeamMember `json:"members"`
}

type TeamMember struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

// Users holds information about a particular user
type Users struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	FullName    string    `json:"full_name"`
	Password    *string   `json:"-"`
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

// Paste holds information about a paste
type Paste struct {
	ID          int64             `json:"id"`
	PasteID     string            `json:"paste_id"`
	Data        []byte            `json:"data,omitempty"`
	Preview     []byte            `json:"preview,omitempty"`
	Language    string            `json:"language"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Expires     *time.Time        `json:"expires,omitempty"`
	Public      bool              `json:"public"`
	CreatedAt   time.Time         `json:"created_at"`
	Encrypted   bool              `json:"encrypted"`
	Metadata    map[string]string `json:"metadata"`
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

// JWTResponse holds the JWT token returned as a result of a
// successful auth
type JWTResponse struct {
	Token string `json:"token"`
}
