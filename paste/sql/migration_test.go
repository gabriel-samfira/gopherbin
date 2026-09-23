package sql_test

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"gopherbin/config"
	pasteSQL "gopherbin/paste/sql"
)

// DDL exactly as emitted by the e1f26e3 release, including the ON DELETE
// CASCADE constraints on team_users and pastes that reference teams.
const (
	oldUsersDDL     = "CREATE TABLE `users` (`id` integer PRIMARY KEY AUTOINCREMENT,`created_at` datetime,`updated_at` datetime,`username` varchar(64),`full_name` varchar(254),`email` varchar(254),`password` varchar(60),`is_admin` numeric,`is_super_user` numeric,`enabled` numeric,CONSTRAINT `uni_users_email` UNIQUE (`email`))"
	oldPastesDDL    = "CREATE TABLE `pastes` (`id` integer PRIMARY KEY AUTOINCREMENT,`paste_id` varchar(32),`data` longblob,`language` varchar(64),`name` text,`description` text,`metadata` JSON,`owner_id` integer,`created_at` datetime,`expires` datetime,`public` numeric,`max_accesses` integer DEFAULT NULL,`access_count` integer DEFAULT 0,`team_id` integer,CONSTRAINT `fk_pastes_owner` FOREIGN KEY (`owner_id`) REFERENCES `users`(`id`) ON DELETE CASCADE ON UPDATE CASCADE,CONSTRAINT `fk_pastes_team` FOREIGN KEY (`team_id`) REFERENCES `teams`(`id`) ON DELETE CASCADE ON UPDATE CASCADE)"
	oldTeamsDDL     = "CREATE TABLE `teams` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` varchar(32),`owner_id` integer,CONSTRAINT `fk_teams_owner` FOREIGN KEY (`owner_id`) REFERENCES `users`(`id`))"
	oldTeamUsersDDL = "CREATE TABLE `team_users` (`teams_id` integer,`users_id` integer,PRIMARY KEY (`teams_id`,`users_id`),CONSTRAINT `fk_team_users_teams` FOREIGN KEY (`teams_id`) REFERENCES `teams`(`id`) ON DELETE CASCADE,CONSTRAINT `fk_team_users_users` FOREIGN KEY (`users_id`) REFERENCES `users`(`id`) ON DELETE CASCADE)"
)

// newLegacyDB creates a database with the pre-upgrade schema and seeds it with
// rows that are exposed to the teams-table rebuild cascade: two memberships
// and two team-owned pastes, plus a personal paste as a control. The returned
// config points at the closed, ready-to-migrate file.
func newLegacyDB(t *testing.T) config.Database {
	t.Helper()
	dbCfg := config.Database{
		DbBackend: config.SQLiteBackend,
		SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "legacy.db")},
	}
	dsn, err := dbCfg.SQLite.ConnectionString()
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	conn, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy DB: %v", err)
	}
	for _, ddl := range []string{oldUsersDDL, oldTeamsDDL, oldPastesDDL, oldTeamUsersDDL} {
		if err := conn.Exec(ddl).Error; err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	seed := []string{
		`INSERT INTO users (id, username, email, password, is_admin, is_super_user, enabled) VALUES (1, 'owner', 'owner@example.com', 'x', 1, 1, 1), (2, 'member', 'member@example.com', 'x', 0, 0, 1), (3, 'other', 'other@example.com', 'x', 0, 0, 1)`,
		`INSERT INTO teams (id, name, owner_id) VALUES (1, 'legacy', 1)`,
		`INSERT INTO team_users (teams_id, users_id) VALUES (1, 2), (1, 3)`,
		`INSERT INTO pastes (id, paste_id, data, name, owner_id, public, team_id) VALUES (1, 'team1', 'hello', 'team-paste-1', 1, 0, 1), (2, 'team2', 'hello', 'team-paste-2', 2, 0, 1), (3, 'mine', 'hello', 'own-paste', 2, 0, NULL)`,
	}
	for _, s := range seed {
		if err := conn.Exec(s).Error; err != nil {
			t.Fatalf("seed legacy data: %v", err)
		}
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("raw DB handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close legacy DB: %v", err)
	}
	return dbCfg
}

func openDB(t *testing.T, dbCfg config.Database) *gorm.DB {
	t.Helper()
	dsn, err := dbCfg.SQLite.ConnectionString()
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	conn, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	return conn
}

func countRows(t *testing.T, conn *gorm.DB, query string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	if err := conn.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// The migration adds a foreign key to teams (transfer_to_user_id), which makes
// the SQLite driver rebuild the table with DROP + recreate. With enforcement
// on (as per the production DSN), the DROP used to cascade into team_users and
// pastes and destroy team membership and team-owned pastes.
func TestMigratePreservesTeamRows(t *testing.T) {
	dbCfg := newLegacyDB(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("NewPaster (migration): %v", err)
	}

	migrated := openDB(t, dbCfg)
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM team_users WHERE teams_id = 1"); got != 2 {
		t.Errorf("team memberships lost during migration: team_users has %d rows, want 2", got)
	}
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM pastes WHERE team_id = 1"); got != 2 {
		t.Errorf("team pastes lost during migration: %d rows with team_id, want 2", got)
	}
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM pastes WHERE paste_id = 'mine' AND team_id IS NULL"); got != 1 {
		t.Errorf("personal paste lost during migration: %d, want 1", got)
	}

	// Columns added by the migration must carry their constant defaults on
	// pre-existing rows.
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM team_users WHERE status = 'active' AND role = 'member'"); got != 2 {
		t.Errorf("migrated memberships without active/member defaults: %d, want 2", got)
	}
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM pragma_table_info('team_users') WHERE name = 'role'"); got != 1 {
		t.Errorf("role column missing after migration")
	}
}

// Running the migration over an already migrated database must be a no-op for
// existing data (covers the upgrade-then-restart path).
func TestMigrateIsIdempotent(t *testing.T) {
	dbCfg := newLegacyDB(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("NewPaster (first migration): %v", err)
	}
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("NewPaster (second migration): %v", err)
	}

	migrated := openDB(t, dbCfg)
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM team_users"); got != 2 {
		t.Errorf("memberships changed on second migration: %d, want 2", got)
	}
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM pastes"); got != 3 {
		t.Errorf("pastes changed on second migration: %d, want 3", got)
	}
	if got := countRows(t, migrated, "SELECT COUNT(*) FROM teams"); got != 1 {
		t.Errorf("teams changed on second migration: %d, want 1", got)
	}
	var violations int64
	if err := migrated.Raw("SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&violations).Error; err != nil {
		t.Fatalf("foreign key check: %v", err)
	}
	if violations != 0 {
		t.Errorf("foreign key violations after two migrations: %d", violations)
	}
}

// The whole point of rebuilding the teams table is to attach the
// transfer_to_user_id foreign key; make sure it survived the rebuild and is
// enforced like the other constraints.
func TestMigrateAddsTransferConstraint(t *testing.T) {
	dbCfg := newLegacyDB(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("NewPaster (migration): %v", err)
	}

	migrated := openDB(t, dbCfg)
	if err := migrated.Exec(`INSERT INTO teams (name, owner_id, transfer_to_user_id) VALUES ('ghost', 1, 999)`).Error; err == nil {
		t.Error("transfer_to_user_id pointing at a missing user was accepted; constraint missing after table rebuild")
	}
	if err := migrated.Exec(`INSERT INTO teams (name, owner_id, transfer_to_user_id) VALUES ('ok', 1, 2)`).Error; err != nil {
		t.Errorf("valid transfer_to_user_id rejected: %v", err)
	}
}
