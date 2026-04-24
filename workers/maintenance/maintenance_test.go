package maintenance_test

import (
	"path/filepath"
	"testing"
	"time"

	"gopherbin/config"
	pasteSQL "gopherbin/paste/sql"
	"gopherbin/workers/maintenance"
)

func testDBConfig(t *testing.T) config.Database {
	t.Helper()
	return config.Database{
		DbBackend: config.SQLiteBackend,
		SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "test.db")},
	}
}

func TestNewMaintenanceWorker(t *testing.T) {
	dbCfg := testDBConfig(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	w, err := maintenance.NewMaintenanceWorker(dbCfg)
	if err != nil {
		t.Fatalf("NewMaintenanceWorker: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
}

func TestMaintenanceWorker_StartStop(t *testing.T) {
	dbCfg := testDBConfig(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	w, err := maintenance.NewMaintenanceWorker(dbCfg)
	if err != nil {
		t.Fatalf("NewMaintenanceWorker: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- w.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2 seconds")
	}
}
