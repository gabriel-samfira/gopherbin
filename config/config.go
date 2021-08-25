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
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

const (
	// MySQLBackend represents the MySQL DB backend
	MySQLBackend = "mysql"
	// DefaultJWTTTL is the default duration in seconds a JWT token
	// will be valid.
	DefaultJWTTTL time.Duration = 60 * time.Second
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
	APIServer APIServer
	Database  Database
	Default   Default
}

// Validate validates the config
func (c *Config) Validate() error {
	if err := c.APIServer.Validate(); err != nil {
		return errors.Wrap(err, "validating APIServer config")
	}
	if err := c.Database.Validate(); err != nil {
		return errors.Wrap(err, "validating database config")
	}

	if err := c.Default.Validate(); err != nil {
		return errors.Wrap(err, "validating the default section")
	}
	return nil
}

// Default defines settings
type Default struct {
	RegistrationOpen bool `toml:"registration_open"`
	AllowAnonymous   bool `toml:"allow_anonymous"`
}

// Validate validates the default section of the config
func (d *Default) Validate() error {
	return nil
}

// Database is the database config entry
type Database struct {
	DbBackend string `toml:"backend"`
	MySQL     MySQL  `toml:"mysql"`
}

// GormParams returns the database type and connection URI
func (d *Database) GormParams() (dbType string, uri string, err error) {
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
	default:
		return "", "", fmt.Errorf("invalid database backend: %s", dbType)
	}
	return
}

// Validate validates the database config entry
func (d *Database) Validate() error {
	if d.DbBackend == "" {
		return fmt.Errorf("Invalid databse configuration: backend is required")
	}
	switch d.DbBackend {
	case MySQLBackend:
		if err := d.MySQL.Validate(); err != nil {
			return errors.Wrap(err, "validating mysql config")
		}
	default:
		return fmt.Errorf("Invalid database backend: %s", d.DbBackend)
	}
	return nil
}

// MySQL is the config entry for the mysql section
type MySQL struct {
	Username     string
	Password     string
	Hostname     string
	DatabaseName string `toml:"database"`
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
	CRT    string `toml:"certificate"`
	Key    string `toml:"key"`
	CACert string `toml:"ca_certificate"`
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

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	if err != nil {
		return errors.Wrap(err, "parsing time_to_live")
	}
	return nil
}

// JWTAuth holds settings used to generate JWT tokens
type JWTAuth struct {
	Secret     string   `toml:"secret"`
	TimeToLive duration `toml:"time_to_live"`
}

// Validate validates the JWTAuth config
func (j *JWTAuth) Validate() error {
	// TODO: Set defaults somewhere else.
	if j.TimeToLive.Duration < DefaultJWTTTL {
		j.TimeToLive.Duration = DefaultJWTTTL
	}
	if j.Secret == "" {
		return fmt.Errorf("invalid JWT secret")
	}
	return nil
}

// APIServer holds configuration for the API server
// worker
type APIServer struct {
	Bind        string
	Port        int
	UseTLS      bool
	JWTAuth     JWTAuth   `toml:"jwt_auth"`
	TLSConfig   TLSConfig `toml:"tls"`
	CORSOrigins []string  `toml:"cors_origins"`
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
