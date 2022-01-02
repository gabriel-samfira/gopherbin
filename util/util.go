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

package util

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"unicode"

	"gopherbin/config"

	"github.com/cespare/xxhash"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const alphanumeric = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// From: https://www.alexedwards.net/blog/validation-snippets-for-go#email-validation
var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// IsValidEmail returs a bool indicating if an email is valid
func IsValidEmail(email string) bool {
	if len(email) > 254 || !rxEmail.MatchString(email) {
		return false
	}
	return true
}

func IsAlphanumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

// NewDBConn returns a new gorm db connection, given the config
func NewDBConn(dbCfg config.Database) (conn *gorm.DB, err error) {
	dbType, connURI, err := dbCfg.GormParams()
	if err != nil {
		return nil, errors.Wrap(err, "getting DB URI string")
	}
	switch dbType {
	case config.MySQLBackend:
		conn, err = gorm.Open(mysql.Open(connURI), &gorm.Config{})
	case config.SQLiteBackend:
		conn, err = gorm.Open(sqlite.Open(connURI), &gorm.Config{})
	}
	if err != nil {
		return nil, errors.Wrap(err, "connecting to database")
	}

	if dbCfg.Debug {
		conn = conn.Debug()
	}
	return conn, nil
}

// PaswsordToBcrypt returns a bcrypt hash of the specified password using the default cost
func PaswsordToBcrypt(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// TODO: make this a fatal error, that should return a 500 error to user
		return "", fmt.Errorf("failed to hash password")
	}
	return string(hashedPassword), nil
}

// HashString generates an xxhash from the supplied string
func HashString(input string) (uint64, error) {
	h := xxhash.New()
	if added, err := h.Write([]byte(input)); err != nil {
		return 0, err
	} else if added != len(input) {
		return 0, fmt.Errorf("wrote %d, expected %d", added, len(input))
	}
	return h.Sum64(), nil
}

// GetRandomString returns a secure random string
func GetRandomString(n int) (string, error) {
	data := make([]byte, n)
	_, err := rand.Read(data)
	if err != nil {
		return "", errors.Wrap(err, "getting random data")
	}
	for i, b := range data {
		data[i] = alphanumeric[b%byte(len(alphanumeric))]
	}

	return string(data), nil
}
