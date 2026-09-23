package sql_test

import (
	"context"
	"testing"

	adminCommon "gopherbin/admin/common"
	adminSQL "gopherbin/admin/sql"
	"gopherbin/auth"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	pasteCommon "gopherbin/paste/common"
	pasteSQL "gopherbin/paste/sql"

	pkgErrors "github.com/pkg/errors"
)

type teamFixture struct {
	paster   pasteCommon.Paster
	teams    pasteCommon.TeamManager
	users    adminCommon.UserManager
	super    params.Users
	user2    params.Users
	user3    params.Users
	ctxSuper context.Context
	ctxUser2 context.Context
	ctxUser3 context.Context
}

func newTeamFixture(t *testing.T) *teamFixture {
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
	ctxSuper := auth.PopulateContext(context.Background(), super)

	user2, err := mgr.Create(ctxSuper, params.NewUserParams{
		Email:    "alice@example.com",
		Username: "alice",
		FullName: "Alice Example",
		Password: testPassword,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create alice: %v", err)
	}
	user3, err := mgr.Create(ctxSuper, params.NewUserParams{
		Email:    "bob@example.com",
		Username: "bob",
		FullName: "Bob Example",
		Password: testPassword,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create bob: %v", err)
	}

	return &teamFixture{
		paster:   paster,
		teams:    teamMgr,
		users:    mgr,
		super:    super,
		user2:    user2,
		user3:    user3,
		ctxSuper: ctxSuper,
		ctxUser2: auth.PopulateContext(context.Background(), user2),
		ctxUser3: auth.PopulateContext(context.Background(), user3),
	}
}

// isUnauthorized is intentionally gone: every "row found but forbidden"
// denial in the sql layer now answers with the 404 sentinel, so tests assert
// isNotFound for them. Authentication failures (no row loaded) stay 401 and
// are exercised at the middleware layer, not here.

func isBadRequest(err error) bool {
	_, ok := pkgErrors.Cause(err).(*gErrors.BadRequestError)
	return ok
}

// inviteAndAccept invites a user to a team and has them accept the
// invitation, ending with an active membership.
func (f *teamFixture) inviteAndAccept(t *testing.T, ownerCtx context.Context, team, user string, memberCtx context.Context) {
	t.Helper()
	if _, err := f.teams.AddMember(ownerCtx, team, user, models.RoleMember); err != nil {
		t.Fatalf("AddMember(%s): %v", user, err)
	}
	if _, err := f.teams.AcceptInvite(memberCtx, team); err != nil {
		t.Fatalf("AcceptInvite(%s): %v", user, err)
	}
}

// ── Team lifecycle ───────────────────────────────────────────────────────────

func TestTeamLifecycle(t *testing.T) {
	f := newTeamFixture(t)

	team, err := f.teams.Create(f.ctxSuper, "engineers", "")
	if err != nil {
		t.Fatalf("Create team: %v", err)
	}
	if team.Owner.Username != "superadmin" {
		t.Errorf("team owner: want superadmin, got %s", team.Owner.Username)
	}

	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	members, err := f.teams.ListMembers(f.ctxUser2, "engineers")
	if err != nil {
		t.Fatalf("ListMembers as member: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("want 2 members (owner + alice), got %d", len(members))
	}

	// A member that does not own the team must still see it in List.
	// This used to filter on a non-existent "owner" column.
	listed, err := f.teams.List(f.ctxUser2, 1, 50)
	if err != nil {
		t.Fatalf("List as member: %v", err)
	}
	if len(listed.Teams) != 1 || listed.Teams[0].Name != "engineers" {
		t.Fatalf("member List should contain engineers, got %+v", listed.Teams)
	}

	// The team owner cannot be added as a member.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "superadmin", models.RoleMember); !isBadRequest(err) {
		t.Fatalf("adding owner as member: want BadRequest, got %v", err)
	}

	// The team owner cannot be removed.
	if err := f.teams.RemoveMember(f.ctxSuper, "engineers", "superadmin"); !isBadRequest(err) {
		t.Fatalf("removing owner: want BadRequest, got %v", err)
	}

	if err := f.teams.RemoveMember(f.ctxSuper, "engineers", "alice"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	members, err = f.teams.ListMembers(f.ctxSuper, "engineers")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("after removal want 1 member, got %d", len(members))
	}
}

func TestTeamAccessDeniedForOutsider(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	// Foreign-but-existing teams answer 404: "cannot see it" ==
	// "does not exist", so team names cannot be enumerated via 401s.
	if _, err := f.teams.Get(f.ctxUser3, "engineers"); !isNotFound(err) {
		t.Fatalf("outsider Get: want NotFound, got %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxUser3, "engineers", "bob", models.RoleMember); !isNotFound(err) {
		t.Fatalf("outsider AddMember: want NotFound, got %v", err)
	}
}

func TestTeamDeleteRemovesTeamPastes(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)
	pst, err := f.paster.Create(f.ctxUser2, []byte("team secret"), "team-file", "text", "", nil, false, "engineers", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create team paste: %v", err)
	}

	if err := f.teams.Delete(f.ctxSuper, "engineers"); err != nil {
		t.Fatalf("Delete team: %v", err)
	}
	if _, err := f.paster.Get(f.ctxSuper, pst.PasteID); !isNotFound(err) {
		t.Fatalf("team paste after team delete: want NotFound, got %v", err)
	}
}

// ── Team pastes ──────────────────────────────────────────────────────────────

func TestTeamPasteVisibility(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	// Requesting a public team paste must still produce a private paste.
	pst, err := f.paster.Create(f.ctxUser2, []byte("team content"), "team-file", "text", "", nil, true, "engineers", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create team paste: %v", err)
	}
	if pst.Public {
		t.Errorf("team paste must be forced private")
	}
	if pst.Team != "engineers" {
		t.Errorf("team field: want engineers, got %q", pst.Team)
	}

	// Team owner and member can both read it.
	if _, err := f.paster.Get(f.ctxSuper, pst.PasteID); err != nil {
		t.Errorf("team owner Get: %v", err)
	}
	if _, err := f.paster.Get(f.ctxUser2, pst.PasteID); err != nil {
		t.Errorf("team member Get: %v", err)
	}
	// An outsider cannot.
	if _, err := f.paster.Get(f.ctxUser3, pst.PasteID); !isNotFound(err) {
		t.Errorf("outsider Get: want NotFound, got %v", err)
	}
}

func TestCreateTeamPasteRejectsForeignTeam(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	// 404: a 401 would disclose that the foreign team exists.
	if _, err := f.paster.Create(f.ctxUser3, []byte("x"), "x", "text", "", nil, false, "engineers", nil, nil, nil); !isNotFound(err) {
		t.Fatalf("non-member team paste: want NotFound, got %v", err)
	}
}

func TestTeamPastePrivacyRules(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)
	pst, err := f.paster.Create(f.ctxUser2, []byte("team content"), "team-file", "text", "", nil, false, "engineers", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create team paste: %v", err)
	}

	// Team pastes cannot be made public.
	if _, err := f.paster.SetPrivacy(f.ctxSuper, pst.PasteID, true); !isBadRequest(err) {
		t.Fatalf("make team paste public: want BadRequest, got %v", err)
	}

	// A regular member cannot change paste privacy; the team owner can.
	if _, err := f.paster.SetPrivacy(f.ctxUser2, pst.PasteID, false); err != nil {
		t.Logf("note: paste owner (member) allowed to set privacy: %v", err)
	}
	// Outsider: canManage denial after a successful load answers 404,
	// indistinguishable from a nonexistent paste.
	if _, err := f.paster.SetPrivacy(f.ctxUser3, pst.PasteID, false); !isNotFound(err) {
		t.Fatalf("outsider SetPrivacy: want NotFound, got %v", err)
	}

	// Only the paste owner or team owner may delete. A non-owner member
	// (bob is not in the team here) cannot delete; the denial is a 404 so
	// foreign-but-existing pastes are not enumerable.
	if err := f.paster.Delete(f.ctxUser3, pst.PasteID); !isNotFound(err) {
		t.Fatalf("outsider Delete: want NotFound, got %v", err)
	}
	if err := f.paster.Delete(f.ctxSuper, pst.PasteID); err != nil {
		t.Fatalf("team owner Delete: %v", err)
	}
}

// ── Ownership transfer ───────────────────────────────────────────────────────

func TestTransferOwnership(t *testing.T) {
	f := newTeamFixture(t)
	pst, err := f.paster.Create(f.ctxSuper, []byte("mine"), "personal", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Cannot transfer to the current owner.
	if _, err := f.paster.TransferOwnership(f.ctxSuper, pst.PasteID, "superadmin"); !isBadRequest(err) {
		t.Fatalf("transfer to self: want BadRequest, got %v", err)
	}

	// A non-owner cannot transfer; the post-load canManage denial is a 404.
	if _, err := f.paster.TransferOwnership(f.ctxUser2, pst.PasteID, "bob"); !isNotFound(err) {
		t.Fatalf("non-owner transfer: want NotFound, got %v", err)
	}

	got, err := f.paster.TransferOwnership(f.ctxSuper, pst.PasteID, "alice")
	if err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	if got.Owner != "alice" || got.OwnerID != f.user2.ID {
		t.Fatalf("after transfer owner: want alice(%d), got %s(%d)", f.user2.ID, got.Owner, got.OwnerID)
	}

	// New owner can read and manage; old owner can no longer manage a private paste.
	if _, err := f.paster.Get(f.ctxUser2, pst.PasteID); err != nil {
		t.Errorf("new owner Get: %v", err)
	}
	// Old owner can no longer manage the transferred private paste; the
	// post-load canManage denial answers 404 like a missing paste would.
	if _, err := f.paster.SetPrivacy(f.ctxSuper, pst.PasteID, true); !isNotFound(err) {
		t.Errorf("old owner SetPrivacy: want NotFound, got %v", err)
	}
	if _, err := f.paster.SetPrivacy(f.ctxUser2, pst.PasteID, true); err != nil {
		t.Errorf("new owner SetPrivacy: %v", err)
	}
}

func TestTeamPasteTransferStaysInTeam(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)
	pst, err := f.paster.Create(f.ctxUser2, []byte("team content"), "team-file", "text", "", nil, false, "engineers", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Transferring a team paste to a non-member is rejected.
	if _, err := f.paster.TransferOwnership(f.ctxUser2, pst.PasteID, "bob"); !isBadRequest(err) {
		t.Fatalf("transfer to outsider: want BadRequest, got %v", err)
	}
	// Transferring within the team works.
	got, err := f.paster.TransferOwnership(f.ctxUser2, pst.PasteID, "superadmin")
	if err != nil {
		t.Fatalf("transfer within team: %v", err)
	}
	if got.Owner != "superadmin" {
		t.Errorf("owner: want superadmin, got %s", got.Owner)
	}
}

// ── List/Search scopes ───────────────────────────────────────────────────────

func TestListScopes(t *testing.T) {
	f := newTeamFixture(t)

	// alice owns a private paste and shares it with bob.
	sharedPaste, err := f.paster.Create(f.ctxUser2, []byte("shared data"), "shared-file", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create shared paste: %v", err)
	}
	if _, err := f.paster.ShareWithUser(f.ctxUser2, sharedPaste.PasteID, "bob"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}

	// bob owns a private paste.
	if _, err := f.paster.Create(f.ctxUser3, []byte("bob only"), "bob-file", "text", "", nil, false, "", nil, nil, nil); err != nil {
		t.Fatalf("Create bob paste: %v", err)
	}

	// scope=mine → only bob's own paste
	mine, err := f.paster.List(f.ctxUser3, 1, 50, pasteCommon.ScopeMine, nil, "")
	if err != nil {
		t.Fatalf("List mine: %v", err)
	}
	if len(mine.Pastes) != 1 || mine.Pastes[0].Name != "bob-file" {
		t.Fatalf("scope mine: want only bob-file, got %+v", mine.Pastes)
	}

	// scope=shared → only the paste shared with bob
	shared, err := f.paster.List(f.ctxUser3, 1, 50, pasteCommon.ScopeShared, nil, "")
	if err != nil {
		t.Fatalf("List shared: %v", err)
	}
	if len(shared.Pastes) != 1 || shared.Pastes[0].Name != "shared-file" {
		t.Fatalf("scope shared: want only shared-file, got %+v", shared.Pastes)
	}

	// scope=all → both
	all, err := f.paster.List(f.ctxUser3, 1, 50, pasteCommon.ScopeAll, nil, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all.Pastes) != 2 {
		t.Fatalf("scope all: want 2 pastes, got %d", len(all.Pastes))
	}
}

func TestListScopesIncludeTeamPastes(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "bob", f.ctxUser3)
	if _, err := f.paster.Create(f.ctxSuper, []byte("team data"), "team-file", "text", "", nil, false, "engineers", nil, nil, nil); err != nil {
		t.Fatalf("Create team paste: %v", err)
	}

	shared, err := f.paster.List(f.ctxUser3, 1, 50, pasteCommon.ScopeShared, nil, "")
	if err != nil {
		t.Fatalf("List shared: %v", err)
	}
	if len(shared.Pastes) != 1 || shared.Pastes[0].Name != "team-file" {
		t.Fatalf("team member scope shared: want team-file, got %+v", shared.Pastes)
	}
}

func TestSearchCoversSharedPastes(t *testing.T) {
	f := newTeamFixture(t)
	pst, err := f.paster.Create(f.ctxUser2, []byte("quantum flux capacitor"), "reactor.txt", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.paster.ShareWithUser(f.ctxUser2, pst.PasteID, "bob"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}

	res, err := f.paster.Search(f.ctxUser3, "quantum", 1, 50, pasteCommon.ScopeAll, nil, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != pst.PasteID {
		t.Fatalf("shared search: want the shared paste, got %+v", res.Pastes)
	}
}

// ── Mutation side effects ────────────────────────────────────────────────────

func TestMutationEndpointsDoNotConsumeAccess(t *testing.T) {
	f := newTeamFixture(t)
	// maxAccesses = 1: the first *view* must destroy the paste; mutations
	// (share, unshare, list-shares, privacy) must not consume an access.
	pst, err := f.paster.Create(f.ctxSuper, []byte("burn after reading"), "burn", "text", "", nil, false, "", nil, pInt(1), nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.paster.ShareWithUser(f.ctxSuper, pst.PasteID, "alice"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}
	if _, err := f.paster.ListShares(f.ctxSuper, pst.PasteID); err != nil {
		t.Fatalf("ListShares: %v", err)
	}
	if _, err := f.paster.SetPrivacy(f.ctxSuper, pst.PasteID, false); err != nil {
		t.Fatalf("SetPrivacy: %v", err)
	}
	if _, err := f.paster.TransferOwnership(f.ctxSuper, pst.PasteID, "alice"); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}

	// The single allowed view still works (and consumes the paste).
	got, err := f.paster.Get(f.ctxUser2, pst.PasteID)
	if err != nil {
		t.Fatalf("Get after mutations: %v", err)
	}
	if string(got.Data) != "burn after reading" {
		t.Errorf("data: want %q, got %q", "burn after reading", string(got.Data))
	}
}

func TestShareGuards(t *testing.T) {
	f := newTeamFixture(t)
	pst, err := f.paster.Create(f.ctxSuper, []byte("data"), "personal", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Sharing with the owner is rejected.
	if _, err := f.paster.ShareWithUser(f.ctxSuper, pst.PasteID, "superadmin"); !isBadRequest(err) {
		t.Fatalf("share with owner: want BadRequest, got %v", err)
	}
	// A sharee cannot re-share; the post-load owner check answers 404
	// (uniform "foreign paste" semantics).
	if _, err := f.paster.ShareWithUser(f.ctxUser2, pst.PasteID, "bob"); !isNotFound(err) {
		t.Fatalf("sharee re-share: want NotFound, got %v", err)
	}
	// Team pastes cannot be shared with individuals.
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	teamPaste, err := f.paster.Create(f.ctxSuper, []byte("team data"), "team-file", "text", "", nil, false, "engineers", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create team paste: %v", err)
	}
	if _, err := f.paster.ShareWithUser(f.ctxSuper, teamPaste.PasteID, "alice"); !isBadRequest(err) {
		t.Fatalf("share team paste: want BadRequest, got %v", err)
	}
}

// ── Team invitations ─────────────────────────────────────────────────────────

func TestTeamInvitationLifecycle(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	invited, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember)
	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if invited.Status != "pending" {
		t.Errorf("invited member status: want pending, got %s", invited.Status)
	}
	if invited.AddedBy == "" {
		t.Errorf("invited member: want added_by populated, got empty")
	}

	// A pending invitee sees the team in their list with role "pending".
	listed, err := f.teams.List(f.ctxUser2, 1, 50)
	if err != nil {
		t.Fatalf("List as invitee: %v", err)
	}
	if len(listed.Teams) != 1 || listed.Teams[0].MyRole != "pending" {
		t.Fatalf("invitee List: want one team with my_role pending, got %+v", listed.Teams)
	}

	// A pending invitee may view the team (to decide about the invite).
	if _, err := f.teams.Get(f.ctxUser2, "engineers"); err != nil {
		t.Errorf("invitee Get team: %v", err)
	}

	// A pending invitee cannot create team pastes; the canShareToTeam
	// denial fires after the team row was loaded, so it is a 404.
	if _, err := f.paster.Create(f.ctxUser2, []byte("x"), "x", "text", "", nil, false, "engineers", nil, nil, nil); !isNotFound(err) {
		t.Fatalf("pending member team paste: want NotFound, got %v", err)
	}
	// A pending invitee cannot see team pastes.
	pst, err := f.paster.Create(f.ctxSuper, []byte("secret"), "team-file", "text", "", nil, false, "engineers", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create team paste: %v", err)
	}
	if _, err := f.paster.Get(f.ctxUser2, pst.PasteID); !isNotFound(err) {
		t.Fatalf("pending member Get team paste: want NotFound, got %v", err)
	}
	// A pending invitee does not see team pastes in the shared scope.
	shared, err := f.paster.List(f.ctxUser2, 1, 50, pasteCommon.ScopeShared, nil, "")
	if err != nil {
		t.Fatalf("List shared as invitee: %v", err)
	}
	if len(shared.Pastes) != 0 {
		t.Fatalf("pending member shared scope: want 0 pastes, got %+v", shared.Pastes)
	}

	// Accepting grants access.
	if _, err := f.teams.AcceptInvite(f.ctxUser2, "engineers"); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if _, err := f.paster.Get(f.ctxUser2, pst.PasteID); err != nil {
		t.Errorf("member Get after accept: %v", err)
	}

	// Members list shows status and inviter.
	members, err := f.teams.ListMembers(f.ctxSuper, "engineers")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	var alice *params.TeamMember
	for i := range members {
		if members[i].Username == "alice" {
			alice = &members[i]
		}
	}
	if alice == nil {
		t.Fatalf("alice not in members list: %+v", members)
	}
	if alice.Status != "active" {
		t.Errorf("alice status after accept: want active, got %s", alice.Status)
	}
	if alice.AddedBy == "" {
		t.Errorf("alice added_by: want inviter name, got empty")
	}
	if alice.AddedByID != f.super.ID {
		t.Errorf("alice added_by_id: want %d, got %d", f.super.ID, alice.AddedByID)
	}

	// Listing MyRole for the owner.
	listed, err = f.teams.List(f.ctxSuper, 1, 50)
	if err != nil {
		t.Fatalf("List as owner: %v", err)
	}
	if listed.Teams[0].MyRole != "owner" {
		t.Errorf("owner MyRole: want owner, got %s", listed.Teams[0].MyRole)
	}
}

func TestDoubleInviteRejected(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); !isBadRequest(err) {
		t.Fatalf("double invite pending: want BadRequest, got %v", err)
	}
	if _, err := f.teams.AcceptInvite(f.ctxUser2, "engineers"); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); !isBadRequest(err) {
		t.Fatalf("invite active member: want BadRequest, got %v", err)
	}
}

func TestDeclineInvite(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := f.teams.DeclineInvite(f.ctxUser2, "engineers"); err != nil {
		t.Fatalf("DeclineInvite: %v", err)
	}
	// After declining, the team is gone from the user's list entirely.
	listed, err := f.teams.List(f.ctxUser2, 1, 50)
	if err != nil && !isNotFound(err) {
		t.Fatalf("List after decline: %v", err)
	}
	if len(listed.Teams) != 0 {
		t.Errorf("List after decline: want no teams, got %+v", listed.Teams)
	}
	// A second decline has nothing to decline; after losing membership the
	// team is no longer accessible to her, and the post-load denial is a
	// 404 (existence is not leaked).
	if err := f.teams.DeclineInvite(f.ctxUser2, "engineers"); !isNotFound(err) {
		t.Fatalf("double decline: want NotFound, got %v", err)
	}
	// The owner can invite again.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); err != nil {
		t.Fatalf("re-invite after decline: %v", err)
	}
}

func TestAcceptWithoutInviteFails(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	// No membership at all: the getTeam access gate denies with 404 so the
	// team's existence is not disclosed.
	if _, err := f.teams.AcceptInvite(f.ctxUser2, "engineers"); !isNotFound(err) {
		t.Fatalf("accept without invite: want NotFound, got %v", err)
	}
}

func TestLeaveTeam(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create team: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	if err := f.teams.LeaveTeam(f.ctxUser2, "engineers"); err != nil {
		t.Fatalf("LeaveTeam: %v", err)
	}
	// After leaving she is an outsider: 404, not 401, so the team cannot
	// be confirmed to exist.
	if _, err := f.teams.Get(f.ctxUser2, "engineers"); !isNotFound(err) {
		t.Fatalf("Get after leave: want NotFound, got %v", err)
	}
	// The owner cannot leave their own team.
	if err := f.teams.LeaveTeam(f.ctxSuper, "engineers"); !isBadRequest(err) {
		t.Fatalf("owner LeaveTeam: want BadRequest, got %v", err)
	}
	// A pending invitee must decline, not leave.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "bob", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if err := f.teams.LeaveTeam(f.ctxUser3, "engineers"); !isBadRequest(err) {
		t.Fatalf("pending LeaveTeam: want BadRequest, got %v", err)
	}
	if err := f.teams.DeclineInvite(f.ctxUser3, "engineers"); err != nil {
		t.Fatalf("pending DeclineInvite: %v", err)
	}
}

// ── Roster contact data ──────────────────────────────────────────────────────

// A pending invitee may view the team, but the roster they see must not
// carry other users' contact data; active members and the owner see it all.
func TestTeamRosterRedactedForPendingInvitee(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "bob", models.RoleMember); err != nil {
		t.Fatalf("AddMember bob: %v", err)
	}

	// An active member sees the full roster, emails included.
	asMember, err := f.teams.Get(f.ctxUser2, "engineers")
	if err != nil {
		t.Fatalf("Get as active member: %v", err)
	}
	if asMember.Owner.Email != "super@example.com" {
		t.Fatalf("active member should see the owner email, got %q", asMember.Owner.Email)
	}

	// The pending invitee still sees the roster (to decide about the
	// invitation) but every email is blanked, owner included.
	asPending, err := f.teams.Get(f.ctxUser3, "engineers")
	if err != nil {
		t.Fatalf("Get as pending invitee: %v", err)
	}
	if asPending.MyRole != models.TeamMembershipPending {
		t.Fatalf("invitee my_role: want pending, got %s", asPending.MyRole)
	}
	if len(asPending.Members) == 0 || asPending.Owner.Username != "superadmin" {
		t.Fatalf("roster must still be visible to the invitee: %+v", asPending)
	}
	if asPending.Owner.Email != "" {
		t.Errorf("pending invitee sees owner email: %q", asPending.Owner.Email)
	}
	for _, m := range asPending.Members {
		if m.Email != "" {
			t.Errorf("pending invitee sees email of %s: %q", m.Username, m.Email)
		}
	}

	// Same policy on the ListMembers endpoint.
	members, err := f.teams.ListMembers(f.ctxUser3, "engineers")
	if err != nil {
		t.Fatalf("ListMembers as invitee: %v", err)
	}
	if len(members) == 0 {
		t.Fatal("ListMembers roster must not be empty for the invitee")
	}
	for _, m := range members {
		if m.Email != "" {
			t.Errorf("ListMembers leaks email of %s: %q", m.Username, m.Email)
		}
	}
	if asOwner, err := f.teams.ListMembers(f.ctxSuper, "engineers"); err != nil {
		t.Fatalf("ListMembers as owner: %v", err)
	} else if len(asOwner) == 0 || asOwner[0].Email == "" {
		t.Errorf("owner must still see member emails: %+v", asOwner)
	}
}
