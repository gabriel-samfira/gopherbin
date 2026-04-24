package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopherbin/config"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.toml")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	f.Close()
	return f.Name()
}

func validTOML(dbFile string) string {
	return fmt.Sprintf(`
[apiserver]
bind = "0.0.0.0"
port = 9997

  [apiserver.jwt_auth]
  secret = "super-secret-key-for-testing"
  time_to_live = "1h"

[database]
backend = "sqlite3"

  [database.sqlite3]
  db_file = %q
`, dbFile)
}

// ── NewConfig ─────────────────────────────────────────────────────────────────

func TestNewConfig_ValidFile(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")
	path := writeTOML(t, validTOML(dbFile))
	cfg, err := config.NewConfig(path)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	if cfg.APIServer.Port != 9997 {
		t.Errorf("want port 9997, got %d", cfg.APIServer.Port)
	}
	if cfg.Database.DbBackend != config.SQLiteBackend {
		t.Errorf("want sqlite3 backend, got %q", cfg.Database.DbBackend)
	}
}

func TestNewConfig_MissingFile(t *testing.T) {
	_, err := config.NewConfig("/no/such/file.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewConfig_InvalidTOML(t *testing.T) {
	path := writeTOML(t, "not valid toml ][")
	_, err := config.NewConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

// ── SQLite.Validate ───────────────────────────────────────────────────────────

func TestSQLite_Validate_EmptyPath(t *testing.T) {
	s := config.SQLite{}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for empty db_file")
	}
}

func TestSQLite_Validate_RelativePath(t *testing.T) {
	s := config.SQLite{DBFile: "relative/path.db"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for relative path")
	}
}

func TestSQLite_Validate_NonExistentParent(t *testing.T) {
	s := config.SQLite{DBFile: "/no/such/dir/test.db"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error when parent dir does not exist")
	}
}

func TestSQLite_Validate_Valid(t *testing.T) {
	s := config.SQLite{DBFile: filepath.Join(t.TempDir(), "test.db")}
	if err := s.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLite_ConnectionString(t *testing.T) {
	s := config.SQLite{DBFile: "/tmp/test.db"}
	cs, err := s.ConnectionString()
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	if cs == "" {
		t.Error("expected non-empty connection string")
	}
	// WAL mode and foreign keys must be enabled
	for _, param := range []string{"_journal_mode=WAL", "_foreign_keys=ON"} {
		if !contains(cs, param) {
			t.Errorf("connection string missing %q", param)
		}
	}
}

// ── MySQL.Validate ────────────────────────────────────────────────────────────

func TestMySQL_Validate_MissingFields(t *testing.T) {
	m := config.MySQL{}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty MySQL config")
	}
}

func TestMySQL_Validate_Valid(t *testing.T) {
	m := config.MySQL{
		Username: "user", Password: "pass",
		Hostname: "localhost", DatabaseName: "db",
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMySQL_ConnectionString(t *testing.T) {
	m := config.MySQL{
		Username: "user", Password: "pass",
		Hostname: "localhost", DatabaseName: "db",
	}
	cs, err := m.ConnectionString()
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	for _, part := range []string{"user", "pass", "localhost", "db"} {
		if !contains(cs, part) {
			t.Errorf("connection string missing %q", part)
		}
	}
}

// ── JWTAuth.Validate ──────────────────────────────────────────────────────────

func TestJWTAuth_Validate_EmptySecret(t *testing.T) {
	j := config.JWTAuth{}
	if err := j.Validate(); err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestJWTAuth_Validate_SetsDefaultTTL(t *testing.T) {
	j := config.JWTAuth{Secret: "s3cr3t"}
	if err := j.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if j.TimeToLive.Duration() < config.DefaultJWTTTL {
		t.Errorf("want TTL >= %v, got %v", config.DefaultJWTTTL, j.TimeToLive.Duration())
	}
}

// ── Database.Validate ─────────────────────────────────────────────────────────

func TestDatabase_Validate_EmptyBackend(t *testing.T) {
	d := config.Database{}
	if err := d.Validate(); err == nil {
		t.Fatal("expected error for empty backend")
	}
}

func TestDatabase_Validate_InvalidBackend(t *testing.T) {
	d := config.Database{DbBackend: "postgres"}
	if err := d.Validate(); err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}

func TestDatabase_Validate_SQLite(t *testing.T) {
	d := config.Database{
		DbBackend: config.SQLiteBackend,
		SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "test.db")},
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatabase_Validate_MySQL(t *testing.T) {
	d := config.Database{
		DbBackend: config.MySQLBackend,
		MySQL: config.MySQL{
			Username: "u", Password: "p", Hostname: "h", DatabaseName: "db",
		},
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Database.GormParams ───────────────────────────────────────────────────────

func TestDatabase_GormParams_SQLite(t *testing.T) {
	d := config.Database{
		DbBackend: config.SQLiteBackend,
		SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "test.db")},
	}
	dbType, uri, err := d.GormParams()
	if err != nil {
		t.Fatalf("GormParams: %v", err)
	}
	if dbType != config.SQLiteBackend {
		t.Errorf("want sqlite3, got %q", dbType)
	}
	if uri == "" {
		t.Error("expected non-empty URI")
	}
}

func TestDatabase_GormParams_MySQL(t *testing.T) {
	d := config.Database{
		DbBackend: config.MySQLBackend,
		MySQL: config.MySQL{
			Username: "u", Password: "p", Hostname: "h", DatabaseName: "db",
		},
	}
	dbType, uri, err := d.GormParams()
	if err != nil {
		t.Fatalf("GormParams: %v", err)
	}
	if dbType != config.MySQLBackend {
		t.Errorf("want mysql, got %q", dbType)
	}
	if uri == "" {
		t.Error("expected non-empty URI")
	}
}

// ── APIServer.Validate ────────────────────────────────────────────────────────

func TestAPIServer_Validate_InvalidPort(t *testing.T) {
	for _, port := range []int{0, 99999} {
		a := config.APIServer{
			Port:    port,
			Bind:    "0.0.0.0",
			JWTAuth: config.JWTAuth{Secret: "s"},
		}
		if err := a.Validate(); err == nil {
			t.Errorf("expected error for port %d", port)
		}
	}
}

func TestAPIServer_Validate_InvalidBind(t *testing.T) {
	a := config.APIServer{
		Port:    9997,
		Bind:    "not-an-ip",
		JWTAuth: config.JWTAuth{Secret: "s"},
	}
	if err := a.Validate(); err == nil {
		t.Fatal("expected error for invalid bind IP")
	}
}

func TestAPIServer_Validate_MissingJWTSecret(t *testing.T) {
	a := config.APIServer{Port: 9997, Bind: "0.0.0.0"}
	if err := a.Validate(); err == nil {
		t.Fatal("expected error for missing JWT secret")
	}
}

func TestAPIServer_Validate_Valid(t *testing.T) {
	a := config.APIServer{
		Port:    9997,
		Bind:    "0.0.0.0",
		JWTAuth: config.JWTAuth{Secret: "super-secret"},
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── utility ───────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
