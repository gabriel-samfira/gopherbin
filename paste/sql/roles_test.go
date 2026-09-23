package sql_test

import (
	"context"
	"testing"

	"gopherbin/models"
	"gopherbin/params"
)

// inviteAndAcceptRole invites a user with a specific role and accepts.
func (f *teamFixture) inviteAndAcceptRole(t *testing.T, ownerCtx context.Context, team, user, role string, memberCtx context.Context) {
	t.Helper()
	if _, err := f.teams.AddMember(ownerCtx, team, user, role); err != nil {
		t.Fatalf("AddMember(%s as %s): %v", user, role, err)
	}
	if _, err := f.teams.AcceptInvite(memberCtx, team); err != nil {
		t.Fatalf("AcceptInvite(%s): %v", user, err)
	}
}

func TestTeamRoleInvitePermissions(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Owner can invite an admin.
	f.inviteAndAcceptRole(t, f.ctxSuper, "engineers", "alice", models.RoleAdmin, f.ctxUser2)
	// Admin can invite a plain member.
	f.inviteAndAcceptRole(t, f.ctxUser2, "engineers", "bob", models.RoleMember, f.ctxUser3)

	// A plain member cannot invite anyone.
	if _, err := f.teams.AddMember(f.ctxUser3, "engineers", "superadmin", models.RoleMember); !isUnauthorized(err) {
		t.Fatalf("member invite: want unauthorized, got %v", err)
	}
	// An admin cannot invite other admins.
	if _, err := f.teams.AddMember(f.ctxUser2, "engineers", "bob", models.RoleAdmin); !isUnauthorized(err) {
		t.Fatalf("admin inviting admin: want unauthorized, got %v", err)
	}
	// Invalid roles are rejected.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "bob", "wizard"); !isBadRequest(err) {
		t.Fatalf("invalid role: want bad request, got %v", err)
	}

	// Roles are visible in the member list.
	members, err := f.teams.ListMembers(f.ctxSuper, "engineers")
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	want := map[string]string{"alice": models.RoleAdmin, "bob": models.RoleMember}
	for _, m := range members {
		if role, ok := want[m.Username]; ok && m.Role != role {
			t.Errorf("role of %s: want %s, got %s", m.Username, role, m.Role)
		}
		if m.Username == "superadmin" && m.Role != "" {
			t.Errorf("owner should carry no stored role, got %q", m.Role)
		}
	}
}

func TestTeamRoleRemoveRules(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAcceptRole(t, f.ctxSuper, "engineers", "alice", models.RoleAdmin, f.ctxUser2)
	f.inviteAndAcceptRole(t, f.ctxSuper, "engineers", "bob", models.RoleMember, f.ctxUser3)

	// Admin may remove a plain member.
	if err := f.teams.RemoveMember(f.ctxUser2, "engineers", "bob"); err != nil {
		t.Fatalf("admin removing member: %v", err)
	}
	// Only the owner may remove another admin.
	if err := f.teams.RemoveMember(f.ctxUser2, "engineers", "alice"); !isUnauthorized(err) {
		t.Fatalf("admin removing self(admin): want unauthorized, got %v", err)
	}
	if err := f.teams.RemoveMember(f.ctxUser3, "engineers", "alice"); !isUnauthorized(err) {
		t.Fatalf("removed member removing admin: want unauthorized, got %v", err)
	}
	// Nobody but removal target: the owner is untouchable.
	if err := f.teams.RemoveMember(f.ctxUser2, "engineers", "superadmin"); !isBadRequest(err) {
		t.Fatalf("removing owner: want bad request, got %v", err)
	}
	// The owner may remove an admin.
	if err := f.teams.RemoveMember(f.ctxSuper, "engineers", "alice"); err != nil {
		t.Fatalf("owner removing admin: %v", err)
	}
}

func TestTeamRoleShareGate(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAcceptRole(t, f.ctxSuper, "engineers", "alice", models.RoleViewer, f.ctxUser2)

	// Viewers can see the team...
	if _, err := f.teams.Get(f.ctxUser2, "engineers"); err != nil {
		t.Fatalf("viewer reading team: %v", err)
	}
	// ...but cannot share team-scoped pastes.
	if _, err := f.paster.Create(f.ctxUser2, []byte("x"), "x", "text", "", nil, false, "engineers", nil, nil, nil); !isUnauthorized(err) {
		t.Fatalf("viewer sharing team paste: want unauthorized, got %v", err)
	}

	// Members can share.
	f.inviteAndAcceptRole(t, f.ctxSuper, "engineers", "bob", models.RoleMember, f.ctxUser3)
	if _, err := f.paster.Create(f.ctxUser3, []byte("x"), "x", "text", "", nil, false, "engineers", nil, nil, nil); err != nil {
		t.Fatalf("member sharing team paste: %v", err)
	}

	// A demotion takes effect immediately.
	if _, err := f.teams.SetMemberRole(f.ctxSuper, "engineers", "bob", models.RoleViewer); err != nil {
		t.Fatalf("demote bob: %v", err)
	}
	if _, err := f.paster.Create(f.ctxUser3, []byte("x"), "x", "text", "", nil, false, "engineers", nil, nil, nil); !isUnauthorized(err) {
		t.Fatalf("demoted member sharing: want unauthorized, got %v", err)
	}
	// A promotion restores it.
	if _, err := f.teams.SetMemberRole(f.ctxSuper, "engineers", "bob", models.RoleAdmin); err != nil {
		t.Fatalf("promote bob: %v", err)
	}
	if _, err := f.paster.Create(f.ctxUser3, []byte("x"), "x", "text", "", nil, false, "engineers", nil, nil, nil); err != nil {
		t.Fatalf("promoted member sharing: %v", err)
	}
}

func TestTeamRoleManagement(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	// Only the owner may change roles.
	if _, err := f.teams.SetMemberRole(f.ctxUser2, "engineers", "bob", models.RoleAdmin); !isUnauthorized(err) {
		t.Fatalf("self role change: want unauthorized, got %v", err)
	}
	// Owner promotes alice, who then may invite.
	if _, err := f.teams.SetMemberRole(f.ctxSuper, "engineers", "alice", models.RoleAdmin); err != nil {
		t.Fatalf("promote alice: %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxUser2, "engineers", "bob", models.RoleMember); err != nil {
		t.Fatalf("newly promoted admin inviting: %v", err)
	}
	// The owner's role is not assignable.
	if _, err := f.teams.SetMemberRole(f.ctxSuper, "engineers", "superadmin", models.RoleViewer); !isBadRequest(err) {
		t.Fatalf("demoting owner: want bad request, got %v", err)
	}
	// Invalid role strings are rejected.
	if _, err := f.teams.SetMemberRole(f.ctxSuper, "engineers", "alice", "wizard"); !isBadRequest(err) {
		t.Fatalf("invalid role: want bad request, got %v", err)
	}
}

func TestTeamTransfer(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "bob", f.ctxUser3)
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	// Non-owners cannot request a transfer.
	if _, err := f.teams.RequestTransfer(f.ctxUser2, "engineers", "bob"); !isUnauthorized(err) {
		t.Fatalf("non-owner transfer: want unauthorized, got %v", err)
	}
	// Transfers target accepted members only.
	if _, err := f.users.Create(f.ctxSuper, params.NewUserParams{
		Email: "carol@example.com", Username: "carol", FullName: "Carol Example",
		Password: testPassword, Enabled: true,
	}); err != nil {
		t.Fatalf("Create carol: %v", err)
	}
	if _, err := f.teams.RequestTransfer(f.ctxSuper, "engineers", "carol"); !isBadRequest(err) {
		t.Fatalf("transfer to non-member: want bad request, got %v", err)
	}
	if _, err := f.teams.RequestTransfer(f.ctxSuper, "engineers", "superadmin"); !isBadRequest(err) {
		t.Fatalf("transfer to self: want bad request, got %v", err)
	}

	team, err := f.teams.RequestTransfer(f.ctxSuper, "engineers", "bob")
	if err != nil {
		t.Fatalf("RequestTransfer: %v", err)
	}
	if team.TransferTo == nil || team.TransferTo.Username != "bob" {
		t.Fatalf("pending transfer target: %+v", team.TransferTo)
	}
	// A second request while one is pending is rejected.
	if _, err := f.teams.RequestTransfer(f.ctxSuper, "engineers", "bob"); !isBadRequest(err) {
		t.Fatalf("double transfer: want bad request, got %v", err)
	}

	// The notice surfaces for the target with the inviter attached.
	notices, err := f.teams.ListPendingTransfers(f.ctxUser3)
	if err != nil {
		t.Fatalf("ListPendingTransfers: %v", err)
	}
	if len(notices) != 1 || notices[0].TeamName != "engineers" || notices[0].FromUser == "" {
		t.Fatalf("transfer notice: %+v", notices)
	}
	if asOther, err := f.teams.ListPendingTransfers(f.ctxUser2); err != nil || len(asOther) != 0 {
		t.Fatalf("transfer notice for unrelated user: %+v %v", asOther, err)
	}

	// A member the transfer was not offered to cannot accept it.
	if _, err := f.teams.AcceptTransfer(f.ctxUser2, "engineers"); !isBadRequest(err) {
		t.Fatalf("wrong user accepting: want bad request, got %v", err)
	}

	// Bob accepts: ownership moves, the old owner stays as admin.
	accepted, err := f.teams.AcceptTransfer(f.ctxUser3, "engineers")
	if err != nil {
		t.Fatalf("AcceptTransfer: %v", err)
	}
	if accepted.Owner.Username != "bob" || accepted.TransferTo != nil {
		t.Fatalf("after accept: owner=%+v transfer=%+v", accepted.Owner, accepted.TransferTo)
	}
	if accepted.MyRole != models.RoleOwner {
		t.Errorf("bob my_role: want owner, got %s", accepted.MyRole)
	}
	if got, err := f.teams.Get(f.ctxSuper, "engineers"); err != nil {
		t.Fatalf("old owner reading team: %v", err)
	} else if got.MyRole != models.RoleAdmin {
		t.Errorf("old owner role after transfer: want admin, got %s", got.MyRole)
	}

	// The previous owner (now admin) can still manage members but cannot
	// remove the owner.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "carol", models.RoleMember); err != nil {
		t.Fatalf("old owner inviting: %v", err)
	}
	if err := f.teams.RemoveMember(f.ctxSuper, "engineers", "bob"); !isBadRequest(err) {
		t.Fatalf("removing owner: want bad request, got %v", err)
	}
	// The new owner can remove the admin.
	if err := f.teams.RemoveMember(f.ctxUser3, "engineers", "superadmin"); err != nil {
		t.Fatalf("owner removing ex-owner admin: %v", err)
	}
	if after, err := f.teams.Get(f.ctxUser3, "engineers"); err != nil {
		t.Fatalf("Get: %v", err)
	} else if after.MyRole != models.RoleOwner {
		t.Fatalf("bob should still be owner, got %s", after.MyRole)
	}
}

func TestTeamTransferDeclineAndCancel(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "bob", f.ctxUser3)

	// Decline by the target leaves ownership untouched.
	if _, err := f.teams.RequestTransfer(f.ctxSuper, "engineers", "bob"); err != nil {
		t.Fatalf("RequestTransfer: %v", err)
	}
	if err := f.teams.DeclineTransfer(f.ctxUser3, "engineers"); err != nil {
		t.Fatalf("DeclineTransfer: %v", err)
	}
	if after, err := f.teams.Get(f.ctxSuper, "engineers"); err != nil {
		t.Fatalf("Get: %v", err)
	} else if after.TransferTo != nil || after.Owner.Username != "superadmin" {
		t.Fatalf("after decline: %+v", after)
	}

	// Cancel by the owner clears the pending offer.
	if _, err := f.teams.RequestTransfer(f.ctxSuper, "engineers", "bob"); err != nil {
		t.Fatalf("RequestTransfer: %v", err)
	}
	if err := f.teams.CancelTransfer(f.ctxUser3, "engineers"); !isUnauthorized(err) {
		t.Fatalf("target cancelling: want unauthorized, got %v", err)
	}
	if err := f.teams.CancelTransfer(f.ctxSuper, "engineers"); err != nil {
		t.Fatalf("CancelTransfer: %v", err)
	}
	if err := f.teams.CancelTransfer(f.ctxSuper, "engineers"); !isBadRequest(err) {
		t.Fatalf("cancelling twice: want bad request, got %v", err)
	}
}
