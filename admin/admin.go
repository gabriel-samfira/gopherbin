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

package admin

import (
	"fmt"

	"gopherbin/admin/common"
	"gopherbin/admin/sql"
	"gopherbin/config"
)

// GetUserManager returns a common.UserManager based on the selected database type
func GetUserManager(dbCfg config.Database, defCfg config.Default) (common.UserManager, error) {
	dbBackend := dbCfg.DbBackend
	switch dbBackend {
	case config.MySQLBackend, config.SQLiteBackend:
		return sql.NewUserManager(dbCfg, defCfg)
	default:
		return nil, fmt.Errorf("no user manager available for db backend %s", dbBackend)
	}
}
