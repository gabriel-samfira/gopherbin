package sql

import (
	"context"
	"sync"
	"testing"

	"gopherbin/auth"
	"gopherbin/config"
	"gopherbin/models"
	"gopherbin/params"

	"gorm.io/gorm"
)

// insertRecorder records the tables GORM inserts into through a connection.
type insertRecorder struct {
	mu     sync.Mutex
	tables []string
}

func (r *insertRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tables = nil
}

func (r *insertRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.tables...)
}

func recordInserts(t *testing.T, conn *gorm.DB) *insertRecorder {
	t.Helper()
	rec := &insertRecorder{}
	err := conn.Callback().Create().Before("gorm:create").Register("test:record_inserts", func(db *gorm.DB) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.tables = append(rec.tables, db.Statement.Table)
	})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
	return rec
}

func newInternalFixture(t *testing.T) (*paste, *insertRecorder) {
	t.Helper()
	conn := openEnforcedSQLite(t)
	p := &paste{conn: conn, dbBackend: config.SQLiteBackend, teamMgr: &teamManager{conn: conn}}
	if err := p.migrateDB(); err != nil {
		t.Fatalf("migrateDB: %v", err)
	}
	return p, recordInserts(t, conn)
}

func mustUser(t *testing.T, conn *gorm.DB, name string) (models.Users, context.Context) {
	t.Helper()
	user := models.Users{Username: name, Email: name + "@example.com", Enabled: true, Discoverable: true}
	if err := conn.Create(&user).Error; err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	ctx := auth.PopulateContext(context.Background(), params.Users{ID: user.ID, Enabled: true})
	return user, ctx
}

// Mutations load rows together with their associations. Writing such a row
// back with Save (or creating a row that embeds a loaded association) makes
// GORM re-insert the association's join rows, which resurrects memberships,
// shares and labels deleted concurrently; a resurrected team_users row even
// comes back as an *active* member through the column default. These
// mutations must only write the rows they mean to.
func TestMutationsDoNotRewriteAssociations(t *testing.T) {
	p, rec := newInternalFixture(t)
	owner, ownerCtx := mustUser(t, p.conn, "owner")
	_, memberCtx := mustUser(t, p.conn, "member")
	mustUser(t, p.conn, "other")
	tm := p.teamMgr

	if _, err := tm.Create(ownerCtx, "team", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	if _, err := tm.AddMember(ownerCtx, "team", "member", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if _, err := tm.AcceptInvite(memberCtx, "team"); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	personal, err := p.Create(ownerCtx, []byte("data"), "personal", "text", "", nil, false, "", nil, nil, []string{"mine"})
	if err != nil {
		t.Fatalf("Create personal paste: %v", err)
	}
	if _, err := p.ShareWithUser(ownerCtx, personal.PasteID, "member"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}

	for _, tc := range []struct {
		name    string
		allowed map[string]bool
		run     func() error
	}{
		{"create team paste", map[string]bool{"pastes": true, "labels": true, "paste_labels": true}, func() error {
			_, err := p.Create(memberCtx, []byte("data"), "team paste", "text", "", nil, false, "team", nil, nil, []string{"shared"})
			return err
		}},
		{"rename team", nil, func() error {
			name := "renamed"
			_, err := tm.Update(ownerCtx, "team", params.UpdateTeamParams{Name: &name})
			return err
		}},
		{"set privacy", nil, func() error {
			_, err := p.SetPrivacy(ownerCtx, personal.PasteID, true)
			return err
		}},
		{"share", map[string]bool{"paste_users": true}, func() error {
			_, err := p.ShareWithUser(ownerCtx, personal.PasteID, "other")
			return err
		}},
		{"transfer", nil, func() error {
			_, err := p.TransferOwnership(ownerCtx, personal.PasteID, "member")
			return err
		}},
	} {
		rec.reset()
		if err := tc.run(); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		for _, table := range rec.seen() {
			if !tc.allowed[table] {
				t.Errorf("%s: unexpected insert into %s", tc.name, table)
			}
		}
	}
	_ = owner
}
