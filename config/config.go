package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

const (
	// MySQLBackend represents the MySQL DB backend
	MySQLBackend = "mysql"
	// SQLiteBackend represents the Sqlite3 DB backend
	SQLiteBackend = "sqlite3"
)

// NewConfig returns a new Config
func NewConfig(cfgFile string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(cfgFile, &config); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
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
	RegistrationOpen bool
	AllowAnonymous   bool
}

// Validate validates the default section of the config
func (d *Default) Validate() error {
	return nil
}

// Database is the database config entry
type Database struct {
	SQLBackend string `toml:"backend"`
	SQLite     SQLite `toml:"sqlite"`
	MySQL      MySQL  `toml:"mysql"`
}

// GormParams returns the database type and connection URI
func (d *Database) GormParams() (dbType string, uri string, err error) {
	if err := d.Validate(); err != nil {
		return "", "", errors.Wrap(err, "validating database config")
	}
	dbType = d.SQLBackend
	switch dbType {
	case MySQLBackend:
		uri, err = d.MySQL.ConnectionString()
		if err != nil {
			return "", "", errors.Wrap(err, "validating mysql config")
		}
	case SQLiteBackend:
		uri, err = d.SQLite.ConnectionString()
		if err != nil {
			return "", "", errors.Wrap(err, "validating sqlite3 config")
		}
	}
	return
}

// Validate validates the database config entry
func (d *Database) Validate() error {
	if d.SQLBackend == "" {
		return fmt.Errorf("Invalid databse configuration: backend is required")
	}
	switch d.SQLBackend {
	case MySQLBackend:
		if err := d.MySQL.Validate(); err != nil {
			return errors.Wrap(err, "validating mysql config")
		}
	case SQLiteBackend:
		if err := d.SQLite.Validate(); err != nil {
			return errors.Wrap(err, "validating sqlite3 config")
		}
	default:
		return fmt.Errorf("Invalid database backend: %s", d.SQLBackend)
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

// SQLite is the SQLite3 backend
type SQLite struct {
	DBFile string `toml:"db_file"`
}

// ConnectionString returns a gorm compatible connection string
func (s *SQLite) ConnectionString() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}

	return s.DBFile, nil
}

// Validate validates the SQLite config section
func (s *SQLite) Validate() error {
	absPath, err := filepath.Abs(s.DBFile)
	if err != nil {
		return errors.Wrap(err, "getting dirname")
	}
	parent := filepath.Dir(absPath)
	if _, err := os.Stat(parent); err != nil {
		return errors.Wrap(err, "fetching info about dirname")
	}
	return nil
}

// TLSConfig is the API server TLS config
type TLSConfig struct {
	CRT    string
	Key    string
	CACert string
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
		ClientAuth:   tls.RequireAndVerifyClientCert,
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

// APIServer holds configuration for the API server
// worker
type APIServer struct {
	Bind      string
	Port      int
	UseTLS    bool
	TLSConfig TLSConfig `toml:"tls"`
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
	ip := net.ParseIP(a.Bind)
	if ip == nil {
		// No need for deeper validation here, as any invalid
		// IP address specified in this setting will raise an error
		// when we try to bind to it.
		return fmt.Errorf("invalid IP address")
	}
	return nil
}
