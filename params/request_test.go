package params_test

import (
	"testing"

	"gopherbin/params"
)

const strongPassword = "Correct-Horse-Battery-Staple-G0pherbin-2024!"

func TestNewUserParams_Validate_Valid(t *testing.T) {
	p := params.NewUserParams{
		Email:    "user@example.com",
		Username: "testuser",
		FullName: "Test User",
		Password: strongPassword,
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestNewUserParams_Validate_WeakPassword(t *testing.T) {
	p := params.NewUserParams{
		Email:    "user@example.com",
		Username: "testuser",
		FullName: "Test User",
		Password: "password",
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for weak password")
	}
}

func TestNewUserParams_Validate_InvalidEmail(t *testing.T) {
	p := params.NewUserParams{
		Email:    "not-an-email",
		Username: "testuser",
		FullName: "Test User",
		Password: strongPassword,
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for invalid email")
	}
}

func TestNewUserParams_Validate_NonAlphanumericUsername(t *testing.T) {
	p := params.NewUserParams{
		Email:    "user@example.com",
		Username: "user-name",
		FullName: "Test User",
		Password: strongPassword,
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for non-alphanumeric username")
	}
}

func TestNewUserParams_Validate_EmptyFullName(t *testing.T) {
	p := params.NewUserParams{
		Email:    "user@example.com",
		Username: "testuser",
		FullName: "",
		Password: strongPassword,
	}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for empty full name")
	}
}

func TestPasswordLoginParams_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		p := params.PasswordLoginParams{Username: "user", Password: "pass"}
		if err := p.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})
	t.Run("empty_username", func(t *testing.T) {
		p := params.PasswordLoginParams{Password: "pass"}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for empty username")
		}
	})
	t.Run("empty_password", func(t *testing.T) {
		p := params.PasswordLoginParams{Username: "user"}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for empty password")
		}
	})
}
