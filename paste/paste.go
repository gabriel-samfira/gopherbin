package paste

import (
	"fmt"

	"gopherbin/config"
	"gopherbin/paste/common"
	"gopherbin/paste/mysql"
)

// NewPaster returns a new paste implementation based on the configured
// database backend
func NewPaster(dbCfg config.Database, cfg config.Default) (common.Paster, error) {
	dbBackend := dbCfg.DbBackend
	switch dbBackend {
	case config.MySQLBackend:
		return mysql.NewPaster(dbCfg, cfg)
	default:
		return nil, fmt.Errorf("no user manager available for db backend %s", dbBackend)
	}
}
