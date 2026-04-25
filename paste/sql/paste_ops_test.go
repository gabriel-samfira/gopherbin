package sql_test

import (
	"context"
	"strings"
	"testing"
	"time"

	adminSQL "gopherbin/admin/sql"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	pasteCommon "gopherbin/paste/common"
	pasteSQL "gopherbin/paste/sql"

	pkgErrors "github.com/pkg/errors"
)

// newTwoUserFixture returns a paster and two independent user contexts.
func newTwoUserFixture(t *testing.T) (pasteCommon.Paster, context.Context, context.Context) {
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

	super, err := userMgr.CreateSuperUser(params.NewUserParams{
		Email: "owner@example.com", Username: "owneruser", FullName: "Owner", Password: testPassword,
	})
	if err != nil {
		t.Fatalf("CreateSuperUser: %v", err)
	}
	superCtx := auth.PopulateContext(context.Background(), super)

	other, err := userMgr.Create(superCtx, params.NewUserParams{
		Email: "other@example.com", Username: "otheruser", FullName: "Other", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create other user: %v", err)
	}
	otherCtx := auth.PopulateContext(context.Background(), other)

	return paster, superCtx, otherCtx
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestCreate_EmptyDataReturnsError(t *testing.T) {
	paster, ctx := newFixture(t)
	_, err := paster.Create(ctx, []byte{}, "title", "text", "", nil, false, "", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestCreate_EmptyTitleReturnsError(t *testing.T) {
	paster, ctx := newFixture(t)
	_, err := paster.Create(ctx, []byte("content"), "", "text", "", nil, false, "", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestCreate_WithMetadata(t *testing.T) {
	paster, ctx := newFixture(t)
	meta := map[string]string{"key": "value", "foo": "bar"}
	p, err := paster.Create(ctx, []byte("content"), "title", "text", "", nil, false, "", nil, meta)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := paster.Get(ctx, p.PasteID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata["key"] != "value" || got.Metadata["foo"] != "bar" {
		t.Errorf("metadata not preserved: %v", got.Metadata)
	}
}

func TestCreate_ExpiredPasteNotAccessible(t *testing.T) {
	paster, ctx := newFixture(t)
	past := time.Now().Add(-time.Minute)
	p, err := paster.Create(ctx, []byte("content"), "title", "text", "", &past, false, "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = paster.Get(ctx, p.PasteID)
	if pkgErrors.Cause(err) != gErrors.ErrNotFound {
		t.Fatalf("expected ErrNotFound for expired paste, got %v", err)
	}
}

// ── Get / access control ─────────────────────────────────────────────────────

func TestGet_NonOwnerCannotAccessPrivatePaste(t *testing.T) {
	paster, ownerCtx, otherCtx := newTwoUserFixture(t)
	p := createPaste(t, paster, ownerCtx, false, nil)

	_, err := paster.Get(otherCtx, p.PasteID)
	if pkgErrors.Cause(err) != gErrors.ErrNotFound {
		t.Fatalf("expected ErrNotFound for non-owner accessing private paste, got %v", err)
	}
}

func TestGet_PublicPasteAccessibleByAnyone(t *testing.T) {
	paster, ownerCtx, otherCtx := newTwoUserFixture(t)
	p := createPaste(t, paster, ownerCtx, true, nil)

	// Other authenticated user via Get.
	if _, err := paster.Get(otherCtx, p.PasteID); err != nil {
		t.Fatalf("Get public paste as other user: %v", err)
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestList_ReturnsPastes(t *testing.T) {
	paster, ctx := newFixture(t)
	createPaste(t, paster, ctx, false, nil)
	createPaste(t, paster, ctx, false, nil)

	res, err := paster.List(ctx, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Pastes) < 2 {
		t.Fatalf("expected at least 2 pastes, got %d", len(res.Pastes))
	}
}

func TestList_OnlyReturnsOwnerPastes(t *testing.T) {
	paster, ownerCtx, otherCtx := newTwoUserFixture(t)
	createPaste(t, paster, ownerCtx, false, nil)

	res, err := paster.List(otherCtx, 1, 10)
	if err != nil {
		t.Fatalf("List as other user: %v", err)
	}
	if len(res.Pastes) != 0 {
		t.Fatalf("expected 0 pastes for other user, got %d", len(res.Pastes))
	}
}

func TestList_PreviewTruncatesLargeData(t *testing.T) {
	paster, ctx := newFixture(t)
	bigContent := []byte(strings.Repeat("x", 1024))
	p, err := paster.Create(ctx, bigContent, "big-paste", "text", "", nil, false, "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := paster.List(ctx, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found params.Paste
	for _, rp := range res.Pastes {
		if rp.PasteID == p.PasteID {
			found = rp
			break
		}
	}
	if len(found.Data) > 512 {
		t.Fatalf("expected preview <= 512 bytes, got %d", len(found.Data))
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestDelete_OwnerCanDelete(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, false, nil)

	if err := paster.Delete(ctx, p.PasteID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := paster.Get(ctx, p.PasteID)
	if !isNotFound(err) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_NonOwnerCannotDelete(t *testing.T) {
	paster, ownerCtx, otherCtx := newTwoUserFixture(t)
	p := createPaste(t, paster, ownerCtx, false, nil)

	// otherCtx cannot even see the paste, so Delete should return not-found.
	if err := paster.Delete(otherCtx, p.PasteID); err == nil {
		t.Fatal("expected error when non-owner deletes paste")
	}
}

// ── SetPrivacy ────────────────────────────────────────────────────────────────

func TestSetPrivacy_TogglesPublicFlag(t *testing.T) {
	paster, ctx := newFixture(t)
	p := createPaste(t, paster, ctx, false, nil)

	updated, err := paster.SetPrivacy(ctx, p.PasteID, true)
	if err != nil {
		t.Fatalf("SetPrivacy(true): %v", err)
	}
	if !updated.Public {
		t.Error("expected Public=true after SetPrivacy(true)")
	}

	updated, err = paster.SetPrivacy(ctx, p.PasteID, false)
	if err != nil {
		t.Fatalf("SetPrivacy(false): %v", err)
	}
	if updated.Public {
		t.Error("expected Public=false after SetPrivacy(false)")
	}
}

// ── Sharing ───────────────────────────────────────────────────────────────────

func TestShare_AllowsSharedUser(t *testing.T) {
	paster, ownerCtx, otherCtx := newTwoUserFixture(t)
	p := createPaste(t, paster, ownerCtx, false, nil)

	// Verify other user cannot access before sharing.
	if _, err := paster.Get(otherCtx, p.PasteID); err == nil {
		t.Fatal("expected error before sharing")
	}

	otherUser, _ := paster.Get(ownerCtx, p.PasteID) // fetch to get owner info; share by email
	_ = otherUser
	if _, err := paster.ShareWithUser(ownerCtx, p.PasteID, "other@example.com"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}

	// Other user should now be able to access.
	if _, err := paster.Get(otherCtx, p.PasteID); err != nil {
		t.Fatalf("Get after share: %v", err)
	}
}

func TestShare_ListShares(t *testing.T) {
	paster, ownerCtx, _ := newTwoUserFixture(t)
	p := createPaste(t, paster, ownerCtx, false, nil)

	if _, err := paster.ShareWithUser(ownerCtx, p.PasteID, "other@example.com"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}
	shares, err := paster.ListShares(ownerCtx, p.PasteID)
	if err != nil {
		t.Fatalf("ListShares: %v", err)
	}
	if len(shares.Users) != 1 {
		t.Fatalf("expected 1 share, got %d", len(shares.Users))
	}
}

func TestUnshare_RevokesAccess(t *testing.T) {
	paster, ownerCtx, otherCtx := newTwoUserFixture(t)
	p := createPaste(t, paster, ownerCtx, false, nil)

	if _, err := paster.ShareWithUser(ownerCtx, p.PasteID, "other@example.com"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}
	if _, err := paster.Get(otherCtx, p.PasteID); err != nil {
		t.Fatalf("Get after share: %v", err)
	}

	if err := paster.UnshareWithUser(ownerCtx, p.PasteID, "other@example.com"); err != nil {
		t.Fatalf("UnshareWithUser: %v", err)
	}
	if _, err := paster.Get(otherCtx, p.PasteID); err == nil {
		t.Fatal("expected error after unsharing")
	}
}

// ── Search ───────────────────────────────────────────────────────────────────

func TestSearch_FindsByName(t *testing.T) {
	paster, ctx := newFixture(t)
	_, err := paster.Create(ctx, []byte("content"), "uniquesearchtarget", "text", "", nil, false, "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := paster.Search(ctx, "uniquesearchtarget", 1, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Pastes) == 0 {
		t.Fatal("expected at least one search result")
	}
}
