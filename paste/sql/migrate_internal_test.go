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

// migrateDB must hand the pool back as it found it, whether the migration
// succeeds or fails: foreign keys enforced and the pool no longer pinned to
// a single connection (a pinned pool would serialize, or deadlock, every
// later request).
func TestMigrateDBRestoresConnectionState(t *testing.T) {
	check := func(t *testing.T, conn *gorm.DB) {
		t.Helper()
		if !foreignKeysEnabled(t, conn) {
			t.Error("foreign keys left suspended after migration")
		}
		sqlDB, err := conn.DB()
		if err != nil {
			t.Fatalf("raw DB handle: %v", err)
		}
		if got := sqlDB.Stats().MaxOpenConnections; got != 0 {
			t.Errorf("MaxOpenConnections after migration = %d, want 0 (unlimited)", got)
		}
	}

	t.Run("success", func(t *testing.T) {
		conn := openEnforcedSQLite(t)
		p := &paste{conn: conn, dbBackend: config.SQLiteBackend, teamMgr: &teamManager{conn: conn}}
		if err := p.migrateDB(); err != nil {
			t.Fatalf("migrateDB: %v", err)
		}
		check(t, conn)
	})

	t.Run("failure", func(t *testing.T) {
		conn := openEnforcedSQLite(t)
		// A view occupying a model table's name makes AutoMigrate fail
		// half-way through.
		if err := conn.Exec(`CREATE VIEW users AS SELECT 1 AS id`).Error; err != nil {
			t.Fatalf("create blocking view: %v", err)
		}
		p := &paste{conn: conn, dbBackend: config.SQLiteBackend, teamMgr: &teamManager{conn: conn}}
		if err := p.migrateDB(); err == nil {
			t.Fatal("expected the migration to fail")
		}
		check(t, conn)
	})
}
