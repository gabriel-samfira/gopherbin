package auth_test

import (
	"context"
	"testing"
	"time"

	"gopherbin/auth"
	"gopherbin/params"
)

func TestPopulateContext(t *testing.T) {
	user := params.Users{
		ID:          42,
		FullName:    "Test User",
		IsAdmin:     true,
		IsSuperUser: false,
		Enabled:     true,
		UpdatedAt:   time.Now(),
	}
	ctx := auth.PopulateContext(context.Background(), user)

	if auth.UserID(ctx) != 42 {
		t.Errorf("UserID: want 42, got %d", auth.UserID(ctx))
	}
	if !auth.IsAdmin(ctx) {
		t.Error("IsAdmin: want true")
	}
	if auth.IsSuperUser(ctx) {
		t.Error("IsSuperUser: want false")
	}
	if !auth.IsEnabled(ctx) {
		t.Error("IsEnabled: want true")
	}
	if auth.FullName(ctx) != "Test User" {
		t.Errorf("FullName: want %q, got %q", "Test User", auth.FullName(ctx))
	}
	if auth.UpdatedAt(ctx) == "" {
		t.Error("UpdatedAt: want non-empty string")
	}
}

func TestIsAnonymous(t *testing.T) {
	ctx := context.Background()
	if !auth.IsAnonymous(ctx) {
		t.Error("empty context should be anonymous")
	}
	ctx = auth.SetUserID(ctx, 1)
	if auth.IsAnonymous(ctx) {
		t.Error("context with UserID=1 should not be anonymous")
	}
}

func TestGetAdminContext(t *testing.T) {
	ctx := auth.GetAdminContext()
	if !auth.IsAdmin(ctx) {
		t.Error("GetAdminContext: IsAdmin should be true")
	}
	if !auth.IsEnabled(ctx) {
		t.Error("GetAdminContext: IsEnabled should be true")
	}
	if auth.IsSuperUser(ctx) {
		t.Error("GetAdminContext: IsSuperUser should be false")
	}
}

func TestDefaultContextValues(t *testing.T) {
	ctx := context.Background()
	if auth.UserID(ctx) != 0 {
		t.Errorf("default UserID: want 0, got %d", auth.UserID(ctx))
	}
	if auth.IsAdmin(ctx) {
		t.Error("default IsAdmin: want false")
	}
	if auth.IsSuperUser(ctx) {
		t.Error("default IsSuperUser: want false")
	}
	if auth.IsEnabled(ctx) {
		t.Error("default IsEnabled: want false")
	}
	if auth.FullName(ctx) != "" {
		t.Errorf("default FullName: want empty, got %q", auth.FullName(ctx))
	}
	if auth.UpdatedAt(ctx) != "" {
		t.Errorf("default UpdatedAt: want empty, got %q", auth.UpdatedAt(ctx))
	}
}
