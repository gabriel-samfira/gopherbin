package config_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	// validTOML does not set max_body_size; the default must be applied.
	if cfg.APIServer.MaxBodySize != config.DefaultMaxBodySize {
		t.Errorf("want default max body size %d, got %d",
			config.DefaultMaxBodySize, cfg.APIServer.MaxBodySize)
	}
	// A configured TTL below the 24h default must be honored, not
	// silently bumped up to it.
	if ttl := cfg.APIServer.JWTAuth.TimeToLive.Duration(); ttl != time.Hour {
		t.Errorf("want configured TTL 1h, got %v", ttl)
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

func TestNewConfig_MaxBodySizeFromTOML(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")
	path := writeTOML(t, fmt.Sprintf(`
[apiserver]
bind = "0.0.0.0"
port = 9997
max_body_size = 12345

  [apiserver.jwt_auth]
  secret = "super-secret-key-for-testing"

[database]
backend = "sqlite3"

  [database.sqlite3]
  db_file = %q
`, dbFile))
	cfg, err := config.NewConfig(path)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}
	if cfg.APIServer.MaxBodySize != 12345 {
		t.Errorf("want max_body_size 12345, got %d", cfg.APIServer.MaxBodySize)
	}
	// time_to_live unset in this file → default 24h.
	if ttl := cfg.APIServer.JWTAuth.TimeToLive.Duration(); ttl != config.DefaultJWTTTL {
		t.Errorf("want default TTL %v, got %v", config.DefaultJWTTTL, ttl)
	}
}

func TestNewConfig_TTLBelowMinimumRejected(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")
	path := writeTOML(t, fmt.Sprintf(`
[apiserver]
bind = "0.0.0.0"
port = 9997

  [apiserver.jwt_auth]
  secret = "super-secret-key-for-testing"
  time_to_live = "30s"

[database]
backend = "sqlite3"

  [database.sqlite3]
  db_file = %q
`, dbFile))
	if _, err := config.NewConfig(path); err == nil || !strings.Contains(err.Error(), "time_to_live") {
		t.Fatalf("time_to_live below the minimum: want a time_to_live error, got %v", err)
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
	if j.TimeToLive.Duration() != config.DefaultJWTTTL {
		t.Errorf("want TTL %v, got %v", config.DefaultJWTTTL, j.TimeToLive.Duration())
	}
}

func TestJWTAuth_Validate_HonorsTTLAboveMinimum(t *testing.T) {
	// A configured TTL >= MinJWTTTL must be kept as-is, NOT silently
	// bumped up to DefaultJWTTTL.
	j := config.JWTAuth{Secret: "s3cr3t", TimeToLive: "6h"}
	if err := j.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := j.TimeToLive.Duration(); got != 6*time.Hour {
		t.Errorf("want TTL 6h, got %v", got)
	}
}

func TestJWTAuth_Validate_AcceptsMinimumTTL(t *testing.T) {
	j := config.JWTAuth{Secret: "s3cr3t", TimeToLive: "10m"}
	if err := j.Validate(); err != nil {
		t.Fatalf("unexpected error for TTL == MinJWTTTL: %v", err)
	}
	if got := j.TimeToLive.Duration(); got != config.MinJWTTTL {
		t.Errorf("want TTL %v, got %v", config.MinJWTTTL, got)
	}
}

func TestJWTAuth_Validate_RejectsTTLBelowMinimum(t *testing.T) {
	j := config.JWTAuth{Secret: "s3cr3t", TimeToLive: "9m59s"}
	if err := j.Validate(); err == nil {
		t.Fatal("expected error for TTL below MinJWTTTL")
	}
	// The rejected value must not have been rewritten to the default.
	if got := j.TimeToLive.Duration(); got != 9*time.Minute+59*time.Second {
		t.Errorf("rejected TTL should not be rewritten, got %v", got)
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

// ── APIServer.Validate: MaxBodySize ───────────────────────────────────────────

func TestAPIServer_Validate_MaxBodySizeDefaultsWhenUnset(t *testing.T) {
	a := config.APIServer{
		Port:    9997,
		Bind:    "0.0.0.0",
		JWTAuth: config.JWTAuth{Secret: "super-secret"},
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.MaxBodySize != config.DefaultMaxBodySize {
		t.Errorf("want default %d, got %d", config.DefaultMaxBodySize, a.MaxBodySize)
	}
}

func TestAPIServer_Validate_DefaultMaxBodySizeIs8MiB(t *testing.T) {
	if config.DefaultMaxBodySize != 8388608 {
		t.Errorf("want 8388608, got %d", config.DefaultMaxBodySize)
	}
}

func TestAPIServer_Validate_MaxBodySizeHonored(t *testing.T) {
	a := config.APIServer{
		Port:        9997,
		Bind:        "0.0.0.0",
		JWTAuth:     config.JWTAuth{Secret: "super-secret"},
		MaxBodySize: 42 * 1024 * 1024,
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.MaxBodySize != 42*1024*1024 {
		t.Errorf("configured value must be kept, got %d", a.MaxBodySize)
	}
}

func TestAPIServer_Validate_NegativeMaxBodySizeDefaults(t *testing.T) {
	a := config.APIServer{
		Port:        9997,
		Bind:        "0.0.0.0",
		JWTAuth:     config.JWTAuth{Secret: "super-secret"},
		MaxBodySize: -1,
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.MaxBodySize != config.DefaultMaxBodySize {
		t.Errorf("want default %d for negative value, got %d",
			config.DefaultMaxBodySize, a.MaxBodySize)
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

// ── APIServer.TrustedProxies ─────────────────────────────────────────────────

func TestAPIServer_TrustedProxyNets(t *testing.T) {
	a := config.APIServer{TrustedProxies: []string{"10.0.0.0/8", " 192.0.2.1 ", "2001:db8::1"}}
	nets, err := a.TrustedProxyNets()
	if err != nil {
		t.Fatalf("TrustedProxyNets: %v", err)
	}
	if len(nets) != 3 {
		t.Fatalf("want 3 networks, got %d", len(nets))
	}
	for _, tc := range []struct {
		net  int
		ip   string
		want bool
	}{
		{0, "10.200.1.1", true}, {0, "11.0.0.1", false},
		{1, "192.0.2.1", true}, {1, "192.0.2.2", false},
		{2, "2001:db8::1", true}, {2, "2001:db8::2", false},
	} {
		if got := nets[tc.net].Contains(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("%s in %s: got %v, want %v", tc.ip, nets[tc.net], got, tc.want)
		}
	}
	for _, bad := range []string{"proxy.example.com", "10.0.0.0/33", ""} {
		a := config.APIServer{TrustedProxies: []string{bad}}
		if _, err := a.TrustedProxyNets(); err == nil {
			t.Errorf("TrustedProxyNets(%q): want an error", bad)
		}
	}
}
