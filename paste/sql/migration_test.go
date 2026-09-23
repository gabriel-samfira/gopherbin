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

// The migration adds a foreign key to teams (transfer_to_user_id), which makes
// the SQLite driver rebuild the table with DROP + recreate. With enforcement
// on (as per the production DSN), the DROP used to cascade into team_users and
// pastes and destroy team membership and team-owned pastes.
func TestMigratePreservesTeamRows(t *testing.T) {
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
		`INSERT INTO users (id, username, email, password, is_admin, is_super_user, enabled) VALUES (1, 'owner', 'owner@example.com', 'x', 1, 1, 1), (2, 'member', 'member@example.com', 'x', 0, 0, 1)`,
		`INSERT INTO teams (id, name, owner_id) VALUES (1, 'legacy', 1)`,
		`INSERT INTO team_users (teams_id, users_id) VALUES (1, 2)`,
		`INSERT INTO pastes (id, paste_id, data, name, owner_id, public, team_id) VALUES (1, 'abc123', 'hello', 'team-paste', 1, 0, 1)`,
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

	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("NewPaster (migration): %v", err)
	}

	migrated, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open migrated DB: %v", err)
	}
	var members int64
	if err := migrated.Raw("SELECT COUNT(*) FROM team_users WHERE teams_id = 1 AND users_id = 2").Scan(&members).Error; err != nil {
		t.Fatalf("count team_users: %v", err)
	}
	if members != 1 {
		t.Errorf("team membership lost during migration: team_users has %d rows, want 1", members)
	}
	var pasteCount int64
	if err := migrated.Raw("SELECT COUNT(*) FROM pastes WHERE paste_id = 'abc123' AND team_id = 1").Scan(&pasteCount).Error; err != nil {
		t.Fatalf("count pastes: %v", err)
	}
	if pasteCount != 1 {
		t.Errorf("team paste lost during migration: pastes has %d rows with team_id, want 1", pasteCount)
	}
	var roleCol int64
	if err := migrated.Raw("SELECT COUNT(*) FROM pragma_table_info('team_users') WHERE name = 'role'").Scan(&roleCol).Error; err != nil {
		t.Fatalf("check role column: %v", err)
	}
	if roleCol != 1 {
		t.Errorf("role column missing after migration")
	}
}
