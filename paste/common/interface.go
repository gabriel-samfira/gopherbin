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
		metadata map[string]string,
		maxAccesses *int,
		labels []string) (paste params.Paste, err error)
	Get(ctx context.Context, pasteID string) (paste params.Paste, err error)
	GetPublicPaste(ctx context.Context, pasteID string) (paste params.Paste, err error)
	// List returns pastes visible to the user. Valid scopes are
	// ScopeMine, ScopeShared and ScopeAll. Labels restricts results to
	// pastes carrying every named label (in the viewer's personal or team
	// label scopes); team restricts results to pastes of the named team.
	List(ctx context.Context, page int64, results int64, scope string, labels []string, team string) (paste params.PasteListResult, err error)
	// Search returns pastes matching query, visible to the user. Valid
	// scopes are ScopeMine, ScopeShared and ScopeAll. Label and team
	// filters apply on top of the text query.
	Search(ctx context.Context, query string, page int64, results int64, scope string, labels []string, team string) (paste params.PasteListResult, err error)
	Delete(ctx context.Context, pasteID string) error
	SetPrivacy(ctx context.Context, pasteID string, public bool) (params.Paste, error)
	// SetLabels replaces the labels attached to a paste. Labels are scoped
	// to the paste's owner (personal pastes) or team (team pastes), and are
	// created on first use.
	SetLabels(ctx context.Context, pasteID string, labels []string) (params.Paste, error)
	// ListLabels returns the label vocabulary usable by the viewer.
	ListLabels(ctx context.Context) (params.LabelVocabulary, error)
	// ListOwnedLabels returns the viewer's personal labels with usage counts.
	ListOwnedLabels(ctx context.Context) ([]params.LabelInfo, error)
	// UpdateLabel renames and/or recolors a label the caller manages.
	UpdateLabel(ctx context.Context, labelID uint, args params.UpdateLabelParams) (params.LabelInfo, error)
	// DeleteLabel removes a managed label from all pastes and its vocabulary.
	DeleteLabel(ctx context.Context, labelID uint) error
	// TransferOwnership transfers a paste to another user. Only the current
	// owner of the paste (or of the owning team) can transfer it. Team pastes
	// can only be transferred to users that are part of the team.
	TransferOwnership(ctx context.Context, pasteID string, userID string) (params.Paste, error)
	ShareWithUser(ctx context.Context, pasteID string, userID string) (params.TeamMember, error)
	UnshareWithUser(ctx context.Context, pasteID string, userID string) error
	ListShares(ctx context.Context, pasteID string) (params.PasteShareListResponse, error)
}

// Paste visibility scopes accepted by List and Search
const (
	// ScopeMine returns only pastes owned by the user
	ScopeMine = "mine"
	// ScopeShared returns pastes shared with the user (directly or via a team)
	ScopeShared = "shared"
	// ScopeAll returns pastes owned by the user, shared with the user, or
	// belonging to a team the user is part of
	ScopeAll = "all"
)

type TeamManager interface {
	// Create creates a new team.
	Create(ctx context.Context, name string, description string) (team params.Teams, err error)
	// Update changes the name and/or description of a team. Owner only.
	Update(ctx context.Context, name string, update params.UpdateTeamParams) (team params.Teams, err error)
	// ListPendingInvites returns the teams the calling user was invited to
	// but has not accepted yet.
	ListPendingInvites(ctx context.Context) ([]params.TeamInviteInfo, error)
	// SetLabels replaces the team-scoped label vocabulary. Owner and active
	// members may manage it.
	SetLabels(ctx context.Context, team string, labels []string) (out params.Teams, err error)
	// Delete will delete a team. All pastes created within this team will also be deleted.
	Delete(ctx context.Context, name string) error
	// Get will return details about a single team.
	Get(ctx context.Context, name string) (team params.Teams, err error)
	// List returns a list of teams created by the user.
	List(ctx context.Context, page int64, results int64) (teams params.TeamListResult, err error)
	// AddMember invites a new member to a team with the given role
	// (admin/member/viewer, default member). The invitee must accept the
	// invitation before gaining access. Owner and admins may invite; only
	// the owner may invite admins.
	AddMember(ctx context.Context, team string, member string, role string) (params.TeamMember, error)
	// SetMemberRole assigns a role to an invited or accepted member. Owner only.
	SetMemberRole(ctx context.Context, team, member string, role string) (params.TeamMember, error)
	// RequestTransfer asks an accepted member to take over ownership; the
	// transfer completes when they accept. Owner only.
	RequestTransfer(ctx context.Context, team string, member string) (params.Teams, error)
	// CancelTransfer withdraws a pending transfer. Owner only.
	CancelTransfer(ctx context.Context, team string) error
	// AcceptTransfer completes a transfer offered to the calling user.
	AcceptTransfer(ctx context.Context, team string) (params.Teams, error)
	// DeclineTransfer rejects a transfer offered to the calling user.
	DeclineTransfer(ctx context.Context, team string) error
	// ListPendingTransfers returns the transfers awaiting the calling user.
	ListPendingTransfers(ctx context.Context) ([]params.TeamTransferInfo, error)
	// AcceptInvite accepts a pending invitation for the calling user.
	AcceptInvite(ctx context.Context, team string) (params.Teams, error)
	// DeclineInvite rejects a pending invitation for the calling user.
	DeclineInvite(ctx context.Context, team string) error
	// LeaveTeam removes the calling user from a team they have joined.
	LeaveTeam(ctx context.Context, team string) error
	// ListMembers returns a list of all users that are part of a team.
	ListMembers(ctx context.Context, team string) ([]params.TeamMember, error)
	// RemoveMember removes a user from a team. Admins may remove members and
	// viewers; only the team owner may remove admins.
	RemoveMember(ctx context.Context, team, member string) error
}
