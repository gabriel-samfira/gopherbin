package paste

import (
	"gopherbin/config"
	"gopherbin/paste/common"
	"gopherbin/util"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/pkg/errors"
)

type paste struct {
	conn *gorm.DB
}

func (m *paste) Create(ownerID string, data []byte, expires time.Time, isPublic bool) (paste common.Paste, err error) {
	return common.Paste{}, nil
}

func (m *paste) Get(ownerID string, pasteID string) (paste common.Paste, err error) {
	return common.Paste{}, nil
}

func (m *paste) Delete(id string) error {
	return nil
}

func (m *paste) ShareWithUser(id string, user string) error {
	return nil
}

func (m *paste) ShareWithTeam(id string, team string) error {
	return nil
}

func (m *paste) SetPrivacy(id string, private bool) error {
	return nil
}

// NewPaster returns a SQL backed paste implementation
func NewPaster(dbCfg config.Database) (common.Paster, error) {
	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}
	return &paste{
		conn: db,
	}, nil
}
