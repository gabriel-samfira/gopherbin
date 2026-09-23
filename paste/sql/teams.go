package sql

import (
	"context"
	"math"

	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	"gopherbin/paste/common"
	"gopherbin/util"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func NewTeamManager(dbCfg config.Database) (common.TeamManager, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}

	p := &teamManager{
		conn: db,
	}

	return p, nil
}

type teamManager struct {
	conn *gorm.DB
}

// TODO: dedup user lookup. Use the admin.UserManager?
func (t *teamManager) getUserByUsernameOrEmail(userID string) (models.Users, error) {
	isEmail := util.IsValidEmail(userID)
	var tmpUser models.Users
	queryString := "username = ?"
	if isEmail {
		queryString = "email = ?"
	}

	q := t.conn.Where(queryString, userID).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (t *teamManager) getUser(userID uint) (models.Users, error) {
	// TODO: abstract this into a common interface
	var tmpUser models.Users
	q := t.conn.Where("id = ?", userID).First(&tmpUser)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Users{}, gErrors.ErrNotFound
		}
		return models.Users{}, errors.Wrap(q.Error, "fetching user from database")
	}
	return tmpUser, nil
}

func (p *teamManager) getUserFromContext(ctx context.Context) (models.Users, error) {
	if auth.IsAnonymous(ctx) || !auth.IsEnabled(ctx) {
		return models.Users{}, gErrors.ErrUnauthorized
	}
	userID := auth.UserID(ctx)
	user, err := p.getUser(userID)
	if err != nil {
		return models.Users{}, errors.Wrap(err, "fetching user")
	}
	return user, nil
}

// membership returns the team_users join row for a user, or nil if the user
// has no relation to the team (and is not the owner).
func (t *teamManager) membership(teamID, userID uint) (*models.TeamUser, error) {
	var row models.TeamUser
	q := t.conn.Where("teams_id = ? AND users_id = ?", teamID, userID).First(&row)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(q.Error, "fetching team membership")
	}
	return &row, nil
}

// isActiveMember returns true if the user is an accepted member of the team.
// Invited (pending) users are not members yet.
func (t *teamManager) isActiveMember(teamID, userID uint) bool {
	row, err := t.membership(teamID, userID)
	if err != nil {
		return false
	}
	return row != nil && row.Status == models.TeamMembershipActive
}

// canAccess returns true if the user may view the team: the owner, any
// accepted member, or a user with a pending invitation (so they can decide
// whether to accept it).
func (t *teamManager) canAccess(team models.Teams, user models.Users) bool {
	if team.OwnerID == user.ID {
		return true
	}
	for _, val := range team.Members {
		if user.ID == val.ID {
			return true
		}
	}
	return false
}

// isMember returns true if the user owns the team or is an accepted member.
// It is also used by the paster to gate team-paste access.
func (t *teamManager) isMember(team models.Teams, user models.Users) bool {
	if team.OwnerID == user.ID {
		return true
	}
	return t.isActiveMember(team.ID, user.ID)
}

// effectiveRole normalizes a stored role; rows created before roles existed
// (or with an unset role) are plain members.
func effectiveRole(role string) string {
	switch role {
	case models.RoleAdmin:
		return models.RoleAdmin
	case models.RoleViewer:
		return models.RoleViewer
	default:
		return models.RoleMember
	}
}

// teamRole returns the effective role of a user in a team: owner, admin,
// member, viewer, "pending" for unanswered invitations, or "" when the user
// has no relation to the team.
func (t *teamManager) teamRole(team models.Teams, userID uint) string {
	if team.OwnerID == userID {
		return models.RoleOwner
	}
	row, err := t.membership(team.ID, userID)
	if err != nil || row == nil {
		return ""
	}
	if row.Status == models.TeamMembershipPending {
		return models.TeamMembershipPending
	}
	return effectiveRole(row.Role)
}

// canManageMembers reports whether the user may invite members or remove
// them: the team owner and members with the admin role.
func (t *teamManager) canManageMembers(team models.Teams, userID uint) bool {
	role := t.teamRole(team, userID)
	return role == models.RoleOwner || role == models.RoleAdmin
}

// canShareToTeam reports whether the user may share team-scoped pastes: the
// owner and any accepted member except viewers.
func (t *teamManager) canShareToTeam(team models.Teams, user models.Users) bool {
	if team.OwnerID == user.ID {
		return true
	}
	row, err := t.membership(team.ID, user.ID)
	if err != nil || row == nil || row.Status != models.TeamMembershipActive {
		return false
	}
	return effectiveRole(row.Role) != models.RoleViewer
}

func (t *teamManager) get(name string) (models.Teams, error) {
	var teamModel models.Teams
	q := t.conn.Preload("Members").Preload("Owner").Preload("TransferTo").Where("name = ?", name).First(&teamModel)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return models.Teams{}, gErrors.ErrNotFound
		}
		return models.Teams{}, errors.Wrap(q.Error, "fetching team from database")
	}

	return teamModel, nil
}

func (t *teamManager) getTeam(ctx context.Context, name string) (models.Teams, error) {
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return models.Teams{}, errors.Wrap(err, "fetching user from context")
	}

	team, err := t.get(name)
	if err != nil {
		return models.Teams{}, errors.Wrap(err, "fetching team")
	}

	if !t.canAccess(team, user) {
		return models.Teams{}, errors.Wrap(gErrors.ErrUnauthorized, "accessing team")
	}
	return team, nil
}

func sqlUserToTeamMember(user models.Users) params.TeamMember {
	return params.TeamMember{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		FullName: user.FullName,
	}
}

// userDisplay renders a user as "Full Name (username)" or just the username.
func userDisplay(user models.Users) string {
	if user.FullName != "" {
		return user.FullName + " (" + user.Username + ")"
	}
	return user.Username
}

// sqlToCommonTeams renders a team for a viewer. joinRows maps user IDs to
// their team_users rows (may be empty); addedBy maps user IDs to the user who
// added or invited them (may be empty). myStatus is the viewer's own status
// in the team (used in preview mode, where member details are omitted).
// preview skips the member list.
func (t *teamManager) sqlToCommonTeams(team models.Teams, viewerID uint, joinRows map[uint]models.TeamUser, addedBy map[uint]models.Users, myStatus string, preview bool) params.Teams {
	var members []params.TeamMember = []params.TeamMember{}
	myRole := ""
	if team.OwnerID == viewerID {
		myRole = models.RoleOwner
	}
	if !preview {
		members = make([]params.TeamMember, len(team.Members))
		for idx, val := range team.Members {
			member := sqlUserToTeamMember(*val)
			member.Status = models.TeamMembershipActive
			member.Role = models.RoleMember
			if row, ok := joinRows[val.ID]; ok {
				member.Status = row.Status
				member.Role = effectiveRole(row.Role)
				if row.AddedByID != nil {
					member.AddedByID = *row.AddedByID
					if adder, ok := addedBy[*row.AddedByID]; ok {
						member.AddedBy = userDisplay(adder)
					}
				}
			}
			if val.ID == viewerID {
				myRole = member.Status
				if member.Status == models.TeamMembershipActive {
					myRole = member.Role
				}
			}
			members[idx] = member
		}
	} else if myRole == "" && myStatus != "" {
		myRole = myStatus
	}
	out := params.Teams{
		ID:          team.ID,
		Name:        team.Name,
		Description: team.Description,
		Owner:       sqlUserToTeamMember(team.Owner),
		Members:     members,
		MyRole:      myRole,
	}
	if team.TransferTo != nil && !preview {
		transfer := sqlUserToTeamMember(*team.TransferTo)
		transfer.Status = models.TeamMembershipActive
		transfer.Role = t.teamRole(team, team.TransferTo.ID)
		out.TransferTo = &transfer
		out.MyPendingTransfer = team.TransferToUserID != nil && *team.TransferToUserID == viewerID
	}
	return out
}

// fetchMembershipData loads the join rows for a team and resolves the users
// who added/invited each member.
func (t *teamManager) fetchMembershipData(teamID uint) (map[uint]models.TeamUser, map[uint]models.Users, error) {
	var rows []models.TeamUser
	if err := t.conn.Where("teams_id = ?", teamID).Find(&rows).Error; err != nil {
		return nil, nil, errors.Wrap(err, "fetching team memberships")
	}
	joinRows := make(map[uint]models.TeamUser, len(rows))
	adderIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		joinRows[row.UserID] = row
		if row.AddedByID != nil {
			adderIDs = append(adderIDs, *row.AddedByID)
		}
	}
	addedBy := map[uint]models.Users{}
	if len(adderIDs) > 0 {
		var adders []models.Users
		if err := t.conn.Where("id IN ?", adderIDs).Find(&adders).Error; err != nil {
			return nil, nil, errors.Wrap(err, "fetching team membership inviters")
		}
		for _, adder := range adders {
			addedBy[adder.ID] = adder
		}
	}
	return joinRows, addedBy, nil
}

func (t *teamManager) Create(ctx context.Context, name string, description string) (params.Teams, error) {
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}
	_, err = t.get(name)
	if err != nil {
		if !errors.Is(err, gErrors.ErrNotFound) {
			return params.Teams{}, errors.Wrap(err, "creating team")
		}
	} else {
		return params.Teams{}, errors.Wrap(gErrors.ErrDuplicateEntity, "creating team")
	}

	team := models.Teams{
		OwnerID:     user.ID,
		Owner:       user,
		Name:        name,
		Description: description,
	}

	q := t.conn.Create(&team)
	if q.Error != nil {
		return params.Teams{}, errors.Wrap(q.Error, "creating team")
	}

	return t.sqlToCommonTeams(team, user.ID, nil, nil, "", true), nil
}

// Delete removes a team: it deletes all pastes belonging to the team and
// disassociates every member (accepted and pending).
func (t *teamManager) Delete(ctx context.Context, name string) error {
	team, err := t.getTeam(ctx, name)
	if err != nil {
		if !errors.Is(err, gErrors.ErrNotFound) {
			return errors.Wrap(err, "looking up team")
		}
		return nil
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user from context")
	}

	if team.OwnerID != user.ID {
		return errors.Wrap(gErrors.ErrUnauthorized, "accessing team")
	}

	err = t.conn.Transaction(func(tx *gorm.DB) error {
		// Delete the team's pastes explicitly: foreign key cascades are not
		// guaranteed to exist on databases created before the constraints
		// were added to the model.
		if err := tx.Unscoped().Where("team_id = ?", team.ID).Delete(&models.Paste{}).Error; err != nil {
			return errors.Wrap(err, "deleting team pastes")
		}
		// Remove join rows that FK cascades may not cover on older databases.
		if err := tx.Exec("DELETE FROM paste_labels WHERE paste_id IN (SELECT id FROM pastes WHERE team_id = ?)", team.ID).Error; err != nil {
			return errors.Wrap(err, "clearing team paste labels")
		}
		if err := tx.Exec("DELETE FROM paste_users WHERE paste_id IN (SELECT id FROM pastes WHERE team_id = ?)", team.ID).Error; err != nil {
			return errors.Wrap(err, "clearing team paste shares")
		}
		if err := tx.Where("teams_id = ?", team.ID).Delete(&models.TeamUser{}).Error; err != nil {
			return errors.Wrap(err, "disassociating team members")
		}
		if err := tx.Model(&team).Association("Members").Clear(); err != nil {
			return errors.Wrap(err, "clearing team members")
		}
		if err := tx.Delete(&team).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Wrap(err, "deleting team")
		}
		return nil
	})
	return err
}

// Update changes the name and/or description of a team. Only the owner may
// update a team. Renaming keeps the team's ID, so membership, labels and
// pastes follow automatically; only URLs referencing the old name break.
func (t *teamManager) Update(ctx context.Context, name string, update params.UpdateTeamParams) (params.Teams, error) {
	if err := update.Validate(); err != nil {
		return params.Teams{}, errors.Wrap(err, "validating team update")
	}
	team, err := t.get(name)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}
	if team.OwnerID != user.ID {
		return params.Teams{}, errors.Wrap(gErrors.ErrUnauthorized, "updating team")
	}

	if update.Name != nil && *update.Name != team.Name {
		newName := *update.Name
		if _, dupErr := t.get(newName); dupErr == nil {
			return params.Teams{}, errors.Wrap(gErrors.ErrDuplicateEntity, "renaming team")
		}
		team.Name = newName
	}
	if update.Description != nil {
		team.Description = *update.Description
	}

	if err := t.conn.Save(&team).Error; err != nil {
		return params.Teams{}, errors.Wrap(err, "saving team")
	}
	team, err = t.get(team.Name)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "re-fetching team")
	}
	joinRows, addedBy, err := t.fetchMembershipData(team.ID)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching membership data")
	}
	return t.sqlToCommonTeams(team, user.ID, joinRows, addedBy, "", false), nil
}

// SetLabels replaces the full set of team-scoped labels. Any active member
// (or the owner) may manage the shared team label vocabulary.
func (t *teamManager) SetLabels(ctx context.Context, teamName string, names []string) (params.Teams, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}
	if !t.isMember(team, user) {
		return params.Teams{}, errors.Wrap(gErrors.ErrUnauthorized, "managing team labels")
	}

	clean, err := dedupeLabels(names)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "validating labels")
	}

	err = t.conn.Transaction(func(tx *gorm.DB) error {
		var current []models.Label
		if err := tx.Where("team_id = ?", team.ID).Find(&current).Error; err != nil {
			return errors.Wrap(err, "listing team labels")
		}
		currentNames := map[string]uint{}
		for _, l := range current {
			currentNames[l.Name] = l.ID
		}
		for _, name := range clean {
			if _, ok := currentNames[name]; ok {
				continue
			}
			if _, err := getOrCreateLabel(tx, name, 0, team.ID); err != nil {
				return err
			}
		}
		for _, l := range current {
			found := false
			for _, name := range clean {
				if name == l.Name {
					found = true
					break
				}
			}
			if !found {
				// Drop the label entirely: it is team-scoped, so no personal
				// paste can reference it. Clean join rows explicitly for
				// databases created before FK cascades existed.
				if err := tx.Exec("DELETE FROM paste_labels WHERE label_id = ?", l.ID).Error; err != nil {
					return errors.Wrap(err, "clearing label associations")
				}
				if err := tx.Unscoped().Delete(&models.Label{}, l.ID).Error; err != nil {
					return errors.Wrap(err, "deleting team label")
				}
			}
		}
		return nil
	})
	if err != nil {
		return params.Teams{}, err
	}
	return t.Get(ctx, team.Name)
}

func (t *teamManager) Get(ctx context.Context, name string) (params.Teams, error) {
	team, err := t.getTeam(ctx, name)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}
	joinRows, addedBy, err := t.fetchMembershipData(team.ID)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching membership data")
	}

	out := t.sqlToCommonTeams(team, user.ID, joinRows, addedBy, "", false)

	active, pending := 0, 0
	for _, row := range joinRows {
		if row.Status == models.TeamMembershipPending {
			pending++
		} else {
			active++
		}
	}
	stats := &params.TeamStats{Members: active + 1, Pending: pending}
	var pasteCnt, contribCnt int64
	if err := t.conn.Model(&models.Paste{}).Where("team_id = ?", team.ID).Count(&pasteCnt).Error; err != nil {
		return params.Teams{}, errors.Wrap(err, "counting team pastes")
	}
	if err := t.conn.Model(&models.Paste{}).
		Where("team_id = ?", team.ID).
		Distinct("owner_id").
		Count(&contribCnt).Error; err != nil {
		return params.Teams{}, errors.Wrap(err, "counting team contributors")
	}
	stats.Pastes = int(pasteCnt)
	stats.Contributors = int(contribCnt)
	out.Stats = stats

	var labels []models.Label
	if err := t.conn.Where("team_id = ?", team.ID).Order("name ASC").Find(&labels).Error; err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team labels")
	}
	for _, l := range labels {
		out.Labels = append(out.Labels, l.Name)
		var usage int64
		if err := t.conn.Table("paste_labels").Where("label_id = ?", l.ID).Count(&usage).Error; err != nil {
			return params.Teams{}, errors.Wrap(err, "counting team label usage")
		}
		out.LabelDetails = append(out.LabelDetails, params.LabelInfo{ID: l.ID, Name: l.Name, Color: l.Color, Usage: usage})
	}

	return out, nil
}

func (t *teamManager) List(ctx context.Context, page int64, results int64) (teams params.TeamListResult, err error) {
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.TeamListResult{}, errors.Wrap(err, "fetching user from DB")
	}
	if page == 0 {
		page = 1
	}
	if results == 0 {
		results = 1
	}
	var teamsResults []models.Teams
	var cnt int64
	startFrom := (page - 1) * results

	// Teams the user owns, plus teams the user is a member of, plus teams
	// the user has been invited to (pending).
	q := t.conn.Preload("Owner").Where(
		"owner_id = ? OR id IN (SELECT teams_id FROM team_users WHERE team_users.users_id = ?)",
		user.ID, user.ID).Order("id desc")

	cntQ := q.Model(&models.Teams{}).Count(&cnt)
	if cntQ.Error != nil {
		return params.TeamListResult{}, errors.Wrap(cntQ.Error, "counting results")
	}

	resQ := q.Offset(int(startFrom)).Limit(int(results)).Find(&teamsResults)
	if resQ.Error != nil {
		if errors.Is(resQ.Error, gorm.ErrRecordNotFound) {
			return params.TeamListResult{}, gErrors.ErrNotFound
		}
		return params.TeamListResult{}, errors.Wrap(resQ.Error, "fetching teams from database")
	}

	// Load the viewer's membership status for the listed teams in one query.
	teamIDs := make([]uint, len(teamsResults))
	for idx, val := range teamsResults {
		teamIDs[idx] = val.ID
	}
	joinRows := map[uint]models.TeamUser{}
	if len(teamIDs) > 0 {
		var rows []models.TeamUser
		if err := t.conn.Where("teams_id IN ? AND users_id = ?", teamIDs, user.ID).Find(&rows).Error; err != nil {
			return params.TeamListResult{}, errors.Wrap(err, "fetching team memberships")
		}
		for _, row := range rows {
			joinRows[row.TeamID] = row
		}
	}

	asParams := make([]params.Teams, len(teamsResults))
	for idx, val := range teamsResults {
		myStatus := ""
		if row, ok := joinRows[val.ID]; ok {
			myStatus = row.Status
		}
		asParams[idx] = t.sqlToCommonTeams(val, user.ID, nil, nil, myStatus, true)
	}

	totalPages := int64(math.Ceil(float64(cnt) / float64(results)))
	if totalPages == 0 {
		totalPages = 1
	}

	if totalPages < page {
		page = totalPages
	}
	return params.TeamListResult{
		Teams:      asParams,
		TotalPages: totalPages,
		Page:       page,
	}, nil
}

// AddMember invites a user to the team. The invitation stays pending until
// the invitee accepts it; pending invitees have no access to team resources.
func (t *teamManager) AddMember(ctx context.Context, teamName string, userID string, role string) (params.TeamMember, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching team")
	}

	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching user from context")
	}

	if !t.canManageMembers(team, user.ID) {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrUnauthorized, "inviting members to team")
	}
	if role == "" {
		role = models.RoleMember
	}
	if role != models.RoleAdmin && role != models.RoleMember && role != models.RoleViewer {
		return params.TeamMember{}, gErrors.NewBadRequestError("invalid role")
	}
	if role == models.RoleAdmin && team.OwnerID != user.ID {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrUnauthorized, "only the team owner can invite admins")
	}

	memberUser, err := t.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching member")
	}

	if memberUser.ID == team.OwnerID {
		return params.TeamMember{}, gErrors.NewBadRequestError("the team owner is already part of the team")
	}

	existing, err := t.membership(team.ID, memberUser.ID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "checking membership")
	}
	if existing != nil {
		if existing.Status == models.TeamMembershipPending {
			return params.TeamMember{}, gErrors.NewBadRequestError("this user has already been invited and has not answered yet")
		}
		return params.TeamMember{}, gErrors.NewBadRequestError("user is already a member of this team")
	}

	row := models.TeamUser{
		TeamID:    team.ID,
		UserID:    memberUser.ID,
		Status:    models.TeamMembershipPending,
		Role:      role,
		AddedByID: &user.ID,
	}
	if err := t.conn.Create(&row).Error; err != nil {
		return params.TeamMember{}, errors.Wrapf(err, "inviting %s to team %s", memberUser.Email, team.Name)
	}

	member := sqlUserToTeamMember(memberUser)
	member.Status = models.TeamMembershipPending
	member.Role = effectiveRole(role)
	member.AddedByID = user.ID
	member.AddedBy = userDisplay(user)
	return member, nil
}

// SetMemberRole assigns a role to an invited or accepted member. Only the
// team owner may change roles.
func (t *teamManager) SetMemberRole(ctx context.Context, teamName, member string, role string) (params.TeamMember, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching user from context")
	}
	if team.OwnerID != user.ID {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrUnauthorized, "only the team owner can change roles")
	}
	if role != models.RoleAdmin && role != models.RoleMember && role != models.RoleViewer {
		return params.TeamMember{}, gErrors.NewBadRequestError("invalid role")
	}

	memberUser, err := t.getUserByUsernameOrEmail(member)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching member")
	}
	if memberUser.ID == team.OwnerID {
		return params.TeamMember{}, gErrors.NewBadRequestError("the team owner's role cannot be changed")
	}
	row, err := t.membership(team.ID, memberUser.ID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "checking membership")
	}
	if row == nil {
		return params.TeamMember{}, gErrors.NewBadRequestError("user is not a member of this team")
	}
	if err := t.conn.Model(row).Update("role", role).Error; err != nil {
		return params.TeamMember{}, errors.Wrap(err, "updating member role")
	}
	out := sqlUserToTeamMember(memberUser)
	out.Status = row.Status
	out.Role = effectiveRole(role)
	return out, nil
}

// AcceptInvite confirms a pending invitation for the calling user.
func (t *teamManager) AcceptInvite(ctx context.Context, teamName string) (params.Teams, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}

	row, err := t.membership(team.ID, user.ID)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "checking membership")
	}
	if row == nil || row.Status != models.TeamMembershipPending {
		return params.Teams{}, gErrors.NewBadRequestError("there is no pending invitation for this team")
	}

	if err := t.conn.Model(row).Update("status", models.TeamMembershipActive).Error; err != nil {
		return params.Teams{}, errors.Wrap(err, "accepting invitation")
	}
	return t.Get(ctx, teamName)
}

// DeclineInvite rejects an invitation: the invitee removes their own pending
// membership.
func (t *teamManager) DeclineInvite(ctx context.Context, teamName string) error {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user from context")
	}

	row, err := t.membership(team.ID, user.ID)
	if err != nil {
		return errors.Wrap(err, "checking membership")
	}
	if row == nil || row.Status != models.TeamMembershipPending {
		return gErrors.NewBadRequestError("there is no pending invitation for this team")
	}
	if err := t.conn.Delete(row).Error; err != nil {
		return errors.Wrap(err, "declining invitation")
	}
	return nil
}

// LeaveTeam removes the calling user from a team they have joined. Team
// owners must delete the team instead.
func (t *teamManager) LeaveTeam(ctx context.Context, teamName string) error {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user from context")
	}
	if team.OwnerID == user.ID {
		return gErrors.NewBadRequestError("the team owner cannot leave the team; delete it instead")
	}

	row, err := t.membership(team.ID, user.ID)
	if err != nil {
		return errors.Wrap(err, "checking membership")
	}
	if row == nil {
		return gErrors.NewBadRequestError("you are not a member of this team")
	}
	if row.Status != models.TeamMembershipActive {
		return gErrors.NewBadRequestError("you have not joined this team yet; decline the invitation instead")
	}
	if err := t.conn.Delete(row).Error; err != nil {
		return errors.Wrap(err, "leaving team")
	}
	return nil
}

func (t *teamManager) RemoveMember(ctx context.Context, teamName, member string) error {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return errors.Wrap(err, "fetching team")
	}

	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user from context")
	}

	actorRole := t.teamRole(team, user.ID)
	if actorRole != models.RoleOwner && actorRole != models.RoleAdmin {
		return errors.Wrap(gErrors.ErrUnauthorized, "removing team members")
	}

	memberUser, err := t.getUserByUsernameOrEmail(member)
	if err != nil {
		return errors.Wrap(err, "fetching member")
	}

	if memberUser.ID == team.OwnerID {
		return gErrors.NewBadRequestError("the team owner cannot be removed from the team")
	}

	targetRow, err := t.membership(team.ID, memberUser.ID)
	if err != nil {
		return errors.Wrap(err, "checking membership")
	}
	if targetRow == nil {
		return gErrors.NewBadRequestError("user is not a member of this team")
	}
	// Members allowed to remove others (admins) may only be removed by the
	// team owner.
	if actorRole == models.RoleAdmin && effectiveRole(targetRow.Role) == models.RoleAdmin {
		return errors.Wrap(gErrors.ErrUnauthorized, "only the team owner can remove admins")
	}

	if err := t.conn.Where("teams_id = ? AND users_id = ?", team.ID, memberUser.ID).Delete(&models.TeamUser{}).Error; err != nil {
		return errors.Wrap(err, "removing member")
	}
	return nil
}

func (t *teamManager) ListMembers(ctx context.Context, teamName string) ([]params.TeamMember, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return []params.TeamMember{}, errors.Wrap(err, "fetching team")
	}
	joinRows, addedBy, err := t.fetchMembershipData(team.ID)
	if err != nil {
		return []params.TeamMember{}, errors.Wrap(err, "fetching membership data")
	}

	ret := make([]params.TeamMember, 0, len(team.Members)+1)
	owner := sqlUserToTeamMember(team.Owner)
	owner.Status = models.TeamMembershipActive
	ret = append(ret, owner)

	for _, val := range team.Members {
		member := sqlUserToTeamMember(*val)
		member.Status = models.TeamMembershipActive
		member.Role = models.RoleMember
		if row, ok := joinRows[val.ID]; ok {
			member.Status = row.Status
			member.Role = effectiveRole(row.Role)
			if row.AddedByID != nil {
				member.AddedByID = *row.AddedByID
				if adder, ok := addedBy[*row.AddedByID]; ok {
					member.AddedBy = userDisplay(adder)
				}
			}
		}
		ret = append(ret, member)
	}

	return ret, nil
}

// RequestTransfer asks an accepted member to take over team ownership. The
// transfer completes only when the target accepts, mirroring the invitation
// flow. Only the team owner may request it and only one transfer can be
// pending at a time.
func (t *teamManager) RequestTransfer(ctx context.Context, teamName string, userID string) (params.Teams, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}
	if team.OwnerID != user.ID {
		return params.Teams{}, errors.Wrap(gErrors.ErrUnauthorized, "only the team owner can transfer the team")
	}
	if team.TransferToUserID != nil {
		return params.Teams{}, gErrors.NewBadRequestError("a transfer is already pending")
	}

	memberUser, err := t.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching member")
	}
	if memberUser.ID == team.OwnerID {
		return params.Teams{}, gErrors.NewBadRequestError("the team already belongs to you")
	}
	if !t.isActiveMember(team.ID, memberUser.ID) {
		return params.Teams{}, gErrors.NewBadRequestError("only accepted team members can be offered ownership")
	}

	if err := t.conn.Model(&models.Teams{}).Where("id = ?", team.ID).
		Update("transfer_to_user_id", memberUser.ID).Error; err != nil {
		return params.Teams{}, errors.Wrap(err, "requesting team transfer")
	}
	return t.Get(ctx, teamName)
}

// CancelTransfer withdraws a pending ownership transfer (owner only).
func (t *teamManager) CancelTransfer(ctx context.Context, teamName string) error {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user from context")
	}
	if team.OwnerID != user.ID {
		return errors.Wrap(gErrors.ErrUnauthorized, "only the team owner can cancel the transfer")
	}
	if team.TransferToUserID == nil {
		return gErrors.NewBadRequestError("there is no pending transfer")
	}
	if err := t.conn.Model(&models.Teams{}).Where("id = ?", team.ID).
		Update("transfer_to_user_id", nil).Error; err != nil {
		return errors.Wrap(err, "cancelling team transfer")
	}
	return nil
}

// AcceptTransfer completes a pending transfer offered to the calling user:
// the target becomes the owner, the previous owner stays as an admin.
func (t *teamManager) AcceptTransfer(ctx context.Context, teamName string) (params.Teams, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching user from context")
	}
	if team.TransferToUserID == nil || *team.TransferToUserID != user.ID {
		return params.Teams{}, gErrors.NewBadRequestError("there is no pending transfer for you on this team")
	}

	previousOwnerID := team.OwnerID
	err = t.conn.Transaction(func(tx *gorm.DB) error {
		team.OwnerID = user.ID
		team.TransferToUserID = nil
		if err := tx.Model(&models.Teams{}).Where("id = ?", team.ID).
			Updates(map[string]interface{}{"owner_id": user.ID, "transfer_to_user_id": nil}).Error; err != nil {
			return errors.Wrap(err, "updating team owner")
		}
		// The new owner holds no join row (ownership is not membership).
		if err := tx.Where("teams_id = ? AND users_id = ?", team.ID, user.ID).
			Delete(&models.TeamUser{}).Error; err != nil {
			return errors.Wrap(err, "clearing new owner membership")
		}
		// The previous owner remains on the team as admin.
		row, err := t.membership(team.ID, previousOwnerID)
		if err != nil {
			return errors.Wrap(err, "checking previous owner membership")
		}
		if row == nil {
			if err := tx.Create(&models.TeamUser{
				TeamID:    team.ID,
				UserID:    previousOwnerID,
				Status:    models.TeamMembershipActive,
				Role:      models.RoleAdmin,
				AddedByID: &user.ID,
			}).Error; err != nil {
				return errors.Wrap(err, "keeping previous owner as admin")
			}
		} else if err := tx.Model(row).Updates(map[string]interface{}{
			"status": models.TeamMembershipActive,
			"role":   models.RoleAdmin,
		}).Error; err != nil {
			return errors.Wrap(err, "demoting previous owner to admin")
		}
		return nil
	})
	if err != nil {
		return params.Teams{}, err
	}
	return t.Get(ctx, teamName)
}

// DeclineTransfer rejects an ownership transfer offered to the calling user.
func (t *teamManager) DeclineTransfer(ctx context.Context, teamName string) error {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return errors.Wrap(err, "fetching team")
	}
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user from context")
	}
	if team.TransferToUserID == nil || *team.TransferToUserID != user.ID {
		return gErrors.NewBadRequestError("there is no pending transfer for you on this team")
	}
	if err := t.conn.Model(&models.Teams{}).Where("id = ?", team.ID).
		Update("transfer_to_user_id", nil).Error; err != nil {
		return errors.Wrap(err, "declining team transfer")
	}
	return nil
}

// ListPendingTransfers returns the teams whose ownership transfer is waiting
// on the calling user, for the site-wide notice.
func (t *teamManager) ListPendingTransfers(ctx context.Context) ([]params.TeamTransferInfo, error) {
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching user")
	}
	var teams []models.Teams
	if err := t.conn.Preload("Owner").Where("transfer_to_user_id = ?", user.ID).Find(&teams).Error; err != nil {
		return nil, errors.Wrap(err, "listing pending transfers")
	}
	out := []params.TeamTransferInfo{}
	for _, team := range teams {
		out = append(out, params.TeamTransferInfo{
			TeamID:   team.ID,
			TeamName: team.Name,
			FromUser: userDisplay(team.Owner),
		})
	}
	return out, nil
}

// ListPendingInvites returns the teams the calling user was invited to but
// has not accepted yet, for the site-wide invitation notice.
func (t *teamManager) ListPendingInvites(ctx context.Context) ([]params.TeamInviteInfo, error) {
	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching user")
	}
	var rows []models.TeamUser
	if err := t.conn.Where("users_id = ? AND status = ?", user.ID, models.TeamMembershipPending).Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "listing pending invitations")
	}
	out := []params.TeamInviteInfo{}
	if len(rows) == 0 {
		return out, nil
	}
	teamIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		teamIDs = append(teamIDs, r.TeamID)
	}
	var teams []models.Teams
	if err := t.conn.Select("id", "name").Where("id IN ?", teamIDs).Find(&teams).Error; err != nil {
		return nil, errors.Wrap(err, "fetching invited teams")
	}
	names := map[uint]string{}
	for _, tm := range teams {
		names[tm.ID] = tm.Name
	}
	var adderIDs []uint
	for _, r := range rows {
		if r.AddedByID != nil {
			adderIDs = append(adderIDs, *r.AddedByID)
		}
	}
	adders := map[uint]models.Users{}
	if len(adderIDs) > 0 {
		var users []models.Users
		if err := t.conn.Select("id", "username", "full_name").Where("id IN ?", adderIDs).Find(&users).Error; err != nil {
			return nil, errors.Wrap(err, "fetching inviters")
		}
		for _, u := range users {
			adders[u.ID] = u
		}
	}
	for _, r := range rows {
		info := params.TeamInviteInfo{TeamID: r.TeamID, TeamName: names[r.TeamID]}
		if r.AddedByID != nil {
			if a, ok := adders[*r.AddedByID]; ok {
				info.InvitedBy = userDisplay(a)
			}
		}
		out = append(out, info)
	}
	return out, nil
}
