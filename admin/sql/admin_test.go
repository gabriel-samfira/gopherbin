package sql_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	adminCommon "gopherbin/admin/common"
	adminSQL "gopherbin/admin/sql"
	"gopherbin/auth"
	"gopherbin/config"
	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"
	pasteSQL "gopherbin/paste/sql"
	"gopherbin/util"

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

// newAdminFixture creates a fresh DB (migrations via NewPaster), a UserManager,
// a superuser, and returns the manager plus a superuser-authenticated context.
func newAdminFixture(t *testing.T) (adminCommon.UserManager, context.Context) {
	t.Helper()
	dbCfg := testDBConfig(t)

	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("migrations: %v", err)
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
	return mgr, ctx
}

func isUnauthorized(err error) bool {
	_, ok := pkgErrors.Cause(err).(*gErrors.UnauthorizedError)
	return ok
}

func isDuplicate(err error) bool {
	_, ok := pkgErrors.Cause(err).(*gErrors.DuplicateUserError)
	return ok
}

// ── HasSuperUser ─────────────────────────────────────────────────────────────

func TestHasSuperUser_FalseOnFreshDB(t *testing.T) {
	dbCfg := testDBConfig(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	mgr, err := adminSQL.NewUserManager(dbCfg)
	if err != nil {
		t.Fatalf("NewUserManager: %v", err)
	}
	if mgr.HasSuperUser() {
		t.Fatal("expected false on fresh DB")
	}
}

func TestHasSuperUser_TrueAfterCreation(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	if !mgr.HasSuperUser() {
		t.Fatal("expected true after CreateSuperUser")
	}
}

// ── CreateSuperUser ──────────────────────────────────────────────────────────

func TestCreateSuperUser_FailsIfAlreadyExists(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	_, err := mgr.CreateSuperUser(params.NewUserParams{
		Email:    "second@example.com",
		Username: "second",
		FullName: "Second",
		Password: testPassword,
	})
	if err == nil {
		t.Fatal("expected error when superuser already exists")
	}
}

// ── Authenticate ─────────────────────────────────────────────────────────────

func TestAuthenticate_ValidCredentials(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	ctx, err := mgr.Authenticate(context.Background(), params.PasswordLoginParams{
		Username: "super@example.com",
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if auth.UserID(ctx) == 0 {
		t.Fatal("expected non-zero UserID after successful auth")
	}
}

func TestAuthenticate_WrongPassword(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	_, err := mgr.Authenticate(context.Background(), params.PasswordLoginParams{
		Username: "super@example.com",
		Password: "wrong-password",
	})
	if !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError, got %v", err)
	}
}

func TestAuthenticate_EmptyCredentials(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	_, err := mgr.Authenticate(context.Background(), params.PasswordLoginParams{})
	if !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError for empty credentials, got %v", err)
	}
}

func TestAuthenticate_DisabledUser(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)

	// Create a regular (disabled) user.
	user, err := mgr.Create(superCtx, params.NewUserParams{
		Email:    "user@example.com",
		Username: "regularuser",
		FullName: "Regular User",
		Password: testPassword,
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = mgr.Authenticate(context.Background(), params.PasswordLoginParams{
		Username: user.Email,
		Password: testPassword,
	})
	if !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError for disabled user, got %v", err)
	}
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestUserCreate_RequiresAdmin(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	_, err := mgr.Create(context.Background(), params.NewUserParams{
		Email:    "x@example.com",
		Username: "xuser",
		FullName: "X User",
		Password: testPassword,
		Enabled:  true,
	})
	if !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError without admin ctx, got %v", err)
	}
}

func TestUserCreate_DuplicateEmailRejected(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	p := params.NewUserParams{
		Email:    "dup@example.com",
		Username: "dupuser",
		FullName: "Dup User",
		Password: testPassword,
		Enabled:  true,
	}
	if _, err := mgr.Create(superCtx, p); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	p.Username = "dupuser2"
	_, err := mgr.Create(superCtx, p)
	if !isDuplicate(err) {
		t.Fatalf("expected DuplicateUserError, got %v", err)
	}
}

func TestUserCreate_Success(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	u, err := mgr.Create(superCtx, params.NewUserParams{
		Email:    "new@example.com",
		Username: "newuser",
		FullName: "New User",
		Password: testPassword,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.Email != "new@example.com" {
		t.Errorf("Email: want new@example.com, got %s", u.Email)
	}
}

// ── Get ──────────────────────────────────────────────────────────────────────

func TestUserGet_UserCanViewSelf(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	u, err := mgr.Create(superCtx, params.NewUserParams{
		Email:    "self@example.com",
		Username: "selfuser",
		FullName: "Self User",
		Password: testPassword,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	selfCtx := auth.PopulateContext(context.Background(), u)
	got, err := mgr.Get(selfCtx, u.ID)
	if err != nil {
		t.Fatalf("Get self: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("Get returned wrong user: want %d, got %d", u.ID, got.ID)
	}
}

func TestUserGet_NonAdminCannotViewOthers(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	u1, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "u1@example.com", Username: "u1user", FullName: "U1", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create u1: %v", err)
	}
	u2, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "u2@example.com", Username: "u2user", FullName: "U2", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create u2: %v", err)
	}

	u1Ctx := auth.PopulateContext(context.Background(), u1)
	_, err = mgr.Get(u1Ctx, u2.ID)
	if !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError, got %v", err)
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestUserList_RequiresAdmin(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	_, err := mgr.List(context.Background(), 1, 10)
	if !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError, got %v", err)
	}
}

func TestUserList_ReturnsUsers(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	res, err := mgr.List(superCtx, 1, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// At minimum the superuser exists.
	if len(res.Users) == 0 {
		t.Fatal("expected at least one user in list")
	}
}

// ── Enable / Disable ─────────────────────────────────────────────────────────

func TestUserEnable_AdminCanToggle(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	u, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "toggle@example.com", Username: "toggleuser", FullName: "Toggle", Password: testPassword, Enabled: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.Enable(superCtx, u.ID); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	got, _ := mgr.Get(superCtx, u.ID)
	if !got.Enabled {
		t.Error("expected Enabled=true after Enable()")
	}

	if err := mgr.Disable(superCtx, u.ID); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	got, _ = mgr.Get(superCtx, u.ID)
	if got.Enabled {
		t.Error("expected Enabled=false after Disable()")
	}
}

func TestUserEnable_RequiresAdmin(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	u, _ := mgr.Create(superCtx, params.NewUserParams{
		Email: "e@example.com", Username: "euser", FullName: "E", Password: testPassword, Enabled: false,
	})
	if err := mgr.Enable(context.Background(), u.ID); !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError, got %v", err)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestUserDelete_AdminCanDeleteRegularUser(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	u, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "del@example.com", Username: "deluser", FullName: "Del", Password: testPassword, Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mgr.Delete(superCtx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestUserDelete_CannotDeleteSelf(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)
	superID := auth.UserID(superCtx)
	err := mgr.Delete(superCtx, superID)
	if err == nil {
		t.Fatal("expected error when deleting own account")
	}
}

func TestUserDelete_CannotDeleteSuperUser(t *testing.T) {
	mgr, superCtx := newAdminFixture(t)

	// Create a second admin to attempt deleting the superuser.
	admin, err := mgr.Create(superCtx, params.NewUserParams{
		Email: "admin2@example.com", Username: "admin2", FullName: "Admin2", Password: testPassword, Enabled: true, IsAdmin: true,
	})
	if err != nil {
		t.Fatalf("Create admin: %v", err)
	}
	adminCtx := auth.PopulateContext(context.Background(), admin)

	superID := auth.UserID(superCtx)
	if err := mgr.Delete(adminCtx, superID); err == nil {
		t.Fatal("expected error when deleting superuser")
	}
}

// ── Token blacklist ───────────────────────────────────────────────────────────

func TestValidateToken_NotBlacklisted(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	if err := mgr.ValidateToken("token-abc"); err != nil {
		t.Fatalf("expected nil for non-blacklisted token, got %v", err)
	}
}

func TestValidateToken_EmptyTokenID(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	if err := mgr.ValidateToken(""); !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError for empty token, got %v", err)
	}
}

func TestBlacklistAndValidateToken(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	tokenID := "tok-xyz"
	expiry := time.Now().Add(time.Hour).Unix()

	if err := mgr.BlacklistToken(tokenID, expiry); err != nil {
		t.Fatalf("BlacklistToken: %v", err)
	}
	if err := mgr.ValidateToken(tokenID); !isUnauthorized(err) {
		t.Fatalf("expected UnauthorizedError for blacklisted token, got %v", err)
	}
}

func TestCleanTokens_RemovesExpired(t *testing.T) {
	mgr, _ := newAdminFixture(t)
	tokenID := "tok-expired"
	expiry := time.Now().Add(-time.Hour).Unix() // already expired

	if err := mgr.BlacklistToken(tokenID, expiry); err != nil {
		t.Fatalf("BlacklistToken: %v", err)
	}
	if err := mgr.CleanTokens(); err != nil {
		t.Fatalf("CleanTokens: %v", err)
	}
	// Token should have been pruned; ValidateToken should return nil.
	if err := mgr.ValidateToken(tokenID); err != nil {
		t.Fatalf("expected nil after CleanTokens removed expired token, got %v", err)
	}
}

// ── Superuser account protection ─────────────────────────────────────────────

func TestAdminCannotTakeOverSuperUser(t *testing.T) {
	mgr, ctxSuper := newAdminFixture(t)

	admin, err := mgr.Create(ctxSuper, params.NewUserParams{
		Email:    "admin@example.com",
		Username: "plainadmin",
		FullName: "Plain Admin",
		Password: testPassword,
		IsAdmin:  true,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create admin: %v", err)
	}
	ctxAdmin := auth.PopulateContext(context.Background(), admin)

	// The superuser is user ID 1 in the fresh fixture.
	newPassword := "Another-Correct-Horse-Staple-G0pherbin-2026!"
	if _, err := mgr.Update(ctxAdmin, 1, params.UpdateUserPayload{
		Password: &newPassword,
	}); !isUnauthorized(err) {
		t.Fatalf("admin resetting superuser password: want Unauthorized, got %v", err)
	}
	if err := mgr.Disable(ctxAdmin, 1); !isUnauthorized(err) {
		t.Fatalf("admin disabling superuser: want Unauthorized, got %v", err)
	}
	// The superuser can still manage themselves.
	if _, err := mgr.Update(ctxSuper, 1, params.UpdateUserPayload{
		FullName: ptr("Super Admin Renamed"),
	}); err != nil {
		t.Fatalf("superuser self-update: %v", err)
	}
}

func ptr(s string) *string { return &s }

func isConflict(err error) bool {
	_, ok := pkgErrors.Cause(err).(*gErrors.ConflictError)
	return ok
}

func TestUserDelete_OwnerOfTeamsCannotBeDeleted(t *testing.T) {
	dbCfg := testDBConfig(t)
	if _, err := pasteSQL.NewPaster(dbCfg); err != nil {
		t.Fatalf("migrations: %v", err)
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

	u, err := mgr.Create(ctx, params.NewUserParams{
		Email:    "teamlead@example.com",
		Username: "teamlead",
		FullName: "Team Lead",
		Password: testPassword,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	db, err := util.NewDBConn(dbCfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Create(&models.Teams{OwnerID: u.ID, Name: "owned"}).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}

	if err := mgr.Delete(ctx, u.ID); !isConflict(err) {
		t.Fatalf("delete team owner: want Conflict, got %v", err)
	}

	// Once the team is gone, deletion succeeds.
	if err := db.Where("name = ?", "owned").Delete(&models.Teams{}).Error; err != nil {
		t.Fatalf("delete team: %v", err)
	}
	if err := mgr.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete after team removal: %v", err)
	}
}
