package sql_test

import (
	"context"
	"path/filepath"
	"testing"

	adminSQL "gopherbin/admin/sql"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	pasteCommon "gopherbin/paste/common"
	pasteSQL "gopherbin/paste/sql"

	pkgErrors "github.com/pkg/errors"
)

const testPassword = "Correct-Horse-Battery-Staple-G0pherbin-2024!"

func testDBConfig(t *testing.T) config.Database {
	t.Helper()
	return config.Database{
		DbBackend: config.SQLiteBackend,
		SQLite:    config.SQLite{DBFile: filepath.Join(t.TempDir(), "test.db")},
	}
}

// newPasterFixture creates a DB, runs migrations, creates a superuser, and
// returns a Paster plus an authenticated context for that user.
func newPasterFixture(t *testing.T) (pasteCommon.Paster, context.Context) {
	t.Helper()
	dbCfg := testDBConfig(t)

	paster, err := pasteSQL.NewPaster(dbCfg)
	if err != nil {
		t.Fatalf("NewPaster: %v", err)
	}
	mgr, err := adminSQL.NewUserManager(dbCfg)
	if err != nil {
		t.Fatalf("NewUserManager: %v", err)
	}
	super, err := mgr.CreateSuperUser(params.NewUserParams{
		Email:    "super@example.com",
		Username: "superadmin",
		FullName: "Super Admin",
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("CreateSuperUser: %v", err)
	}
	ctx := auth.PopulateContext(context.Background(), super)
	return paster, ctx
}

func isNotFound(err error) bool {
	_, ok := pkgErrors.Cause(err).(*gErrors.NotFoundError)
	return ok
}

func pInt(n int) *int { return &n }

// mustCreate is a helper to create a paste and fail the test on error.
func mustCreate(t *testing.T, paster pasteCommon.Paster, ctx context.Context, title string, public bool, maxAccesses *int) params.Paste {
	t.Helper()
	p, err := paster.Create(ctx, []byte("paste content"), title, "text", "", nil, public, "", nil, maxAccesses)
	if err != nil {
		t.Fatalf("Create(%q): %v", title, err)
	}
	return p
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_WithoutMaxAccesses(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "no-limit", false, nil)
	if p.MaxAccesses != nil {
		t.Errorf("MaxAccesses: want nil, got %d", *p.MaxAccesses)
	}
	if p.AccessCount != 0 {
		t.Errorf("AccessCount: want 0, got %d", p.AccessCount)
	}
}

func TestCreate_WithMaxAccesses(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "self-destruct", false, pInt(3))
	if p.MaxAccesses == nil || *p.MaxAccesses != 3 {
		t.Errorf("MaxAccesses: want 3, got %v", p.MaxAccesses)
	}
	if p.AccessCount != 0 {
		t.Errorf("AccessCount: want 0, got %d", p.AccessCount)
	}
}

// ── Self-destruct via Get ─────────────────────────────────────────────────────

func TestGet_NoSelfDestructWithoutMaxAccesses(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "persistent", false, nil)

	for i := 0; i < 5; i++ {
		if _, err := paster.Get(ctx, p.PasteID); err != nil {
			t.Fatalf("Get attempt %d: %v", i+1, err)
		}
	}
}

func TestGet_AccessCountIncrements(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "counted", false, pInt(5))

	got, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessCount != 1 {
		t.Errorf("AccessCount after 1 get: want 1, got %d", got.AccessCount)
	}

	got, err = paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessCount != 2 {
		t.Errorf("AccessCount after 2 gets: want 2, got %d", got.AccessCount)
	}
}

func TestGet_SelfDestructAfterSingleAccess(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "one-time", false, pInt(1))

	got, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if string(got.Data) != "paste content" {
		t.Errorf("data: want %q, got %q", "paste content", string(got.Data))
	}

	_, err = paster.Get(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("second Get: want NotFound, got %v", err)
	}
}

func TestGet_SelfDestructAfterNthAccess(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "three-time", false, pInt(3))

	for i := 1; i <= 3; i++ {
		if _, err := paster.Get(ctx, p.PasteID); err != nil {
			t.Fatalf("Get attempt %d: %v", i, err)
		}
	}

	_, err := paster.Get(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("Get after exhaustion: want NotFound, got %v", err)
	}
}

func TestGet_LastAccessReturnsData(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "last-read", false, pInt(2))

	paster.Get(ctx, p.PasteID) //nolint:errcheck

	got, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("final Get: %v", err)
	}
	if string(got.Data) != "paste content" {
		t.Errorf("final Get data: want %q, got %q", "paste content", string(got.Data))
	}
}

// ── Self-destruct via GetPublicPaste ─────────────────────────────────────────

func TestGetPublicPaste_SelfDestructAfterSingleAccess(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "public-one-time", true, pInt(1))

	got, err := paster.GetPublicPaste(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("first GetPublicPaste: %v", err)
	}
	if string(got.Data) != "paste content" {
		t.Errorf("data: want %q, got %q", "paste content", string(got.Data))
	}

	_, err = paster.GetPublicPaste(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("second GetPublicPaste: want NotFound, got %v", err)
	}
}

func TestGetPublicPaste_NoSelfDestructWithoutMaxAccesses(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "public-persistent", true, nil)

	for i := 0; i < 5; i++ {
		if _, err := paster.GetPublicPaste(ctx, p.PasteID); err != nil {
			t.Fatalf("GetPublicPaste attempt %d: %v", i+1, err)
		}
	}
}

func TestGetPublicPaste_AccessCountIncrements(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "public-counted", true, pInt(5))

	got, err := paster.GetPublicPaste(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("GetPublicPaste: %v", err)
	}
	if got.AccessCount != 1 {
		t.Errorf("AccessCount after 1 get: want 1, got %d", got.AccessCount)
	}
}

// ── Delete does not affect access counting ────────────────────────────────────

func TestDelete_RemovesPasteImmediately(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "to-delete", false, nil)

	if err := paster.Delete(ctx, p.PasteID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := paster.Get(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("Get after Delete: want NotFound, got %v", err)
	}
}
