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

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type DBBackendType string

const (
	// MySQLBackend represents the MySQL DB backend
	MySQLBackend DBBackendType = "mysql"
	// SQLiteBackend represents the SQLite3 DB backend
	SQLiteBackend DBBackendType = "sqlite3"
	// DefaultJWTTTL is the default duration in seconds a JWT token
	// will be valid.
	DefaultJWTTTL time.Duration = 24 * time.Hour
)

// NewConfig returns a new Config
func NewConfig(cfgFile string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(cfgFile, &config); err != nil {
		return nil, errors.Wrap(err, "decoding toml")
	}
	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "validating config")
	}
	return &config, nil
}

// Config represents the configuration of gopherbin
type Config struct {
	APIServer APIServer `toml:"apiserver" json:"apiserver"`
	Database  Database  `toml:"database" json:"database"`
}

// Validate validates the config
func (c *Config) Validate() error {
	if err := c.APIServer.Validate(); err != nil {
		return errors.Wrap(err, "validating APIServer config")
	}
	if err := c.Database.Validate(); err != nil {
		return errors.Wrap(err, "validating database config")
	}

	return nil
}

// Database is the database config entry
type Database struct {
	Debug     bool          `toml:"debug" json:"debug"`
	DbBackend DBBackendType `toml:"backend" json:"backend"`
	MySQL     MySQL         `toml:"mysql" json:"mysql"`
	SQLite    SQLite        `toml:"sqlite3" json:"sqlite3"`
}

// GormParams returns the database type and connection URI
func (d *Database) GormParams() (dbType DBBackendType, uri string, err error) {
	if err := d.Validate(); err != nil {
		return "", "", errors.Wrap(err, "validating database config")
	}
	dbType = d.DbBackend
	switch dbType {
	case MySQLBackend:
		uri, err = d.MySQL.ConnectionString()
		if err != nil {
			return "", "", errors.Wrap(err, "validating mysql config")
		}
	case SQLiteBackend:
		uri, err = d.SQLite.ConnectionString()
		if err != nil {
			return "", "", errors.Wrap(err, "validating mysql config")
		}
	default:
		return "", "", fmt.Errorf("invalid database backend: %s", dbType)
	}
	return
}

// Validate validates the database config entry
func (d *Database) Validate() error {
	if d.DbBackend == "" {
		return fmt.Errorf("invalid databse configuration: backend is required")
	}
	switch d.DbBackend {
	case MySQLBackend:
		if err := d.MySQL.Validate(); err != nil {
			return errors.Wrap(err, "validating mysql config")
		}
	case SQLiteBackend:
		if err := d.SQLite.Validate(); err != nil {
			return errors.Wrap(err, "validating sqlite3 config")
		}
	default:
		return fmt.Errorf("invalid database backend: %s", d.DbBackend)
	}
	return nil
}

// SQLite is the config entry for the sqlite3 section
type SQLite struct {
	DBFile string `toml:"db_file" json:"db-file"`
}

func (s *SQLite) Validate() error {
	if s.DBFile == "" {
		return fmt.Errorf("no valid db_file was specified")
	}

	if !filepath.IsAbs(s.DBFile) {
		return fmt.Errorf("please specify an absolute path for db_file")
	}

	parent := filepath.Dir(s.DBFile)
	if _, err := os.Stat(parent); err != nil {
		return errors.Wrapf(err, "accessing db_file parent dir: %s", parent)
	}
	return nil
}

func (s *SQLite) ConnectionString() (string, error) {
	return s.DBFile, nil
}

// MySQL is the config entry for the mysql section
type MySQL struct {
	Username     string `toml:"username" json:"username"`
	Password     string `toml:"password" json:"password"`
	Hostname     string `toml:"hostname" json:"hostname"`
	DatabaseName string `toml:"database" json:"database"`
}

// Validate validates a Database config entry
func (m *MySQL) Validate() error {
	if m.Username == "" || m.Password == "" || m.Hostname == "" || m.DatabaseName == "" {
		return fmt.Errorf(
			"database, username, password, hostname are mandatory parameters for the database section")
	}
	return nil
}

// ConnectionString returns a gorm compatible connection string
func (m *MySQL) ConnectionString() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}

	connString := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?charset=utf8&parseTime=True&loc=Local&timeout=5s",
		m.Username, m.Password,
		m.Hostname, m.DatabaseName,
	)
	return connString, nil
}

// TLSConfig is the API server TLS config
type TLSConfig struct {
	CRT    string `toml:"certificate" json:"certificate"`
	Key    string `toml:"key" json:"key"`
	CACert string `toml:"ca_certificate" json:"ca-certificate"`
}

// TLSConfig returns a new TLSConfig suitable for use in the
// API server
func (t *TLSConfig) TLSConfig() (*tls.Config, error) {
	// TLS config not present.
	if t.CRT == "" && t.Key == "" {
		return nil, fmt.Errorf("missing crt or key")
	}

	var roots *x509.CertPool
	if t.CACert != "" {
		caCertPEM, err := ioutil.ReadFile(t.CACert)
		if err != nil {
			return nil, err
		}
		roots = x509.NewCertPool()
		ok := roots.AppendCertsFromPEM(caCertPEM)
		if !ok {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
	}

	cert, err := tls.LoadX509KeyPair(t.CRT, t.Key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    roots,
	}, nil
}

// Validate validates the TLS config
func (t *TLSConfig) Validate() error {
	if _, err := t.TLSConfig(); err != nil {
		return err
	}
	return nil
}

type timeToLive string

func (d *timeToLive) Duration() time.Duration {
	duration, err := time.ParseDuration(string(*d))
	if err != nil {
		return DefaultJWTTTL
	}
	return duration
}

func (d *timeToLive) UnmarshalText(text []byte) error {
	_, err := time.ParseDuration(string(text))
	if err != nil {
		return errors.Wrap(err, "parsing time_to_live")
	}

	*d = timeToLive(text)
	return nil
}

// JWTAuth holds settings used to generate JWT tokens
type JWTAuth struct {
	Secret     string     `toml:"secret" json:"secret"`
	TimeToLive timeToLive `toml:"time_to_live" json:"time-to-live"`
}

// Validate validates the JWTAuth config
func (j *JWTAuth) Validate() error {
	// TODO: Set defaults somewhere else.
	if j.TimeToLive.Duration() < DefaultJWTTTL {
		j.TimeToLive = timeToLive(DefaultJWTTTL.String())
	}
	if j.Secret == "" {
		return fmt.Errorf("invalid JWT secret")
	}
	return nil
}

// APIServer holds configuration for the API server
// worker
type APIServer struct {
	Bind        string    `toml:"bind" json:"bind"`
	Port        int       `toml:"port" json:"port"`
	UseTLS      bool      `toml:"use_tls" json:"use_tls"`
	JWTAuth     JWTAuth   `toml:"jwt_auth" json:"jwt-auth"`
	TLSConfig   TLSConfig `toml:"tls" json:"tls"`
	CORSOrigins []string  `toml:"cors_origins" json:"cors-origins"`
}

// Validate validates the API server config
func (a *APIServer) Validate() error {
	if a.UseTLS {
		if err := a.TLSConfig.Validate(); err != nil {
			return errors.Wrap(err, "TLS validation failed")
		}
	}
	if a.Port > 65535 || a.Port < 1 {
		return fmt.Errorf("invalid port nr %q", a.Port)
	}

	if err := a.JWTAuth.Validate(); err != nil {
		return errors.Wrap(err, "validating jwt config")
	}
	ip := net.ParseIP(a.Bind)
	if ip == nil {
		// No need for deeper validation here, as any invalid
		// IP address specified in this setting will raise an error
		// when we try to bind to it.
		return fmt.Errorf("invalid IP address")
	}
	return nil
}
