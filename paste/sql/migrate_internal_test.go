package sql

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"gopherbin/config"
)

func openEnforcedSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	sqliteCfg := config.SQLite{DBFile: filepath.Join(t.TempDir(), "fk.db")}
	dsn, err := sqliteCfg.ConnectionString()
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	conn, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := conn.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return conn
}

func foreignKeysEnabled(t *testing.T, conn *gorm.DB) bool {
	t.Helper()
	var on int
	if err := conn.Raw("PRAGMA foreign_keys").Scan(&on).Error; err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	return on != 0
}

func TestSuspendSQLiteForeignKeys(t *testing.T) {
	conn := openEnforcedSQLite(t)
	for _, stmt := range []string{
		`CREATE TABLE parents (id integer PRIMARY KEY)`,
		`CREATE TABLE children (id integer PRIMARY KEY, parent_id integer REFERENCES parents(id) ON DELETE CASCADE)`,
	} {
		if err := conn.Exec(stmt).Error; err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	if !foreignKeysEnabled(t, conn) {
		t.Fatal("test requires foreign key enforcement enabled by the DSN")
	}
	if err := conn.Exec(`INSERT INTO children (id, parent_id) VALUES (1, 99)`).Error; err == nil {
		t.Fatal("expected foreign key violation with enforcement on")
	}

	restore, err := suspendSQLiteForeignKeys(conn)
	if err != nil {
		t.Fatalf("suspendSQLiteForeignKeys: %v", err)
	}

	if foreignKeysEnabled(t, conn) {
		t.Error("foreign keys still enabled while suspended")
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("raw DB handle: %v", err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections while suspended = %d, want 1", got)
	}
	if err := conn.Exec(`INSERT INTO children (id, parent_id) VALUES (1, 99)`).Error; err != nil {
		t.Errorf("orphan insert rejected while suspended: %v", err)
	}

	restore()

	if !foreignKeysEnabled(t, conn) {
		t.Error("foreign keys not re-enabled after restore")
	}
	if err := conn.Exec(`INSERT INTO children (id, parent_id) VALUES (2, 99)`).Error; err == nil {
		t.Error("expected foreign key violation after restore")
	}
	// The default pool limit is unlimited (0); restore must put it back.
	if got := sqlDB.Stats().MaxOpenConnections; got != 0 {
		t.Errorf("MaxOpenConnections after restore = %d, want 0", got)
	}
}

// A failing migration must not leave foreign keys suspended.
func TestSuspendSQLiteForeignKeysRestoredOnMigrateError(t *testing.T) {
	conn := openEnforcedSQLite(t)
	if err := conn.Exec(`CREATE TABLE teams (id integer PRIMARY KEY, name varchar(32))`).Error; err != nil {
		t.Fatalf("create teams: %v", err)
	}

	restore, err := suspendSQLiteForeignKeys(conn)
	if err != nil {
		t.Fatalf("suspendSQLiteForeignKeys: %v", err)
	}

	// Simulate AutoMigrate failing half-way through; migrateDB defers the
	// restore regardless of the migration outcome.
	if err := conn.Exec(`INSERT INTO does_not_exist (id) VALUES (1)`).Error; err == nil {
		t.Fatal("expected error from bogus statement")
	}
	restore()

	if !foreignKeysEnabled(t, conn) {
		t.Error("foreign keys left suspended after migration failure")
	}
}
