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

// This file exists solely to document the REST API for go-swagger. Each
// swagger:route block maps an HTTP endpoint (as registered in
// apiserver/routers/routers.go) onto its request/response models, which are
// declared with x-go-type bindings in apiserver/swagger-models.yaml. Run
// "go generate ./..." (or make generate) after changing handlers to
// regenerate apiserver/swagger.yaml, then regenerate the webapp client.
package controllers

// ---------------------------------------------------------------------------
// System / bootstrap
//
// The first-run, login and public paste endpoints are registered on mux
// groups (see apiserver/routers/routers.go) that do not run the JWT
// middleware. They carry the x-public extension, which cmd/apigen turns into
// `security: []` so they are exempt from the global Bearer requirement.
// ---------------------------------------------------------------------------

// swagger:route POST /first-run system firstRun
//
// Bootstrap the installation by creating the initial superuser. Only succeeds
// while the instance has no superuser; every subsequent call returns 409.
//
//	Parameters:
//	  + name: Body
//	    description: Initial superuser.
//	    in: body
//	    type: NewUserParams
//	    required: true
//
//
//	Responses:
//	  200: Users
//	  400: APIErrorResponse
//	  409: APIErrorResponse
//
//	Extensions:
//	  x-public: true
const opFirstRun = "firstRun"

// swagger:route POST /auth/login auth login
//
// Exchange username/email and password for a JWT token.
//
//	Parameters:
//	  + name: Body
//	    description: Login credentials.
//	    in: body
//	    type: PasswordLoginParams
//	    required: true
//
//
//	Responses:
//	  200: JWTResponse
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//
//	Extensions:
//	  x-public: true
const opLogin = "login"

// swagger:route GET /logout auth logout
//
// Blacklist the caller's current JWT token, ending the session.
//
//	Responses:
//	  200: OK

// ---------------------------------------------------------------------------
// Public pastes
// ---------------------------------------------------------------------------

// swagger:route GET /public/paste/{pasteID} pastes publicPasteView
//
// Fetch a paste that has been marked public, without authentication.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: X-Consume-Access
//	    description: Any non-empty value confirms a view of a paste with limited views (max_accesses), which consumes one of them. Without it such pastes answer 403 and nothing is consumed.
//	    in: header
//	    type: string
//	    required: false
//
//	Responses:
//	  200: Paste
//	  403: APIErrorResponse
//	  404: APIErrorResponse
//
//	Extensions:
//	  x-public: true
const opPublicPasteView = "publicPasteView"

// ---------------------------------------------------------------------------
// Pastes
// ---------------------------------------------------------------------------

// swagger:route GET /paste pastes listPastes
//
// List pastes visible to the caller, filtered by the optional query
// parameters.
//
//	Parameters:
//	  + name: page
//	    description: Page number, starting at 1.
//	    in: query
//	    type: integer
//	    required: false
//	  + name: max_results
//	    description: Maximum results per page.
//	    in: query
//	    type: integer
//	    required: false
//	  + name: scope
//	    description: Restrict the listing to a scope (e.g. user or team).
//	    in: query
//	    type: string
//	    required: false
//	  + name: team
//	    description: Restrict the listing to a single team.
//	    in: query
//	    type: string
//	    required: false
//	  + name: labels
//	    description: Comma separated label names the pastes must carry.
//	    in: query
//	    type: string
//	    required: false
//
//	Responses:
//	  200: PasteListResult
//	  401: APIErrorResponse
const opListPastes = "listPastes"

// swagger:route POST /paste pastes createPaste
//
// Create a new paste. The content is provided base64 encoded.
//
//	Parameters:
//	  + name: Body
//	    description: Paste payload.
//	    in: body
//	    type: NewPasteParams
//	    required: true
//
//	Responses:
//	  200: Paste
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opCreatePaste = "createPaste"

// swagger:route GET /paste/{pasteID} pastes getPaste
//
// Fetch a single paste the caller is allowed to see.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: X-Consume-Access
//	    description: Any non-empty value confirms a view of a paste with limited views (max_accesses), which consumes one of them. Without it such pastes answer 403 and nothing is consumed.
//	    in: header
//	    type: string
//	    required: false
//
//	Responses:
//	  200: Paste
//	  401: APIErrorResponse
//	  403: APIErrorResponse
//	  404: APIErrorResponse
const opGetPaste = "getPaste"

// swagger:route GET /paste/{pasteID}/download pastes downloadPaste
//
// Download the raw paste content as an attachment.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: X-Consume-Access
//	    description: Any non-empty value confirms a view of a paste with limited views (max_accesses), which consumes one of them. Without it such pastes answer 403 and nothing is consumed.
//	    in: header
//	    type: string
//	    required: false
//
//	Responses:
//	  200: file
//	  401: APIErrorResponse
//	  403: APIErrorResponse
//	  404: APIErrorResponse
const opDownloadPaste = "downloadPaste"

// swagger:route PUT /paste/{pasteID} pastes updatePaste
//
// Update the mutable fields of an existing paste.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Fields to update.
//	    in: body
//	    type: UpdatePasteParams
//	    required: true
//
//	Responses:
//	  200: Paste
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opUpdatePaste = "updatePaste"

// swagger:route DELETE /paste/{pasteID} pastes deletePaste
//
// Delete a paste the caller owns.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opDeletePaste = "deletePaste"

// swagger:route GET /paste/search pastes searchPastes
//
// Full-text search over the pastes visible to the caller.
//
//	Parameters:
//	  + name: q
//	    description: Search query.
//	    in: query
//	    type: string
//	    required: true
//	  + name: page
//	    description: Page number, starting at 1.
//	    in: query
//	    type: integer
//	    required: false
//	  + name: max_results
//	    description: Maximum results per page.
//	    in: query
//	    type: integer
//	    required: false
//	  + name: scope
//	    description: Restrict the search to a scope (e.g. user or team).
//	    in: query
//	    type: string
//	    required: false
//	  + name: team
//	    description: Restrict the search to a single team.
//	    in: query
//	    type: string
//	    required: false
//	  + name: labels
//	    description: Comma separated label names the pastes must carry.
//	    in: query
//	    type: string
//	    required: false
//
//	Responses:
//	  200: PasteListResult
//	  400: APIErrorResponse
//	  401: APIErrorResponse
const opSearchPastes = "searchPastes"

// swagger:route POST /paste/{pasteID}/transfer pastes transferPaste
//
// Transfer ownership of a paste to another user.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Target user.
//	    in: body
//	    type: UserActionRequest
//	    required: true
//
//	Responses:
//	  200: Paste
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opTransferPaste = "transferPaste"

// swagger:route POST /paste/{pasteID}/sharing pastes sharePaste
//
// Grant a user access to a private paste.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: User to share with (username or email in userID).
//	    in: body
//	    type: UserActionRequest
//	    required: true
//
//	Responses:
//	  200: TeamMember
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opSharePaste = "sharePaste"

// swagger:route GET /paste/{pasteID}/sharing pastes listPasteShares
//
// List the users a paste is shared with.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: PasteShareListResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opListPasteShares = "listPasteShares"

// swagger:route DELETE /paste/{pasteID}/sharing/{userID} pastes unsharePaste
//
// Revoke a user's access to a paste.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: userID
//	    description: User identifier.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opUnsharePaste = "unsharePaste"

// swagger:route PUT /paste/{pasteID}/labels pastes setPasteLabels
//
// Replace the label set attached to a paste.
//
//	Parameters:
//	  + name: pasteID
//	    description: Paste identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Desired label names.
//	    in: body
//	    type: PasteLabelsParams
//	    required: true
//
//	Responses:
//	  200: Paste
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opSetPasteLabels = "setPasteLabels"

// ---------------------------------------------------------------------------
// Labels
// ---------------------------------------------------------------------------

// swagger:route GET /labels labels listLabels
//
// The label vocabulary visible to the caller, with usage counts.
//
//	Responses:
//	  200: LabelVocabulary
//	  401: APIErrorResponse
const opListLabels = "listLabels"

// swagger:route GET /labels/mine labels listOwnedLabels
//
// Labels owned by the caller.
//
//	Responses:
//	  200: LabelInfoArray
//	  401: APIErrorResponse
const opListOwnedLabels = "listOwnedLabels"

// swagger:route PUT /labels/{labelID} labels updateLabel
//
// Rename a label or change its custom color.
//
//	Parameters:
//	  + name: labelID
//	    description: Numeric label identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Label fields to update.
//	    in: body
//	    type: UpdateLabelParams
//	    required: true
//
//	Responses:
//	  200: LabelInfo
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opUpdateLabel = "updateLabel"

// swagger:route DELETE /labels/{labelID} labels deleteLabel
//
// Delete a label owned by the caller.
//
//	Parameters:
//	  + name: labelID
//	    description: Numeric label identifier.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opDeleteLabel = "deleteLabel"

// ---------------------------------------------------------------------------
// Users (self-service)
// ---------------------------------------------------------------------------

// swagger:route GET /users/search users searchUsers
//
// Type-ahead search over discoverable users, optionally excluding the members
// of a team.
//
//	Parameters:
//	  + name: q
//	    description: Username or email prefix to search for.
//	    in: query
//	    type: string
//	    required: true
//	  + name: team
//	    description: Exclude members of this team from the results.
//	    in: query
//	    type: string
//	    required: false
//
//	Responses:
//	  200: UserSearchResultArray
//	  401: APIErrorResponse
const opSearchUsers = "searchUsers"

// swagger:route GET /me users getMe
//
// The account record of the authenticated caller.
//
//	Responses:
//	  200: Users
//	  401: APIErrorResponse
const opGetMe = "getMe"

// swagger:route PUT /me users updateMe
//
// Update the caller's own settings.
//
//	Parameters:
//	  + name: Body
//	    description: Settings to update.
//	    in: body
//	    type: MeSettingsParams
//	    required: true
//
//	Responses:
//	  200: Users
//	  400: APIErrorResponse
//	  401: APIErrorResponse
const opUpdateMe = "updateMe"

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

// swagger:route GET /teams teams listTeams
//
// List the teams the caller belongs to.
//
//	Parameters:
//	  + name: page
//	    description: Page number, starting at 1.
//	    in: query
//	    type: integer
//	    required: false
//	  + name: max_results
//	    description: Maximum results per page.
//	    in: query
//	    type: integer
//	    required: false
//
//	Responses:
//	  200: TeamListResult
//	  401: APIErrorResponse
const opListTeams = "listTeams"

// swagger:route POST /teams teams createTeam
//
// Create a new team owned by the caller.
//
//	Parameters:
//	  + name: Body
//	    description: Team name and description.
//	    in: body
//	    type: NewTeamParams
//	    required: true
//
//	Responses:
//	  200: Teams
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  409: APIErrorResponse
const opCreateTeam = "createTeam"

// swagger:route GET /teams/invites teams listTeamInvites
//
// Team invitations currently pending on the caller.
//
//	Responses:
//	  200: TeamInviteInfoArray
//	  401: APIErrorResponse
const opListTeamInvites = "listTeamInvites"

// swagger:route GET /teams/transfers teams listTeamTransfers
//
// Pending team-ownership offers addressed to the caller.
//
//	Responses:
//	  200: TeamTransferInfoArray
//	  401: APIErrorResponse
const opListTeamTransfers = "listTeamTransfers"

// swagger:route GET /teams/{teamName} teams getTeam
//
// Fetch a team the caller is a member of.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: Teams
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opGetTeam = "getTeam"

// swagger:route PUT /teams/{teamName} teams updateTeam
//
// Rename a team or change its description (owner only).
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Team fields to update.
//	    in: body
//	    type: UpdateTeamParams
//	    required: true
//
//	Responses:
//	  200: Teams
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
//	  409: APIErrorResponse
const opUpdateTeam = "updateTeam"

// swagger:route DELETE /teams/{teamName} teams deleteTeam
//
// Delete a team (owner only).
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opDeleteTeam = "deleteTeam"

// swagger:route POST /teams/{teamName}/members teams addTeamMember
//
// Invite a user to the team (owner or admin). The invite is pending until the
// target user accepts it.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: User (username/email) and role of the new member.
//	    in: body
//	    type: TeamMemberParams
//	    required: true
//
//	Responses:
//	  200: TeamMember
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opAddTeamMember = "addTeamMember"

// swagger:route GET /teams/{teamName}/members teams listTeamMembers
//
// List the members of a team, including pending invites.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: TeamMemberArray
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opListTeamMembers = "listTeamMembers"

// swagger:route PUT /teams/{teamName}/members/{member} teams setTeamMemberRole
//
// Change a member's role (owner only).
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: member
//	    description: Member username.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: New role.
//	    in: body
//	    type: SetTeamMemberRoleParams
//	    required: true
//
//	Responses:
//	  200: TeamMember
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opSetTeamMemberRole = "setTeamMemberRole"

// swagger:route DELETE /teams/{teamName}/members/{member} teams removeTeamMember
//
// Remove a member from the team (owner or admin, subject to role rules).
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: member
//	    description: Member username.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opRemoveTeamMember = "removeTeamMember"

// swagger:route POST /teams/{teamName}/accept teams acceptTeamInvite
//
// Accept a pending team invitation addressed to the caller.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: Teams
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opAcceptTeamInvite = "acceptTeamInvite"

// swagger:route POST /teams/{teamName}/decline teams declineTeamInvite
//
// Decline a pending team invitation addressed to the caller.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opDeclineTeamInvite = "declineTeamInvite"

// swagger:route POST /teams/{teamName}/leave teams leaveTeam
//
// Leave a team the caller belongs to.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opLeaveTeam = "leaveTeam"

// swagger:route PUT /teams/{teamName}/labels teams setTeamLabels
//
// Replace the label vocabulary of a team (owner or admin).
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Desired label names.
//	    in: body
//	    type: TeamLabelsParams
//	    required: true
//
//	Responses:
//	  200: Teams
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opSetTeamLabels = "setTeamLabels"

// swagger:route POST /teams/{teamName}/transfer teams transferTeam
//
// Offer team ownership to one of the team members (owner only). The offer
// takes effect only once the target accepts it.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: Offer target.
//	    in: body
//	    type: TeamTransferParams
//	    required: true
//
//	Responses:
//	  200: Teams
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opTransferTeam = "transferTeam"

// swagger:route POST /teams/{teamName}/transfer/{action} teams teamTransferAction
//
// Act on a pending ownership transfer: accept, decline or cancel.
//
//	Parameters:
//	  + name: teamName
//	    description: Team name.
//	    in: path
//	    type: string
//	    required: true
//	  + name: action
//	    description: Transfer action.
//	    in: path
//	    type: string
//	    enum: accept,decline,cancel
//	    required: true
//
//	Responses:
//	  200: Teams
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opTeamTransferAction = "teamTransferAction"

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

// swagger:route GET /admin/users admin listUsers
//
// List all user accounts (admin only).
//
//	Parameters:
//	  + name: page
//	    description: Page number, starting at 1.
//	    in: query
//	    type: integer
//	    required: false
//	  + name: max_results
//	    description: Maximum results per page.
//	    in: query
//	    type: integer
//	    required: false
//
//	Responses:
//	  200: UserListResult
//	  401: APIErrorResponse
const opListUsers = "listUsers"

// swagger:route POST /admin/users admin createUser
//
// Create a user account (admin only).
//
//	Parameters:
//	  + name: Body
//	    description: New user.
//	    in: body
//	    type: NewUserParams
//	    required: true
//
//	Responses:
//	  200: Users
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  409: APIErrorResponse
const opCreateUser = "createUser"

// swagger:route GET /admin/users/{userID} admin getUser
//
// Fetch a single user account (admin only).
//
//	Parameters:
//	  + name: userID
//	    description: Numeric user identifier.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: Users
//	  401: APIErrorResponse
//	  404: APIErrorResponse
const opGetUser = "getUser"

// swagger:route PUT /admin/users/{userID} admin updateUser
//
// Update a user account (admin only).
//
//	Parameters:
//	  + name: userID
//	    description: Numeric user identifier.
//	    in: path
//	    type: string
//	    required: true
//	  + name: Body
//	    description: User fields to update.
//	    in: body
//	    type: UpdateUserPayload
//	    required: true
//
//	Responses:
//	  200: Users
//	  400: APIErrorResponse
//	  401: APIErrorResponse
//	  404: APIErrorResponse
//	  409: APIErrorResponse
const opUpdateUser = "updateUser"

// swagger:route DELETE /admin/users/{userID} admin deleteUser
//
// Delete a user account (admin only).
//
//	Parameters:
//	  + name: userID
//	    description: Numeric user identifier.
//	    in: path
//	    type: string
//	    required: true
//
//	Responses:
//	  200: OK
//	  401: APIErrorResponse
//	  404: APIErrorResponse
//	  409: APIErrorResponse
const opDeleteUser = "deleteUser"
