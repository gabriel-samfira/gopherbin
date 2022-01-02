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
	Delete(ctx context.Context, pasteID string) error
	ShareWithUser(ctx context.Context, pasteID string, userID string) error
	UnshareWithUser(ctx context.Context, pasteID string, userID string) error
	SetPrivacy(ctx context.Context, pasteID string, public bool) (params.Paste, error)
}

type TeamManager interface {
	Create(ctx context.Context, name string) (team params.Teams, err error)
	Delete(ctx context.Context, name string) error
	Get(ctx context.Context, name string) (team params.Teams, err error)
	List(ctx context.Context, page int64, results int64) (teams []params.Teams, err error)
	AddMember(ctx context.Context, team string, member params.AddTeamMemberRequest) (params.TeamMember, error)
	ListMembers(ctx context.Context, team string, page int64, results int64) ([]params.TeamMember, error)
	RemoveMember(ctx context.Context, team, member string) error
}
