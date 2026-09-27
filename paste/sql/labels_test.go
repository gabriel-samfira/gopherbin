package sql_test

import (
	"testing"

	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	pasteCommon "gopherbin/paste/common"

	pkgErrors "github.com/pkg/errors"
)

// ── Team update / stats / labels ─────────────────────────────────────────────

func TestTeamUpdateRenameAndDescription(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", "original description"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	team, err := f.teams.Get(f.ctxSuper, "engineers")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if team.Description != "original description" {
		t.Fatalf("expected description to be stored, got %q", team.Description)
	}

	// Non-members cannot update; the denial fires after the team row was
	// loaded, so it answers 404 and does not confirm the team exists.
	if _, err := f.teams.Update(f.ctxUser3, "engineers", params.UpdateTeamParams{}); !isNotFound(err) {
		t.Fatalf("expected not-found update for outsider, got %v", err)
	}

	desc := "updated"
	team, err = f.teams.Update(f.ctxSuper, "engineers", params.UpdateTeamParams{Description: &desc})
	if err != nil {
		t.Fatalf("Update description: %v", err)
	}
	if team.Description != "updated" {
		t.Fatalf("description not updated: %q", team.Description)
	}

	newName := "platform"
	team, err = f.teams.Update(f.ctxSuper, "engineers", params.UpdateTeamParams{Name: &newName})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if team.Name != "platform" {
		t.Fatalf("rename did not propagate: %q", team.Name)
	}
	if _, err := f.teams.Get(f.ctxSuper, "platform"); err != nil {
		t.Fatalf("team unreachable under new name: %v", err)
	}
	if _, err := f.teams.Get(f.ctxSuper, "engineers"); !isNotFound(err) {
		t.Fatalf("old name should be gone, got %v", err)
	}

	// Renaming onto an existing team fails.
	if _, err := f.teams.Create(f.ctxUser2, "second", ""); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if _, err := f.teams.Update(f.ctxUser2, "second", params.UpdateTeamParams{Name: &newName}); !pkgErrors.Is(err, gErrors.ErrDuplicateEntity) {
		t.Fatalf("duplicate-name rename: want ErrDuplicateEntity, got %v", err)
	}

	// Membership follows the rename.
	f.inviteAndAccept(t, f.ctxSuper, "platform", "alice", f.ctxUser2)
	listed, err := f.teams.List(f.ctxUser2, 1, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, team := range listed.Teams {
		if team.Name == "platform" {
			found = true
		}
	}
	if !found {
		t.Fatal("membership did not follow the rename")
	}
}

func TestTeamStats(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)
	// bob is invited but never accepts
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "bob", models.RoleMember); err != nil {
		t.Fatalf("AddMember bob: %v", err)
	}

	if _, err := f.paster.Create(f.ctxSuper, []byte("a"), "a.txt", "text", "", nil, false, "engineers", nil, nil, []string{"x"}); err != nil {
		t.Fatalf("Create paste: %v", err)
	}
	if _, err := f.paster.Create(f.ctxUser2, []byte("b"), "b.txt", "text", "", nil, false, "engineers", nil, nil, nil); err != nil {
		t.Fatalf("Create paste 2: %v", err)
	}
	// personal paste must not count
	if _, err := f.paster.Create(f.ctxSuper, []byte("c"), "c.txt", "text", "", nil, false, "", nil, nil, nil); err != nil {
		t.Fatalf("Create personal paste: %v", err)
	}

	team, err := f.teams.Get(f.ctxSuper, "engineers")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if team.Stats == nil {
		t.Fatal("expected stats on team detail")
	}
	if team.Stats.Members != 2 {
		t.Fatalf("expected 2 members (owner+alice), got %d", team.Stats.Members)
	}
	if team.Stats.Pending != 1 {
		t.Fatalf("expected 1 pending invite, got %d", team.Stats.Pending)
	}
	if team.Stats.Pastes != 2 {
		t.Fatalf("expected 2 team pastes, got %d", team.Stats.Pastes)
	}
	if team.Stats.Contributors != 2 {
		t.Fatalf("expected 2 contributors, got %d", team.Stats.Contributors)
	}
}

func TestTeamLabels(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	team, err := f.teams.SetLabels(f.ctxSuper, "engineers", []string{"Infra", "infra", "release"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	if len(team.Labels) != 2 || team.Labels[0] != "infra" || team.Labels[1] != "release" {
		t.Fatalf("expected normalized deduped labels, got %v", team.Labels)
	}

	// An active member may manage them too, and removing drops the label.
	if _, err := f.teams.SetLabels(f.ctxUser2, "engineers", []string{"infra"}); err != nil {
		t.Fatalf("member SetLabels: %v", err)
	}
	team, err = f.teams.Get(f.ctxSuper, "engineers")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(team.Labels) != 1 || team.Labels[0] != "infra" {
		t.Fatalf("label removal failed: %v", team.Labels)
	}

	// A pending invitee may not; the isMember denial fires after the team
	// load and answers 404 like a missing team would.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "bob", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if _, err := f.teams.SetLabels(f.ctxUser3, "engineers", []string{"sneaky"}); !isNotFound(err) {
		t.Fatalf("expected not-found for pending invitee, got %v", err)
	}
}

// ── Paste labels & filtering ─────────────────────────────────────────────────

func TestPasteLabelsAndFiltering(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	// Personal paste with personal labels (create-on-use).
	personal, err := f.paster.Create(f.ctxUser2, []byte("flux notes"), "reactor.txt", "text", "", nil, false, "", nil, nil, []string{"WIP", "reactor"})
	if err != nil {
		t.Fatalf("Create personal: %v", err)
	}
	if len(personal.Labels) != 2 {
		t.Fatalf("expected 2 labels back on create, got %v", personal.Labels)
	}
	if personal.Labels[0].Scope != "personal" {
		t.Fatalf("expected personal scope, got %+v", personal.Labels[0])
	}

	// Team paste with team labels.
	teamPaste, err := f.paster.Create(f.ctxSuper, []byte("runbook"), "runbook.md", "text", "", nil, false, "engineers", nil, nil, []string{"infra"})
	if err != nil {
		t.Fatalf("Create team paste: %v", err)
	}
	if len(teamPaste.Labels) != 1 || teamPaste.Labels[0].Scope != "team" || teamPaste.Labels[0].Team != "engineers" {
		t.Fatalf("expected team-scoped label, got %+v", teamPaste.Labels)
	}

	// Alice filters her visible pastes by her own label.
	res, err := f.paster.List(f.ctxUser2, 1, 50, pasteCommon.ScopeAll, []string{"wip"}, "")
	if err != nil {
		t.Fatalf("List by label: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != personal.PasteID {
		t.Fatalf("label filter returned %+v", res.Pastes)
	}

	// Two labels => intersection.
	if _, err := f.paster.Create(f.ctxUser2, []byte("other"), "other.txt", "text", "", nil, false, "", nil, nil, []string{"wip"}); err != nil {
		t.Fatalf("Create second wip paste: %v", err)
	}
	res, err = f.paster.List(f.ctxUser2, 1, 50, pasteCommon.ScopeAll, []string{"wip", "reactor"}, "")
	if err != nil {
		t.Fatalf("List by two labels: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != personal.PasteID {
		t.Fatalf("label intersection wrong: %+v", res.Pastes)
	}

	// Labels from foreign teams are invisible to outsiders.
	outsider, err := f.paster.List(f.ctxUser3, 1, 50, pasteCommon.ScopeAll, []string{"infra"}, "")
	if err != nil {
		t.Fatalf("outsider List: %v", err)
	}
	if len(outsider.Pastes) != 0 {
		t.Fatalf("outsider must not resolve team labels: %+v", outsider.Pastes)
	}

	// Team member resolves the team label via the shared vocabulary.
	res, err = f.paster.List(f.ctxUser2, 1, 50, pasteCommon.ScopeAll, []string{"infra"}, "")
	if err != nil {
		t.Fatalf("member List team label: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != teamPaste.PasteID {
		t.Fatalf("team label filter failed: %+v", res.Pastes)
	}

	// Team filter alone.
	res, err = f.paster.List(f.ctxUser2, 1, 50, pasteCommon.ScopeAll, nil, "engineers")
	if err != nil {
		t.Fatalf("List by team: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != teamPaste.PasteID {
		t.Fatalf("team filter failed: %+v", res.Pastes)
	}

	// Combined text search + label filter.
	res, err = f.paster.Search(f.ctxUser2, "flux", 1, 50, pasteCommon.ScopeAll, []string{"wip"}, "")
	if err != nil {
		t.Fatalf("Search with label: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != personal.PasteID {
		t.Fatalf("search+label failed: %+v", res.Pastes)
	}
	res, err = f.paster.Search(f.ctxUser2, "flux", 1, 50, pasteCommon.ScopeAll, []string{"nonexistent"}, "")
	if err != nil {
		t.Fatalf("Search with missing label: %v", err)
	}
	if len(res.Pastes) != 0 {
		t.Fatalf("label should have narrowed away the match: %+v", res.Pastes)
	}
}

func TestPasteSetLabels(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	pst, err := f.paster.Create(f.ctxUser2, []byte("data"), "x.txt", "text", "", nil, false, "", nil, nil, []string{"draft"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, err := f.paster.SetLabels(f.ctxUser2, pst.PasteID, []string{"done", "archive"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	if len(updated.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %+v", updated.Labels)
	}
	for _, l := range updated.Labels {
		if l.Name == "draft" {
			t.Fatal("replaced set still contains dropped label")
		}
	}

	// Non-owner cannot relabel; the canManage denial is answered with 404
	// so foreign paste IDs are not distinguishable from missing ones.
	if _, err := f.paster.SetLabels(f.ctxUser3, pst.PasteID, []string{"hijack"}); !isNotFound(err) {
		t.Fatalf("expected not-found for non-owner relabel, got %v", err)
	}

	// Invalid label characters are rejected.
	if _, err := f.paster.SetLabels(f.ctxUser2, pst.PasteID, []string{"has,comma"}); !isBadRequest(err) {
		t.Fatalf("expected bad request for invalid label, got %v", err)
	}
}

func TestLabelVocabulary(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)
	if _, err := f.teams.SetLabels(f.ctxSuper, "engineers", []string{"infra"}); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	if _, err := f.paster.Create(f.ctxUser2, []byte("d"), "n", "text", "", nil, false, "", nil, nil, []string{"wip"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	vocab, err := f.paster.ListLabels(f.ctxUser2)
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(vocab.Personal) != 1 || vocab.Personal[0] != "wip" {
		t.Fatalf("personal vocabulary wrong: %+v", vocab)
	}
	if len(vocab.Teams) != 1 || vocab.Teams[0].Team != "engineers" || len(vocab.Teams[0].Labels) != 1 {
		t.Fatalf("team vocabulary wrong: %+v", vocab)
	}

	// Outsiders see neither personal nor team vocab of others.
	outsider, err := f.paster.ListLabels(f.ctxUser3)
	if err != nil {
		t.Fatalf("ListLabels outsider: %v", err)
	}
	if len(outsider.Personal) != 0 || len(outsider.Teams) != 0 {
		t.Fatalf("outsider vocabulary must be empty: %+v", outsider)
	}
}

// ── Invite search with privacy opt-out ───────────────────────────────────────

func TestUserSearchAndPrivacyOptOut(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.inviteAndAccept(t, f.ctxSuper, "engineers", "bob", f.ctxUser3)

	results, err := f.users.SearchUsers(f.ctxSuper, "ali", "")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(results) != 1 || results[0].Username != "alice" {
		t.Fatalf("expected alice, got %+v", results)
	}

	// Short queries return nothing (no enumeration of the whole userbase).
	results, err = f.users.SearchUsers(f.ctxSuper, "a", "")
	if err != nil {
		t.Fatalf("SearchUsers short: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("1-char query must return nothing, got %+v", results)
	}

	// Team exclusion: bob is already on engineers, alice is not.
	results, err = f.users.SearchUsers(f.ctxSuper, "bo", "engineers")
	if err != nil {
		t.Fatalf("SearchUsers excludeTeam: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("existing member should be excluded, got %+v", results)
	}
	results, err = f.users.SearchUsers(f.ctxSuper, "ali", "engineers")
	if err != nil {
		t.Fatalf("SearchUsers excludeTeam other: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("non-member should still be listed, got %+v", results)
	}

	// Opt-out hides alice from search...
	hidden := false
	if _, err := f.users.Update(f.ctxUser2, f.user2.ID, params.UpdateUserPayload{Discoverable: &hidden}); err != nil {
		t.Fatalf("Update discoverable: %v", err)
	}
	results, err = f.users.SearchUsers(f.ctxSuper, "ali", "")
	if err != nil {
		t.Fatalf("SearchUsers after opt-out: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("opted-out user must not be listed, got %+v", results)
	}

	// ...but exact-username invites still work.
	if _, err := f.teams.AddMember(f.ctxSuper, "engineers", "alice", models.RoleMember); err != nil {
		t.Fatalf("AddMember by exact username after opt-out: %v", err)
	}
}

// ── Label management: color, rename/merge, delete, permissions ──────────────

func ownedLabel(t *testing.T, labels []params.LabelInfo, name string) params.LabelInfo {
	t.Helper()
	for _, l := range labels {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("label %q not found in %+v", name, labels)
	return params.LabelInfo{}
}

func TestLabelManagementColorRenameDelete(t *testing.T) {
	f := newTeamFixture(t)
	onePaste, err := f.paster.Create(f.ctxUser2, []byte("a"), "one.txt", "text", "", nil, false, "", nil, nil, []string{"urgent", "wip"})
	if err != nil {
		t.Fatalf("Create one: %v", err)
	}
	if _, err := f.paster.Create(f.ctxUser2, []byte("b"), "two.txt", "text", "", nil, false, "", nil, nil, []string{"urgent"}); err != nil {
		t.Fatalf("Create two: %v", err)
	}

	owned, err := f.paster.ListOwnedLabels(f.ctxUser2)
	if err != nil {
		t.Fatalf("ListOwnedLabels: %v", err)
	}
	if len(owned) != 2 {
		t.Fatalf("expected 2 personal labels, got %+v", owned)
	}
	urgent := ownedLabel(t, owned, "urgent")
	wip := ownedLabel(t, owned, "wip")
	if urgent.Usage != 2 || wip.Usage != 1 {
		t.Fatalf("usage counts wrong: urgent=%d wip=%d", urgent.Usage, wip.Usage)
	}

	// Recolor; the paste response must carry the color.
	red := "#ff0000"
	info, err := f.paster.UpdateLabel(f.ctxUser2, urgent.ID, params.UpdateLabelParams{Color: &red})
	if err != nil || info.Color != red {
		t.Fatalf("UpdateLabel color: %v %+v", err, info)
	}
	pst, err := f.paster.Get(f.ctxUser2, onePaste.PasteID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(pst.Labels) == 0 || pst.Labels[0].Color == "" {
		t.Fatalf("paste labels missing color: %+v", pst.Labels)
	}

	// Invalid color rejected by the payload contract.
	if err := (params.UpdateLabelParams{Color: strPtr("crimson")}).Validate(); !isBadRequest(err) {
		t.Fatalf("invalid color: want BadRequest, got %v", err)
	}

	// Foreign users cannot touch personal labels. The label row was loaded
	// before the check, so both denials answer 404: label IDs (small
	// integers) must not be enumerable via 401-vs-404 differences.
	if _, err := f.paster.UpdateLabel(f.ctxUser3, urgent.ID, params.UpdateLabelParams{Color: &red}); !isNotFound(err) {
		t.Fatalf("expected not-found for foreign recolor, got %v", err)
	}
	if err := f.paster.DeleteLabel(f.ctxUser3, urgent.ID); !isNotFound(err) {
		t.Fatalf("expected not-found for foreign delete, got %v", err)
	}

	// Rename wip -> urgent merges into the existing label.
	wipName := "urgent"
	info, err = f.paster.UpdateLabel(f.ctxUser2, wip.ID, params.UpdateLabelParams{Name: &wipName})
	if err != nil {
		t.Fatalf("rename merge: %v", err)
	}
	if info.ID != urgent.ID || info.Usage != 2 {
		t.Fatalf("merge did not fold usage into target: %+v", info)
	}
	owned, _ = f.paster.ListOwnedLabels(f.ctxUser2)
	if len(owned) != 1 || owned[0].Name != "urgent" || owned[0].Usage != 2 {
		t.Fatalf("post-merge vocabulary wrong: %+v", owned)
	}

	// Delete removes it from every paste.
	if err := f.paster.DeleteLabel(f.ctxUser2, urgent.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	owned, _ = f.paster.ListOwnedLabels(f.ctxUser2)
	if len(owned) != 0 {
		t.Fatalf("expected empty vocabulary, got %+v", owned)
	}
	pst, err = f.paster.Get(f.ctxUser2, onePaste.PasteID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if len(pst.Labels) != 0 {
		t.Fatalf("labels not detached from paste: %+v", pst.Labels)
	}
	if err := f.paster.DeleteLabel(f.ctxUser2, urgent.ID); !isNotFound(err) {
		t.Fatalf("second delete should 404, got %v", err)
	}
}

func TestTeamLabelColorPermissions(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	team, err := f.teams.SetLabels(f.ctxSuper, "engineers", []string{"infra"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	if len(team.LabelDetails) != 1 || team.LabelDetails[0].Name != "infra" {
		t.Fatalf("LabelDetails missing: %+v", team.LabelDetails)
	}
	labelID := team.LabelDetails[0].ID

	f.inviteAndAccept(t, f.ctxSuper, "engineers", "alice", f.ctxUser2)

	teal := "#008080"
	green := "#00ff00"
	info, err := f.paster.UpdateLabel(f.ctxUser2, labelID, params.UpdateLabelParams{Color: &teal})
	if err != nil || info.Color != teal {
		t.Fatalf("active member should recolor team label: %v %+v", err, info)
	}
	// Outsider recolor of a team label: 404, same non-enumerable answer.
	if _, err := f.paster.UpdateLabel(f.ctxUser3, labelID, params.UpdateLabelParams{Color: &green}); !isNotFound(err) {
		t.Fatalf("outsider should get not-found for team label recolor, got %v", err)
	}
	team, err = f.teams.Get(f.ctxUser2, "engineers")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(team.LabelDetails) != 1 || team.LabelDetails[0].Color != teal {
		t.Fatalf("team label color not persisted: %+v", team.LabelDetails)
	}
}

func strPtr(s string) *string { return &s }

// Viewers are read-only members: they may see the team's labels but not
// curate the shared vocabulary, which would strip labels from every team
// paste.
func TestTeamLabelsReadOnlyForViewers(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "engineers", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	team, err := f.teams.SetLabels(f.ctxSuper, "engineers", []string{"infra"})
	if err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
	labelID := team.LabelDetails[0].ID
	f.inviteAndAcceptRole(t, f.ctxSuper, "engineers", "alice", models.RoleViewer, f.ctxUser2)

	if _, err := f.teams.SetLabels(f.ctxUser2, "engineers", nil); !isNotFound(err) {
		t.Fatalf("viewer SetLabels: want NotFound, got %v", err)
	}
	teal := "#008080"
	if _, err := f.paster.UpdateLabel(f.ctxUser2, labelID, params.UpdateLabelParams{Color: &teal}); !isNotFound(err) {
		t.Fatalf("viewer UpdateLabel: want NotFound, got %v", err)
	}
	if err := f.paster.DeleteLabel(f.ctxUser2, labelID); !isNotFound(err) {
		t.Fatalf("viewer DeleteLabel: want NotFound, got %v", err)
	}
	if got, err := f.teams.Get(f.ctxUser2, "engineers"); err != nil {
		t.Fatalf("viewer Get: %v", err)
	} else if len(got.Labels) != 1 {
		t.Fatalf("viewer should still see the labels, got %v", got.Labels)
	}
}

// A rename that merges into an existing label applies a requested color to
// the label that survives the merge.
func TestLabelRenameMergeKeepsRequestedColor(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.paster.Create(f.ctxUser2, []byte("a"), "one.txt", "text", "", nil, false, "", nil, nil, []string{"urgent", "wip"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	owned, err := f.paster.ListOwnedLabels(f.ctxUser2)
	if err != nil {
		t.Fatalf("ListOwnedLabels: %v", err)
	}
	urgent := ownedLabel(t, owned, "urgent")
	wip := ownedLabel(t, owned, "wip")

	name, red := "urgent", "#ff0000"
	info, err := f.paster.UpdateLabel(f.ctxUser2, wip.ID, params.UpdateLabelParams{Name: &name, Color: &red})
	if err != nil {
		t.Fatalf("rename merge: %v", err)
	}
	if info.ID != urgent.ID || info.Color != red {
		t.Fatalf("merged label: want id %d color %s, got %+v", urgent.ID, red, info)
	}
	owned, _ = f.paster.ListOwnedLabels(f.ctxUser2)
	if len(owned) != 1 || owned[0].Color != red || owned[0].Usage != 1 {
		t.Fatalf("post-merge vocabulary: %+v", owned)
	}
}

// Transferring a personal paste must not hand the previous owner's personal
// labels (names included) to the new owner.
func TestTransferDropsPreviousOwnersLabels(t *testing.T) {
	f := newTeamFixture(t)
	pst, err := f.paster.Create(f.ctxSuper, []byte("x"), "p", "text", "", nil, false, "", nil, nil, []string{"secret-project"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.paster.ShareWithUser(f.ctxSuper, pst.PasteID, "alice"); err != nil {
		t.Fatalf("ShareWithUser: %v", err)
	}
	got, err := f.paster.TransferOwnership(f.ctxSuper, pst.PasteID, "alice")
	if err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	if len(got.Labels) != 0 {
		t.Fatalf("transfer response carries old labels: %+v", got.Labels)
	}
	view, err := f.paster.Get(f.ctxUser2, pst.PasteID)
	if err != nil {
		t.Fatalf("new owner Get: %v", err)
	}
	if len(view.Labels) != 0 {
		t.Fatalf("new owner sees the previous owner's labels: %+v", view.Labels)
	}
	owned, _ := f.paster.ListOwnedLabels(f.ctxSuper)
	if len(owned) != 1 || owned[0].Usage != 0 {
		t.Fatalf("previous owner's label should stay but be unused: %+v", owned)
	}
	shares, err := f.paster.ListShares(f.ctxUser2, pst.PasteID)
	if err != nil {
		t.Fatalf("ListShares: %v", err)
	}
	if len(shares.Users) != 0 {
		t.Fatalf("new owner should not remain a sharee: %+v", shares.Users)
	}
}

// The invite type-ahead only honors the team filter for teams the caller
// belongs to; otherwise it would reveal foreign rosters.
func TestUserSearchTeamFilterRequiresMembership(t *testing.T) {
	f := newTeamFixture(t)
	if _, err := f.teams.Create(f.ctxSuper, "secret", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.teams.AddMember(f.ctxSuper, "secret", "alice", models.RoleMember); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	// bob is an outsider: the filter is ignored for him.
	res, err := f.users.SearchUsers(f.ctxUser3, "alice", "secret")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("outsider search should ignore the team filter, got %+v", res)
	}
	// For the owner, pending invitees are filtered out.
	res, err = f.users.SearchUsers(f.ctxSuper, "alice", "secret")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("pending invitee should be filtered for the owner, got %+v", res)
	}
	// An admin inviting on the owner's behalf does not get the owner back.
	f.inviteAndAcceptRole(t, f.ctxSuper, "secret", "bob", models.RoleAdmin, f.ctxUser3)
	res, err = f.users.SearchUsers(f.ctxUser3, "super", "secret")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("team owner should be filtered out, got %+v", res)
	}
	// LIKE wildcards in the query are matched literally.
	if res, err := f.users.SearchUsers(f.ctxUser3, "%%", ""); err != nil || len(res) != 0 {
		t.Fatalf("wildcard query should match nothing: %+v %v", res, err)
	}
}
