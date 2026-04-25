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
		SQLite: config.SQLite{
			DBFile: filepath.Join(t.TempDir(), "gopherbin_test.db"),
		},
	}
}

// newFixture creates an in-memory SQLite paster and an authenticated context
// for a superuser. Each test gets its own isolated database via t.TempDir().
func newFixture(t *testing.T) (pasteCommon.Paster, context.Context) {
	t.Helper()
	dbCfg := testDBConfig(t)

	paster, err := pasteSQL.NewPaster(dbCfg)
	if err != nil {
		t.Fatalf("NewPaster: %v", err)
	}

	userMgr, err := adminSQL.NewUserManager(dbCfg)
	if err != nil {
		t.Fatalf("NewUserManager: %v", err)
	}

	user, err := userMgr.CreateSuperUser(params.NewUserParams{
		Email:    "admin@example.com",
		Username: "testadmin",
		FullName: "Test Admin",
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("CreateSuperUser: %v", err)
	}

	return paster, auth.PopulateContext(context.Background(), user)
}

func intPtr(n int) *int { return &n }

func createPaste(t *testing.T, paster pasteCommon.Paster, ctx context.Context, public bool, views *int) params.Paste {
	t.Helper()
	p, err := paster.Create(ctx, []byte("test content"), "test-title", "text", "test desc", nil, public, "", views, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return p
}

func isNotFound(err error) bool {
	return pkgErrors.Cause(err) == gErrors.ErrNotFound
}

func TestSelfDestruct_CreateStoresViewsRemaining(t *testing.T) {
	paster, ctx := newFixture(t)

	p, err := paster.Create(ctx, []byte("hello"), "title", "text", "", nil, false, "", intPtr(3), nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ViewsRemaining == nil || *p.ViewsRemaining != 3 {
		t.Fatalf("expected ViewsRemaining=3, got %v", p.ViewsRemaining)
	}
}

func TestSelfDestruct_NilViewsRemaining_NeverDeleted(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, false, nil)

	for i := 0; i < 5; i++ {
		if _, err := paster.Get(ctx, p.PasteID); err != nil {
			t.Fatalf("Get #%d: %v", i+1, err)
		}
	}
}

// TestSelfDestruct_DecrementsOnGet verifies the view counter decrements in the DB
// on each Get. The returned value reflects the count before the decrement,
// so the decrement is visible on the following Get.
func TestSelfDestruct_DecrementsOnGet(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, false, intPtr(3))

	got1, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if got1.ViewsRemaining == nil || *got1.ViewsRemaining != 3 {
		t.Fatalf("first Get: expected ViewsRemaining=3, got %v", got1.ViewsRemaining)
	}

	// Second Get reads the value written after the first decrement (3→2).
	got2, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got2.ViewsRemaining == nil || *got2.ViewsRemaining != 2 {
		t.Fatalf("second Get: expected ViewsRemaining=2, got %v", got2.ViewsRemaining)
	}
}

func TestSelfDestruct_DeletesAfterLastView(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, false, intPtr(2))

	// View 1 of 2.
	if _, err := paster.Get(ctx, p.PasteID); err != nil {
		t.Fatalf("first Get: %v", err)
	}

	// View 2 of 2 — content is returned, paste is deleted afterwards.
	got, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if string(got.Data) != "test content" {
		t.Fatalf("expected content on last view, got %q", got.Data)
	}

	// Paste is gone; next Get must return not-found.
	_, err = paster.Get(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound after self-destruct, got %v", err)
	}
}

func TestSelfDestruct_SingleView_DeletesOnFirstGet(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, false, intPtr(1))

	got, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got.Data) != "test content" {
		t.Fatalf("expected content on only view, got %q", got.Data)
	}

	_, err = paster.Get(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound after self-destruct, got %v", err)
	}
}

func TestSelfDestruct_PublicPaste_DeletesAfterLastView(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, true, intPtr(1))

	// Anonymous context — public endpoint requires no auth.
	got, err := paster.GetPublicPaste(context.Background(), p.PasteID)
	if err != nil {
		t.Fatalf("GetPublicPaste: %v", err)
	}
	if string(got.Data) != "test content" {
		t.Fatalf("expected content on only public view, got %q", got.Data)
	}

	_, err = paster.GetPublicPaste(context.Background(), p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound after self-destruct, got %v", err)
	}
}
