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

// Package testdb hands out throwaway databases to tests.
package testdb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopherbin/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// MySQLEnv names the environment variable that switches tests to MySQL. Its
// value is "user:password@host:port" of a server where the user may create
// and drop databases, e.g. "root:secret@127.0.0.1:3306".
const MySQLEnv = "GOPHERBIN_TEST_MYSQL"

// Config returns the configuration of a fresh, empty database that lives as
// long as the test: a SQLite file by default, or a new MySQL database when
// MySQLEnv is set.
func Config(t testing.TB) config.Database {
	t.Helper()
	spec := os.Getenv(MySQLEnv)
	if spec == "" {
		return config.Database{
			DbBackend: config.SQLiteBackend,
			SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "test.db")},
		}
	}

	creds, host, ok := strings.Cut(spec, "@")
	user, password, _ := strings.Cut(creds, ":")
	if !ok || user == "" || host == "" {
		t.Fatalf("%s must look like user:password@host:port, got %q", MySQLEnv, spec)
	}
	server, err := gorm.Open(mysql.Open(fmt.Sprintf("%s:%s@tcp(%s)/", user, password, host)), &gorm.Config{})
	if err != nil {
		t.Fatalf("connecting to MySQL: %v", err)
	}
	name := fmt.Sprintf("gopherbin_test_%d", time.Now().UnixNano())
	if err := server.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4").Error; err != nil {
		t.Fatalf("creating test database: %v", err)
	}
	t.Cleanup(func() {
		server.Exec("DROP DATABASE " + name)
		if sqlDB, err := server.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return config.Database{
		DbBackend: config.MySQLBackend,
		MySQL: config.MySQL{
			Username:     user,
			Password:     password,
			Hostname:     host,
			DatabaseName: name,
		},
	}
}

// IsMySQL reports whether tests run against MySQL.
func IsMySQL() bool {
	return os.Getenv(MySQLEnv) != ""
}
