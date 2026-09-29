# Code review: commits `7926bbf..fd0ad3d`

## Summary

I reviewed the 25 commits after `7926bbf7ff9393432adcc176b61f587e4b324ad6` (up to `fd0ad3d`). They add teams, invitations and roles, ownership transfer, labels, the access-count gate, a security-hardening series, the OpenAPI spec and generated web types, and the SvelteKit UI for all of it. Outside `vendor/` and lockfiles that is 88 files and about 21,400 added lines. About 7,500 of those lines are generated (`swagger.yaml`, `openapi3.yaml`, `schema.d.ts`).

The review found real defects. The most serious were:
- access-control bugs that let people keep or regain access they should not have;
- a confirm-before-burn feature the web UI bypassed;
- queries that fail with syntax errors on MySQL;
- a contract test whose route-coverage check tested nothing.

Everything listed under *Findings* is fixed. Every backend bug was first reproduced with a failing test, or on a real MySQL server, and the new tests fail on the original code.

The fixes are two commits on the branch `review-fixes`, cut from `master` at `fd0ad3d`:

| Commit | Scope |
|---|---|
| `1e6e0ad` fix: review fixes for the teams, labels and hardening series | 52 files, +2214 / −837 |
| `0484e02` fix: trusted proxies for login throttling, admin-on-admin rules, search | 21 files, +551 / −246 |

To land them:

```bash
git checkout master && git merge --ff-only review-fixes
```

## How the review was done

- **Backend.** I read every changed Go file in full, not just the diff hunks: models, `paste/sql` (paste, teams, labels), `admin/sql`, controllers, routers, the rate limiter, config, the web UI handler and apigen. For every suspected defect I wrote a failing test first, then fixed it, then ran the new test against a worktree of the original `fd0ad3d` to prove it catches the bug.
- **Frontend and tooling.** Two reviewer agents ran in parallel: one on the Svelte code, one on the API spec tooling and test quality. I checked each of their findings against the code before acting on it. Some findings were dropped as working as designed; the rest were fixed.
- **MySQL.** The project supports MySQL, but only SQLite was ever tested. I added a switch that runs the SQL test suites against MySQL (`GOPHERBIN_TEST_MYSQL`) and ran them against a throwaway MySQL 8.4 container.
- **Mutation testing of the tests.** I deliberately broke the code, for example by adding an undocumented route, adding an undocumented field, removing the FK-suspension restore, or reverting the header change. In each case I checked that the relevant test now fails.

### Verification status (final state of `review-fixes`)

| Check | Result |
|---|---|
| `make tests` (gofmt check plus all Go tests, SQLite) | pass |
| `go vet -tags fts5 ./...` | clean |
| SQL suites (`paste/sql`, `admin/sql`, `workers`) against MySQL 8.4 | pass |
| `go test -tags fts5,webui ./webui/` (CSP hashes against the real bundle) | pass |
| `npm run check` (svelte-check) | 0 errors, 0 warnings |
| `npx vitest run` (integration tests against a live server) | 20/20 |
| `npm run build` | ok |
| `make generate` leaves `swagger.yaml` and the generated TS types unchanged | yes |
| actionlint on `.github/workflows/ci.yml` | clean |

---

## Findings and fixes

Severity reflects impact in a multi-user deployment. Files and lines refer to the state after the fix.

### 1. Access control

#### 1.1 Critical: renaming a team re-added removed members and declined invitees as *active* members
- **Cause.** `teamManager.Update` loaded the team with its `Members` preloaded and wrote it back with GORM `Save`. `Save` upserts associations, so it re-inserted every `team_users` row that existed at load time. A row deleted in the meantime (a removal, a declined invitation or someone leaving) came back with the column defaults `status='active', role='member'`. A declined invitation could therefore turn into full membership.
- **Same pattern elsewhere.** The same "`Save` or `Create` on a struct with loaded associations" pattern was in four more places:
  - `SetPrivacy` re-inserted shares and labels, and wrote back a stale `access_count`, undoing a concurrent view.
  - `TransferOwnership` did the same, and also upserted the target user with their `MemberOf` teams.
  - Paste `Create` upserted the owner together with the owner's preloaded `MemberOf` rows.
  - `ShareWithUser` upserted the target's `users` row, which could re-create a user deleted concurrently.
- **Fix.** These mutations now update only the columns or rows they mean to (`Update`/`Updates`/`UpdateColumn`, and a typed `pasteShare` join-row insert). The `Preload("MemberOf")` calls that were never read are gone.
- **Test.** `TestMutationsDoNotRewriteAssociations` hooks GORM's create callback and fails on any insert into a table the mutation has no business writing. It fails on the original code.

#### 1.2 High: former team members kept their team pastes
- **Cause.** `canAccess` and `canManage` treated authorship as sufficient. After being removed from a team, or leaving it, a user could still:
  - read the team pastes they had written;
  - delete them or transfer them;
  - set labels on them, which creates labels in the team's vocabulary;
  - list them under "Mine".

  The UI's own "Remove member" dialog already said such pastes "will no longer be visible to them".
- **Fix.** Team pastes are governed by membership alone. `canAccess` requires team owner or accepted member. `canManage` requires team owner, or paste owner *while still an accepted member*. The SQL `scopeClause` for list and search mirrors this, and direct shares only apply to personal pastes.
- **UI.** The "Leave team" dialog now states this rule.
- **Test.** `TestFormerMemberLosesTeamPasteAccess`.

#### 1.3 High: stale ownership offers could be accepted
- **Cause.** When the owner offered the team to member X and X then left or was removed, `teams.transfer_to_user_id` still pointed at X. If X was later re-invited, X could accept the old offer and become owner, even while the new invitation was still pending.
- **Fix.**
  - Leaving and removal (`dropMembership`) withdraw the offer in the same transaction.
  - `AcceptTransfer` re-checks active membership.
  - The ownership change is a conditional update (`WHERE owner_id = ? AND transfer_to_user_id = ?`), so a concurrent cancel cannot be overtaken.
- **Test.** `TestTransferOfferWithdrawnWhenTargetDeparts`.

#### 1.4 High: "read-only" viewers could delete team labels
- **Cause.** Invitations describe the viewer role as "Viewer (read-only)", but `TeamManager.SetLabels` and `canManageLabel` allowed any active member. A viewer could rename, recolour or delete team labels, and deleting one strips it from every team paste.
- **Fix.** Label management requires the owner or an admin/member. Team pastes also can no longer be transferred to a viewer (`canShareToTeam`).
- **UI.** The label editor is hidden for viewers.
- **Test.** `TestTeamLabelsReadOnlyForViewers`.

#### 1.5 High: previews leaked the content of limited-view pastes
- **Cause.** List and search return a 512-byte preview without consuming a view. Since the new scopes added "shared with me" and team pastes, sharees and team members could read short limited-view pastes in full, repeatedly, without using up a view. The list queries did not even select `max_accesses`, so the field was always empty on list rows.
- **Fix.** The list and search queries select `max_accesses` and `access_count`. Previews of limited-view pastes are only returned to their owner.
- **UI.** The list shows a click-to-open placeholder instead of the preview, and titles are now clickable.
- **Test.** `TestListPreviewWithheldForLimitedPastesOfOthers`.

#### 1.6 High: the confirm-before-burn gate was bypassed by the UI
- **Cause.** Commit `2126d00` made limited-view reads require an `X-Consume-Access` header, but the SPA sent it on every paste open. Merely opening a paste link consumed a view, including links an attacker redirects a logged-in victim to, and the gate never fired.
- **Fix.** `/p/[id]` and `/public/p/[id]` read without the header first. On a 403 they show `ConfirmViewNotice` ("Opening it uses up one of its views…") and repeat the read with the header only after the user clicks.
- **Test.** The integration test `limited-views.test.ts` renders the real page and checks that exactly one view is consumed, after the click.

#### 1.7 Medium: existence oracle in the burn gate
- **Cause.** `PeekMaxAccesses` ignored visibility. For a limited paste the caller could not see (or a private paste requested on the public route) it answered 403, while a missing paste got 404. This reopened the oracle that `fd0ad3d` set out to close.
- **Fix.** The peek applies the same visibility as the read: public-only on the public route, `canAccess` on the authenticated routes. Anything else is a 404.
- **Test.** `TestPeekMaxAccesses`, which now covers foreign-user and private-on-public cases.

#### 1.8 Medium: user search revealed the rosters of other teams
- **Cause.** `/users/search?team=X` excluded X's members from the results for anyone. Any user could learn who belongs to any team by checking which names disappear.
- **Fix.** The filter only applies to teams the caller owns or has joined. It now also excludes the team owner, who previously appeared as an invitable result.
- **Test.** `TestUserSearchTeamFilterRequiresMembership`.

#### 1.9 Medium: transferring a paste leaked personal label names
- **Cause.** Personal labels stayed attached to a personal paste after a transfer, so the new owner saw the previous owner's private label names.
- **Fix.** The transfer drops the previous owner's personal labels and the new owner's own share row.
- **Test.** `TestTransferDropsPreviousOwnersLabels`.

#### 1.10 Medium: login timing revealed which accounts exist
- **Cause.** For unknown usernames, `Authenticate` generated a bcrypt hash *and* compared against it. That took about twice as long as a wrong password, which is the opposite of what the comment intended.
- **Fix.** The dummy hash is computed once (`sync.OnceValue`) at the real cost, and each request does only one comparison.
- **Test.** `TestDummyPasswordHashIsComputedOnceAtRealCost`.

#### 1.11 Medium: the login rate limit could be exceeded in parallel
- **Cause.** The limiter checked the budget (`allow`) and recorded failures (`recordFailure`) separately. Parallel attempts all passed the check before any failure was recorded.
- **Fix.** Attempts are reserved atomically (`reserve`), and a successful login clears the bucket.
- **Test.** `TestLoginRateLimiterReserveIsAtomic`, which runs 50 concurrent attempts and admits exactly 10.

#### 1.12 Medium: login rate limiting behind a reverse proxy (follow-up commit)
- **Cause.** The limiter keys failures by (client IP, username) and deliberately ignores `X-Forwarded-For`, because clients can forge it. Behind a proxy every client therefore shared the proxy's address. Anyone could lock any account out for 15 minutes with 10 bad passwords.
- **Fix.** New optional setting `apiserver.trusted_proxies` (IPs or CIDRs; snap key `trusted-proxies`). For requests arriving from a trusted proxy, the client is the rightmost `X-Forwarded-For` hop that is not itself a trusted proxy. Hops further left are client-supplied and never used, and a hop that doesn't parse stops the walk at the last trusted address. For any other peer the header is still ignored. The setting is validated at startup and documented in `README.md`.
- **Tests.**
  - `TestClientIPBehindTrustedProxies`: forged hops, proxy chains, ports, IPv6, garbage entries.
  - `TestLoginRateLimitPerClientBehindProxy`: an attacker behind the proxy no longer locks out the victim.
  - `TestAPIServer_TrustedProxyNets`.

#### 1.13 Medium: plain admins could take over other admin accounts (follow-up commit)
- **Cause.** Deleting an admin required the superuser, but a plain admin could reset another admin's password or email, rename them, or disable them.
- **Fix.** Modifying another administrator (update, enable, disable) now requires the superuser, the same as deletion. Admins still manage regular users and their own account. The guard checks both `is_admin` and `is_superuser`, so the superuser stays protected even with the admin flag cleared.
- **UI.**
  - The auth store now carries `isSuperUser` from the token.
  - The admin pages hide the actions the server refuses.
  - The edit page sends only changed fields. Before, it always sent `is_admin`, which the server rejects from anyone but the superuser, so plain admins could not save *any* user. It also re-sent unchanged name, email and enabled values, which signed the user out of every session.
- **Test.** `TestAdminCannotModifyOtherAdmins`.

### 2. MySQL: broken features

The project supports MySQL, but its SQL had only ever run on SQLite. Against MySQL 8.4:

| Where | Problem | Fix |
|---|---|---|
| User search (invite type-ahead), LIKE paste search | `ESCAPE '\'`: in MySQL a backslash escapes the closing quote, so the query is a **syntax error** (1064) | Shared `util.EscapeLike` / `util.LikeEscape` using `ESCAPE '!'`, valid on both backends |
| Label settings list (`GET /labels/mine`) | `COUNT(...) AS usage`: `USAGE` is a reserved word, a **syntax error** | Aliased as `usage_count` |
| Label rename (merge lookup) | `owner_user_id IS ?` with a value is a **syntax error** in MySQL | Explicit per-scope condition (`labelScope`) |
| Paste search | The FULLTEXT index can never be created (`Column 'data' cannot be part of FULLTEXT index`: longblob). That branch was dead, logged an error on every start and cost an `information_schema` query per search. The LIKE fallback matched the whole query as one phrase, case-sensitively on content. | FULLTEXT code removed. Both backends split the query into terms that must all match (see 5.1). MySQL compares `CONVERT(data USING utf8mb4)`, so matching ignores case. |
| Preference-only user update | `Save` of an unchanged row reports 0 rows affected on MySQL and falls back to an upsert | Written with `UpdateColumn("discoverable")` |

I confirmed on MySQL that the syntax errors exist in the original code and that the fixes work. `internal/testdb` gives each test a fresh SQLite file, or a fresh MySQL database when `GOPHERBIN_TEST_MYSQL=user:pass@host:port` is set. CI has a new `Test (MySQL)` job.

### 3. Data integrity

- **Team deletion.** It deleted the team's pastes *before* deleting the join rows it selected through `pastes WHERE team_id = ?`, so those rows were never removed on databases without FK cascades. It also left the team's labels behind. Everything is now deleted children-first, labels included (`TestTeamDeleteRemovesTeamLabels`).
- **User deletion.** Deleting a user with a pending team-ownership offer failed with an FK error, because `teams.transfer_to_user_id` has no cascade. It also left the user's personal labels orphaned. Both are now handled in one transaction (`TestDeleteUserWithPendingTransferOffer`).
- **Paste creation.** The paste was created first and its labels attached in a separate transaction; a failure left a half-created paste. Both now happen in one transaction.
- **Team names** were not validated at all. `/` breaks routing, because mux decodes `%2F`. `invites` and `transfers` are shadowed by fixed routes. `.`/`..` break paths, and MySQL could truncate long names. Names are now 1–32 letters, digits, spaces and `._-`, starting with a letter or digit, and not a reserved name (`TestTeamNameValidation`). Existing teams are unaffected; the rule applies to creation and renaming.

### 4. API spec, contract test and tooling

- **The contract test's route coverage tested nothing.** Its regex never matched `routers.go`, so an undocumented route passed. The served operations now come from `router.Walk` and are checked against the spec in both directions.
- **Array responses were never schema-checked.** The check expected `x-go-type` on items, but items are `$ref`s. Element keys are now validated against the item schema and its `required` list.
- **The two mutation tests now fail as they should:** an undocumented route, and an undocumented field on `TeamInviteInfo`.
- **Dead code and mis-attributed failures.** `runSpecTypes`, `specShape`, `specGoTypes` and the stale gap lists are removed. Helpers now report failures to the subtest that called them. Body-less operations must actually return an empty body.
- **Spec fixes.**
  - `POST /first-run` (the documented form) returned 401 without a trailing slash; both forms are now served.
  - The transfer-action enum was a single value, `"accept decline cancel"`, and it had leaked into the TypeScript types.
  - Decline and cancel now return the team, as documented.
  - The 403 response and the `X-Consume-Access` header parameter are documented.
  - The missing 409s are documented; admin 403s that are never returned were dropped.
  - `security: []` now applies to the public operations, via an `x-public` extension turned into `security: []` by apigen, because go-swagger annotations cannot express it.
  - `POST /paste` now takes a dedicated `NewPasteParams`. It used to take the response model, which marked server-assigned fields as required and `data` as optional.
- **Reproducible generation.** `make generate` (go-swagger v0.31.0, then apigen, then validate) and `npm run gen:api` (`swagger2openapi@7.0.8`, `openapi-typescript@7.13.0`) reproduce the committed files byte for byte. CI checks they are current and now runs the `webui`-tagged CSP tests.
- **apigen.** Dead code removed. It now exits non-zero on an unknown alias instead of silently skipping it, and skips unexported fields.
- **Migration test.** The "FK suspension is restored" test never ran `migrateDB`. It now drives a real success and a real failure (`TestMigrateDBRestoresConnectionState`). Removing the restore makes it fail; before, that made the whole package hang until timeout.
- **Weaker assertions tightened.**
  - A rule that was only `t.Logf`'d is now asserted.
  - The "non-owner member cannot delete" case used an outsider; it now uses a real member.
  - The duplicate-rename, TTL and colour checks now assert the error type.
  - "Admin cannot invite an admin" now targets a fresh user.
- **CI and gofmt.** `gofmt` failed CI's `fmt-check` at `fd0ad3d` (two files).
- **CORS.** Configured `cors_origins` may now send `X-Consume-Access`; without it, a separately hosted frontend could never open a limited paste.

### 5. Other functional bugs

#### 5.1 Search (both backends)
- **SQLite.** Hyphenated searches found nothing: the sanitizer *deleted* FTS syntax characters, so `my-file` became the word `myfile`.
- **Fix.** A shared `searchTerms` now *splits* on those characters (`-`, `"`, `(`, `)`, `:`, `*`, `^`, `{`, `}`, `+`), as the tokenizer does for the stored content. The same terms drive the SQLite FTS phrases and the MySQL per-term LIKE, and the search tests run unchanged on both backends (`TestSearch_PunctuatedAndMixedCaseTerms`).

#### 5.2 Web UI
- **Team page.**
  - It could reload forever when the server spelled the team name differently from the URL (MySQL collations). The page now tracks the name it last loaded.
  - Team names in links were not URL-encoded.
  - One Backspace in the label input removed an in-use team label from every team paste with no confirmation; it now asks first, listing how many pastes are affected.
  - Copy that misstated who can manage members, who sent an invite, or the viewer/admin role was corrected.
- **Paste list.**
  - Team owners could not manage members' team pastes.
  - Share was offered on team pastes, where the server always rejects it; the modal now has a transfer-only mode.
  - The team filter listed only teams that had labels.
  - The label editor suggested labels from other scopes, which would create them in the wrong vocabulary.
  - A slow earlier response could overwrite the current tab.
- **Invite type-ahead.**
  - Late responses reopened the dropdown with stale results.
  - Enter picked the first suggestion instead of the typed name, which could invite the wrong person.
  - Its timer outlived the component.
- **Create page.** It offered pending-invitation and viewer teams, which the server rejects.
- **Admin page.** It offered a self password reset the server rejects (it needs the current password); it now links to Settings.
- **Invite bell.**
  - The header mounted it twice (desktop and mobile copies), doubling its polling. The shared header controls are now rendered once; the integration test counts invite requests on mount.
  - After a 401 the bell kept stale entries on screen; it now clears them.
- **`LabelInput`.** The suggestion dropdown used the previous keystroke's matches.
- **Test harness.**
  - A fatal bootstrap error was swallowed by a `catch {}` and retried for 60 seconds.
  - Two integration tests had assertions that were always true.
  - jsdom lacks `matchMedia`; a stub is now provided.

### 6. Smaller fixes

- `UpdateLabel`: when a rename merged two labels, the requested colour was applied to the label being deleted.
- `UpdateUserHandler` now maps body-size errors to 413 like the other handlers. `DeleteUserHandler` parses the ID as unsigned.
- The web UI CSP comment claimed `'unsafe-inline'` was kept alongside the hashes; it is not.
- The `Teams.MyRole` documentation listed values the API never returns.

---

## Behaviour changes worth noting in release notes

- **Former team members** lose access to all of the team's pastes, including the ones they wrote.
- **Viewers** cannot manage team labels and cannot be given team pastes.
- **Previews:** pastes with limited views show no preview to anyone but their owner.
- **Opening limited-view pastes:** the web UI asks before opening one and using up a view.
- **Team names** must be 1–32 letters, digits, spaces or `._-`, and cannot be `invites` or `transfers`.
- **Admin accounts:** only the superuser may modify or disable other administrator accounts.
- **New `trusted_proxies` setting.** Deployments behind a reverse proxy should set it; until then, all users share the proxy's rate-limit budget.
- **MySQL search** now requires every word and matches case-insensitively. On both backends, query-syntax characters (`-`, `"`, `(`, `)`, `:`, `*`, `^`, `{`, `}`, `+`) separate words instead of being deleted.
- **API changes:**
  - Transfer decline/cancel now return the team.
  - `POST /api/v1/first-run` works without a trailing slash.
  - `POST /paste` is documented with `NewPasteParams` (wire-compatible: same JSON names, and labels still accept strings or objects).

## Tests added

**Go:**
- `TestMutationsDoNotRewriteAssociations`
- `TestFormerMemberLosesTeamPasteAccess`
- `TestTransferOfferWithdrawnWhenTargetDeparts`
- `TestDeleteUserWithPendingTransferOffer`
- `TestTeamNameValidation`
- `TestTeamDeleteRemovesTeamLabels`
- `TestTeamLabelsReadOnlyForViewers`
- `TestLabelRenameMergeKeepsRequestedColor`
- `TestTransferDropsPreviousOwnersLabels`
- `TestUserSearchTeamFilterRequiresMembership`
- `TestListPreviewWithheldForLimitedPastesOfOthers`
- `TestSearch_PunctuatedAndMixedCaseTerms`
- `TestMigrateDBRestoresConnectionState`
- `TestDummyPasswordHashIsComputedOnceAtRealCost`
- `TestLoginRateLimiterReserveIsAtomic`
- `TestClientIPBehindTrustedProxies`
- `TestLoginRateLimitPerClientBehindProxy`
- `TestAPIServer_TrustedProxyNets`
- `TestAdminCannotModifyOtherAdmins`
- Extended: `TestPeekMaxAccesses` and the contract test (router walk, array schemas, transfer decline/cancel).

**Web integration:**
- `limited-views.test.ts` (2 tests)
- "is rendered once by the header, so it polls once" in `invite-bell.test.ts`

## Checked and found correct

- **Access-counter atomicity:** the conditional `UPDATE` plus retry-on-busy design. The concurrency tests passed repeatedly and `-race` is clean.
- **Existence hiding:** foreign pastes, teams and labels answer 404 instead of 401 (apart from the gate oracle above).
- **Roster redaction** for pending invitees.
- **JWT invalidation:** tokens are invalidated when sensitive fields change and survive preference-only changes.
- **Atomic first run:** superuser creation is safe against concurrent first-run requests.
- **Request limits and server hardening:**
  - the request-body cap and the 413 mapping;
  - server timeouts;
  - CORS failing closed;
  - the security headers;
  - the hash-based CSP (the inline bootstrap script hash matches the real bundle).
- **Download names:** `Content-Disposition` handling is sanitised correctly.
- **`/p` and `/public/p` routing:** order of routes and trailing-slash variants.
- **Frontend XSS:** no `{@html}` or `innerHTML`. Custom label colours only reach `style=`, behind a strict `#rrggbb` check.
- **URL encoding** in `teams.ts`, and query strings built with `URLSearchParams`.
- **Dependencies:** real, correctly pinned packages; test libraries in `devDependencies`.
- **Generated spec:** current and deterministic.

## Remaining items (not changed)

- **The new CI jobs have not run on GitHub yet:** the MySQL job, the generated-file check and the embedded-UI test. actionlint passes and each command was run locally, but the first real run is still to come.
- **Test fixtures never close their database pools.** It's harmless on SQLite. With MySQL, a full run needs `max_connections` raised, which the CI job does. Fixing it cleanly would require `Close` methods on the managers.
- **Search can still probe limited-view pastes.** A user allowed to open a limited-view paste can still find out, through search, whether words occur in it without using up a view. The intended recipient is by definition allowed to read it, so I left this as a design question.
- **Mutation responses return content without consuming a view.** `SetPrivacy`, `TransferOwnership` and `SetLabels` return the paste's full content. For a team paste, that hands the *team owner* the content of a member's limited-view paste without using up a view. Everyone else who can call these is the paste's owner.
- **Pre-existing UI leak.** `CodeEditor` and `PastePreview` create the editor after an async language load even if the component was destroyed meanwhile. This predates the reviewed commits.
- **Process-local rate limiting.** The limiter is per process, as documented; several instances multiply the budget.
- **Minor:**
  - `freePort` in the test harness has a small time-of-check/time-of-use race.
  - apigen resolves embedded-field name clashes by first occurrence rather than `encoding/json`'s depth rule; no type in the API is affected.
