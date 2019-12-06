package paste

import (
	"gopherbin/config"
	"gopherbin/models"
	"gopherbin/paste/common"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/pkg/errors"
)

type paster struct {
	conn *gorm.DB
}

func (m *paster) Create(owner models.Users, data []byte, expires time.Time, isPublic bool) (paste models.Paste, err error) {
	return models.Paste{}, nil
}

func (m *paster) Delete(id string) error {
	return nil
}

func (m *paster) ShareWithUser(id string, user string) error {
	return nil
}

func (m *paster) ShareWithTeam(id string, team string) error {
	return nil
}

func (m *paster) SetPrivacy(id string, private bool) error {
	return nil
}

// NewPaster returns a SQL backed paste implementation
func NewPaster(dbCfg config.Database) (common.Paster, error) {
	dbType, connURI, err := dbCfg.GormParams()
	if err != nil {
		return nil, errors.Wrap(err, "getting DB URI string")
	}
	db, err := gorm.Open(dbType, connURI)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}
	return &paster{
		conn: db,
	}, nil
}
