package sql_test

import (
	"context"
	"path/filepath"
	"sync"
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
	p, err := paster.Create(ctx, []byte("paste content"), title, "text", "", nil, public, "", nil, maxAccesses, nil)
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

// ── Search: FTS5 expression sanitization ─────────────────────────────────────

// newSearchFixture creates a DB with two ordinary users (alice, bob) and
// returns a Paster plus an authenticated context for each.
func newSearchFixture(t *testing.T) (pasteCommon.Paster, context.Context, context.Context) {
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
	superCtx := auth.PopulateContext(context.Background(), super)

	alice, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "alice@example.com", Username: "alice", FullName: "Alice", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create alice: %v", err)
	}
	bob, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "bob@example.com", Username: "bob", FullName: "Bob", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create bob: %v", err)
	}
	return paster,
		auth.PopulateContext(context.Background(), alice),
		auth.PopulateContext(context.Background(), bob)
}

// The search string used to be bound verbatim as an FTS5 MATCH expression,
// so quotes, column filters, NEAR and wildcards reached the FTS query
// parser: unbalanced input aborted the statement (HTTP 500) and column
// filters could reference other columns. All of it must now be neutralized
// while ordinary text search keeps working.
func TestSearch_FTSExpressionInjectionIsNeutralized(t *testing.T) {
	paster, aliceCtx, bobCtx := newSearchFixture(t)

	body := "hello world content foo unbalanced near x 4"
	alicePaste, err := paster.Create(aliceCtx, []byte(body), "reactor notes", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create alice: %v", err)
	}
	bobPaste, err := paster.Create(bobCtx, []byte(body), "secret notes", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create bob: %v", err)
	}

	hostile := []string{`content:foo`, `"unbalanced`, `*`, `NEAR/4 x`, `hello "world`}
	for _, q := range hostile {
		res, err := paster.Search(aliceCtx, q, 1, 50, pasteCommon.ScopeAll, nil, "")
		if err != nil {
			t.Errorf("Search(%q): want no error, got %v", q, err)
			continue
		}
		for _, got := range res.Pastes {
			if got.PasteID == bobPaste.PasteID {
				t.Errorf("Search(%q): foreign paste leaked into results", q)
			}
			if got.PasteID != alicePaste.PasteID {
				t.Errorf("Search(%q): unexpected result %q", q, got.PasteID)
			}
		}
	}

	// The plain-text meaning of a hostile-looking query survives:
	// `hello "world` becomes the implicit-AND phrase query and still
	// matches Alice's paste, and Bob's never shows up.
	res, err := paster.Search(aliceCtx, `hello "world`, 1, 50, pasteCommon.ScopeAll, nil, "")
	if err != nil {
		t.Fatalf("Search hello \"world: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != alicePaste.PasteID {
		t.Fatalf("want only alice's paste, got %+v", res.Pastes)
	}
}

func TestSearch_MultiWordImplicitANDStillWorks(t *testing.T) {
	paster, aliceCtx, _ := newSearchFixture(t)

	both, err := paster.Create(aliceCtx, []byte("quantum flux capacitor"), "both.txt", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create both: %v", err)
	}
	alphaOnly, err := paster.Create(aliceCtx, []byte("quantum entanglement"), "alpha.txt", "text", "", nil, false, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create alphaOnly: %v", err)
	}

	// Two words are ANDed: only the paste containing both matches.
	res, err := paster.Search(aliceCtx, "quantum flux", 1, 50, pasteCommon.ScopeAll, nil, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != both.PasteID {
		t.Fatalf("want only both.txt, got %+v", res.Pastes)
	}
	// AND is order-independent.
	res, err = paster.Search(aliceCtx, "flux quantum", 1, 50, pasteCommon.ScopeAll, nil, "")
	if err != nil {
		t.Fatalf("Search reversed: %v", err)
	}
	if len(res.Pastes) != 1 || res.Pastes[0].PasteID != both.PasteID {
		t.Fatalf("want only both.txt for reversed query, got %+v", res.Pastes)
	}
	// A single term still matches everything that contains it.
	res, err = paster.Search(aliceCtx, "quantum", 1, 50, pasteCommon.ScopeAll, nil, "")
	if err != nil {
		t.Fatalf("Search single term: %v", err)
	}
	if len(res.Pastes) != 2 {
		t.Fatalf("want both pastes for 'quantum', got %+v", res.Pastes)
	}
	seen := map[string]bool{}
	for _, got := range res.Pastes {
		seen[got.PasteID] = true
	}
	if !seen[both.PasteID] || !seen[alphaOnly.PasteID] {
		t.Fatalf("want %q and %q, got %+v", both.PasteID, alphaOnly.PasteID, res.Pastes)
	}
}

// ── Access-counter atomicity ─────────────────────────────────────────────────

// Concurrent viewers must never over-serve an access-limited paste: the
// counter is now consumed by a single conditional UPDATE, not a
// read-then-increment that the sqlite driver leaves unlocked (it drops
// SELECT ... FOR UPDATE).

func TestGetPublicPaste_ConcurrentSingleAccessServesExactlyOne(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	p := mustCreate(t, paster, ctx, "race-one-shot", true, pInt(1))

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, workers)
	data := make([]string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			got, err := paster.GetPublicPaste(context.Background(), p.PasteID)
			errs[idx] = err
			if err == nil {
				data[idx] = string(got.Data)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	served := 0
	for i, err := range errs {
		switch {
		case err == nil:
			served++
			if data[i] != "paste content" {
				t.Errorf("worker %d: want content, got %q", i, data[i])
			}
		case !isNotFound(err):
			t.Errorf("worker %d: want content or NotFound, got %v", i, err)
		}
	}
	if served != 1 {
		t.Fatalf("MaxAccesses=1 with %d concurrent viewers: want exactly 1 served, got %d", workers, served)
	}
	// The exhausted paste must be destroyed, not merely over-drawn.
	if _, err := paster.GetPublicPaste(ctx, p.PasteID); !isNotFound(err) {
		t.Fatalf("after concurrent exhaustion: want NotFound, got %v", err)
	}
}

func TestGetPublicPaste_ConcurrentBudgetServesExactlyN(t *testing.T) {
	paster, ctx := newPasterFixture(t)
	const budget = 3
	p := mustCreate(t, paster, ctx, "race-three-shot", true, pInt(budget))

	const workers = 12
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, workers)
	data := make([]string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			got, err := paster.GetPublicPaste(context.Background(), p.PasteID)
			errs[idx] = err
			if err == nil {
				data[idx] = string(got.Data)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	served := 0
	for i, err := range errs {
		switch {
		case err == nil:
			served++
			if data[i] != "paste content" {
				t.Errorf("worker %d: want content, got %q", i, data[i])
			}
		case !isNotFound(err):
			t.Errorf("worker %d: want content or NotFound, got %v", i, err)
		}
	}
	if served != budget {
		t.Fatalf("MaxAccesses=%d with %d concurrent viewers: want exactly %d served, got %d", budget, workers, budget, served)
	}
	if _, err := paster.GetPublicPaste(ctx, p.PasteID); !isNotFound(err) {
		t.Fatalf("after concurrent exhaustion: want NotFound, got %v", err)
	}
}

func TestPeekMaxAccesses(t *testing.T) {
	paster, aliceCtx, _ := newSearchFixture(t)
	limited := mustCreate(t, paster, aliceCtx, "limited", true, pInt(3))
	unlimited := mustCreate(t, paster, aliceCtx, "unlimited", true, nil)

	peeker, ok := paster.(interface {
		PeekMaxAccesses(context.Context, string) (*int, error)
	})
	if !ok {
		t.Fatal("concrete paster does not implement PeekMaxAccesses")
	}
	if got, err := peeker.PeekMaxAccesses(aliceCtx, limited.PasteID); err != nil || got == nil || *got != 3 {
		t.Errorf("limited: got (%v, %v), want (3, nil)", got, err)
	}
	if got, err := peeker.PeekMaxAccesses(aliceCtx, unlimited.PasteID); err != nil || got != nil {
		t.Errorf("unlimited: got (%v, %v), want (nil, nil)", got, err)
	}
	if _, err := peeker.PeekMaxAccesses(aliceCtx, "doesnotexist1234567890ab"); err == nil {
		t.Error("missing paste: want error, got nil")
	}
	// Peeking must not consume budget: three peeks leave all three serves.
	for i := 0; i < 3; i++ {
		if _, err := peeker.PeekMaxAccesses(aliceCtx, limited.PasteID); err != nil {
			t.Fatalf("peek %d: %v", i, err)
		}
	}
	served := 0
	for i := 0; i < 3; i++ {
		if _, err := paster.GetPublicPaste(aliceCtx, limited.PasteID); err == nil {
			served++
		}
	}
	if served != 3 {
		t.Errorf("peeks consumed budget: served %d, want 3", served)
	}
}
