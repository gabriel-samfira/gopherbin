package sql_test

import (
	"context"
	"testing"

	adminSQL "gopherbin/admin/sql"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/params"
	pasteCommon "gopherbin/paste/common"
	pasteSQL "gopherbin/paste/sql"

	pkgErrors "github.com/pkg/errors"
)

type teamFixture struct {
	paster   pasteCommon.Paster
	teamMgr  pasteCommon.TeamManager
	ownerCtx context.Context
	otherCtx context.Context
}

func newTeamFixture(t *testing.T) teamFixture {
	t.Helper()
	dbCfg := testDBConfig(t)

	paster, err := pasteSQL.NewPaster(dbCfg)
	if err != nil {
		t.Fatalf("NewPaster: %v", err)
	}
	teamMgr, err := pasteSQL.NewTeamManager(dbCfg)
	if err != nil {
		t.Fatalf("NewTeamManager: %v", err)
	}
	userMgr, err := adminSQL.NewUserManager(dbCfg)
	if err != nil {
		t.Fatalf("NewUserManager: %v", err)
	}

	owner, err := userMgr.CreateSuperUser(params.NewUserParams{
		Email: "owner@example.com", Username: "owneruser", FullName: "Owner", Password: testPassword,
	})
	if err != nil {
		t.Fatalf("CreateSuperUser: %v", err)
	}
	ownerCtx := auth.PopulateContext(context.Background(), owner)

	other, err := userMgr.Create(ownerCtx, params.NewUserParams{
		Email: "other@example.com", Username: "otheruser", FullName: "Other", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}
	otherCtx := auth.PopulateContext(context.Background(), other)

	return teamFixture{paster: paster, teamMgr: teamMgr, ownerCtx: ownerCtx, otherCtx: otherCtx}
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestTeamCreate_Success(t *testing.T) {
	fix := newTeamFixture(t)
	team, err := fix.teamMgr.Create(fix.ownerCtx, "alpha")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if team.Name != "alpha" {
		t.Errorf("Name: want alpha, got %s", team.Name)
	}
}

func TestTeamCreate_DuplicateNameRejected(t *testing.T) {
	fix := newTeamFixture(t)
	if _, err := fix.teamMgr.Create(fix.ownerCtx, "beta"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := fix.teamMgr.Create(fix.ownerCtx, "beta")
	if pkgErrors.Cause(err) != gErrors.ErrDuplicateEntity {
		t.Fatalf("expected ErrDuplicateEntity, got %v", err)
	}
}

// ── Get ──────────────────────────────────────────────────────────────────────

func TestTeamGet_OwnerAccess(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "gamma")

	team, err := fix.teamMgr.Get(fix.ownerCtx, "gamma")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if team.Name != "gamma" {
		t.Errorf("Name: want gamma, got %s", team.Name)
	}
}

func TestTeamGet_NonMemberDenied(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "delta")

	_, err := fix.teamMgr.Get(fix.otherCtx, "delta")
	if err == nil {
		t.Fatal("expected error for non-member accessing team")
	}
}

// ── AddMember / RemoveMember ─────────────────────────────────────────────────

func TestTeamAddMember_MemberGainsAccess(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "epsilon")

	// Verify access denied before adding.
	if _, err := fix.teamMgr.Get(fix.otherCtx, "epsilon"); err == nil {
		t.Fatal("expected error before adding member")
	}

	if _, err := fix.teamMgr.AddMember(fix.ownerCtx, "epsilon", "other@example.com"); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	// Member should now have access.
	if _, err := fix.teamMgr.Get(fix.otherCtx, "epsilon"); err != nil {
		t.Fatalf("Get after AddMember: %v", err)
	}
}

func TestTeamRemoveMember_RevokesAccess(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "zeta")
	fix.teamMgr.AddMember(fix.ownerCtx, "zeta", "other@example.com")

	if err := fix.teamMgr.RemoveMember(fix.ownerCtx, "zeta", "other@example.com"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	if _, err := fix.teamMgr.Get(fix.otherCtx, "zeta"); err == nil {
		t.Fatal("expected error after RemoveMember")
	}
}

func TestTeamAddMember_RequiresOwner(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "eta")

	_, err := fix.teamMgr.AddMember(fix.otherCtx, "eta", "owner@example.com")
	if err == nil {
		t.Fatal("expected error when non-owner adds member")
	}
}

// ── ListMembers ───────────────────────────────────────────────────────────────

func TestTeamListMembers_IncludesOwnerAndMembers(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "theta")
	fix.teamMgr.AddMember(fix.ownerCtx, "theta", "other@example.com")

	members, err := fix.teamMgr.ListMembers(fix.ownerCtx, "theta")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	// Owner + 1 member = 2 entries.
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestTeamDelete_OwnerCanDelete(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "iota")

	if err := fix.teamMgr.Delete(fix.ownerCtx, "iota"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := fix.teamMgr.Get(fix.ownerCtx, "iota"); err == nil {
		t.Fatal("expected error after team deletion")
	}
}

func TestTeamDelete_NonOwnerDenied(t *testing.T) {
	fix := newTeamFixture(t)
	fix.teamMgr.Create(fix.ownerCtx, "kappa")
	fix.teamMgr.AddMember(fix.ownerCtx, "kappa", "other@example.com")

	if err := fix.teamMgr.Delete(fix.otherCtx, "kappa"); err == nil {
		t.Fatal("expected error when non-owner deletes team")
	}
}
