package sql

import (
	"context"
	"fmt"

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

func (p *teamManager) getUser(userID int64) (models.Users, error) {
	// TODO: abstract this into a common interface
	var tmpUser models.Users
	q := p.conn.Preload("Teams").Where("id = ?", userID).First(&tmpUser)
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
	if team.Owner == user.ID {
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
	q := t.conn.Preload("Members").Where("name = ?", name).First(&teamModel)
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

func (t *teamManager) sqlToCommonTeams(team models.Teams, owner models.Users) params.Teams {
	teamOwner := params.TeamMember{
		Username: owner.Username,
		Email:    owner.Email,
		FullName: owner.FullName,
	}

	members := make([]params.TeamMember, len(team.Members))
	for idx, val := range team.Members {
		members[idx] = params.TeamMember{
			Username: val.Username,
			Email:    val.Email,
			FullName: val.FullName,
		}
	}
	return params.Teams{
		ID:      team.ID,
		Name:    team.Name,
		Owner:   teamOwner,
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
		Owner: user.ID,
		Name:  name,
	}

	err = t.conn.Model(&user).Association("Teams").Append(&team)
	if err != nil {
		return params.Teams{}, errors.Wrap(err, "creating team")
	}

	if team.Owner != user.ID {
		return params.Teams{}, fmt.Errorf("failed to create team")
	}
	return t.sqlToCommonTeams(team, user), nil
}

func (t *teamManager) Delete(ctx context.Context, name string) error {
	return nil
}

func (t *teamManager) Get(ctx context.Context, name string) (params.Teams, error) {
	return params.Teams{}, nil
}

func (t *teamManager) List(ctx context.Context, page int64, results int64) (teams []params.Teams, err error) {
	return []params.Teams{}, nil
}

func (t *teamManager) AddMember(ctx context.Context, team string, member params.AddTeamMemberRequest) (params.TeamMember, error) {
	return params.TeamMember{}, nil
}

func (t *teamManager) ListMembers(ctx context.Context, team string, page int64, results int64) ([]params.TeamMember, error) {
	return []params.TeamMember{}, nil
}

func (t *teamManager) RemoveMember(ctx context.Context, team, member string) error {
	return nil
}
