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

package common

import (
	"context"
	"time"

	"gopherbin/params"
)

// Paster is the interface for pastes
type Paster interface {
	Create(
		ctx context.Context, data []byte,
		title, language, description string,
		expires *time.Time,
		isPublic bool, team string,
		metadata map[string]string) (paste params.Paste, err error)
	Get(ctx context.Context, pasteID string) (paste params.Paste, err error)
	GetPublicPaste(ctx context.Context, pasteID string) (paste params.Paste, err error)
	List(ctx context.Context, page int64, results int64) (paste params.PasteListResult, err error)
	Search(ctx context.Context, query string, page int64, results int64) (paste params.PasteListResult, err error)
	Delete(ctx context.Context, pasteID string) error
	SetPrivacy(ctx context.Context, pasteID string, public bool) (params.Paste, error)
	ShareWithUser(ctx context.Context, pasteID string, userID string) (params.TeamMember, error)
	UnshareWithUser(ctx context.Context, pasteID string, userID string) error
	ListShares(ctx context.Context, pasteID string) (params.PasteShareListResponse, error)
}

type TeamManager interface {
	// Create creates a new team.
	Create(ctx context.Context, name string) (team params.Teams, err error)
	// Delete will delete a team. All pastes created within this team will also be deleted.
	Delete(ctx context.Context, name string) error
	// Get will return details about a single team.
	Get(ctx context.Context, name string) (team params.Teams, err error)
	// List returns a list of teams created by the user.
	List(ctx context.Context, page int64, results int64) (teams params.TeamListResult, err error)
	// AddMember adds a new member to a team. Only the owner of the team can add or remove members.
	AddMember(ctx context.Context, team string, member string) (params.TeamMember, error)
	// ListMembers returns a list of all users that are part of a team.
	ListMembers(ctx context.Context, team string) ([]params.TeamMember, error)
	// RemoveMember removes a user from a team. Only the owner of the team can add or remove members.
	RemoveMember(ctx context.Context, team, member string) error
}
