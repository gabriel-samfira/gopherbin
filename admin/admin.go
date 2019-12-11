package admin

import (
	"fmt"

	"gopherbin/admin/common"
	"gopherbin/admin/mysql"
	"gopherbin/config"
)

// GetUserManager returns a common.UserManager based on the selected database type
func GetUserManager(dbCfg config.Database, defCfg config.Default) (common.UserManager, error) {
	dbBackend := dbCfg.DbBackend
	switch dbBackend {
	case config.MySQLBackend:
		return mysql.NewUserManager(dbCfg, defCfg)
	default:
		return nil, fmt.Errorf("no user manager available for db backend %s", dbBackend)
	}
}
