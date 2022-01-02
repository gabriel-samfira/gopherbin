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

	q := t.conn.Preload("Teams").Where(queryString, userID).First(&tmpUser)
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
	q := t.conn.Preload("Teams").Where("id = ?", userID).First(&tmpUser)
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

func (t *teamManager) get(name string) (models.Teams, error) {
	var teamModel models.Teams
	q := t.conn.Preload("Members").Preload("Owner").Where("name = ?", name).First(&teamModel)
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
		Username: user.Username,
		Email:    user.Email,
		FullName: user.FullName,
	}
}

func (t *teamManager) sqlToCommonTeams(team models.Teams, preview bool) params.Teams {
	var members []params.TeamMember = []params.TeamMember{}
	if !preview {
		members = make([]params.TeamMember, len(team.Members))
		for idx, val := range team.Members {
			members[idx] = sqlUserToTeamMember(*val)
		}
	}
	return params.Teams{
		ID:      team.ID,
		Name:    team.Name,
		Owner:   sqlUserToTeamMember(team.Owner),
		Members: members,
	}
}

func (t *teamManager) Create(ctx context.Context, name string) (params.Teams, error) {
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
		OwnerID: user.ID,
		Owner:   user,
		Name:    name,
	}

	q := t.conn.Create(&team)
	if q.Error != nil {
		return params.Teams{}, errors.Wrap(q.Error, "creating team")
	}

	return t.sqlToCommonTeams(team, true), nil
}

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

	q := t.conn.Delete(&team)
	if q.Error != nil && !errors.Is(q.Error, gorm.ErrRecordNotFound) {
		return errors.Wrap(q.Error, "deleting paste")
	}
	return nil
}

func (t *teamManager) Get(ctx context.Context, name string) (params.Teams, error) {
	team, err := t.getTeam(ctx, name)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "fetching team")
	}

	return t.sqlToCommonTeams(team, false), nil
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

	q := t.conn.Preload("Owner").Select("id, name, owner_id").Where("owner = ?", user.ID).Order("id desc")

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

	asParams := make([]params.Teams, len(teamsResults))
	for idx, val := range teamsResults {
		asParams[idx] = t.sqlToCommonTeams(val, true)
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

func (t *teamManager) AddMember(ctx context.Context, teamName string, userID string) (params.TeamMember, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching team")
	}

	user, err := t.getUserFromContext(ctx)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching user from context")
	}

	if team.OwnerID != user.ID {
		return params.TeamMember{}, errors.Wrap(gErrors.ErrUnauthorized, "accesing team")
	}

	memberUser, err := t.getUserByUsernameOrEmail(userID)
	if err != nil {
		return params.TeamMember{}, errors.Wrap(err, "fetching member")
	}

	if err := t.conn.Model(&team).Association("Members").Append(&memberUser); err != nil {
		return params.TeamMember{}, errors.Wrapf(err, "adding member %s to team %s", memberUser.Email, team.Name)
	}
	return sqlUserToTeamMember(memberUser), nil
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

	if team.OwnerID != user.ID {
		return errors.Wrap(gErrors.ErrUnauthorized, "accesing team")
	}

	memberUser, err := t.getUserByUsernameOrEmail(member)
	if err != nil {
		return errors.Wrap(err, "fetching member")
	}

	if err := t.conn.Model(&team).Association("Members").Delete(memberUser); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return errors.Wrap(err, "removing member")
	}
	return nil
}

func (t *teamManager) ListMembers(ctx context.Context, teamName string) ([]params.TeamMember, error) {
	team, err := t.getTeam(ctx, teamName)
	if err != nil {
		return []params.TeamMember{}, errors.Wrap(err, "fetching team")
	}
	ret := make([]params.TeamMember, len(team.Members)+1)
	ret[0] = sqlUserToTeamMember(team.Owner)

	for idx, val := range team.Members {
		ret[idx+1] = sqlUserToTeamMember(*val)
	}

	return ret, nil
}
