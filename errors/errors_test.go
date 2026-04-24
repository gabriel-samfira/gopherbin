package errors_test

import (
	"testing"

	gErrors "gopherbin/errors"
)

func TestNewUnauthorizedError(t *testing.T) {
	err := gErrors.NewUnauthorizedError("not allowed")
	if err.Error() != "not allowed" {
		t.Errorf("want %q, got %q", "not allowed", err.Error())
	}
	if _, ok := err.(*gErrors.UnauthorizedError); !ok {
		t.Error("expected *UnauthorizedError")
	}
}

func TestNewNotFoundError(t *testing.T) {
	err := gErrors.NewNotFoundError("missing")
	if err.Error() != "missing" {
		t.Errorf("want %q, got %q", "missing", err.Error())
	}
	if _, ok := err.(*gErrors.NotFoundError); !ok {
		t.Error("expected *NotFoundError")
	}
}

func TestNewDuplicateUserError(t *testing.T) {
	err := gErrors.NewDuplicateUserError("already exists")
	if err.Error() != "already exists" {
		t.Errorf("want %q, got %q", "already exists", err.Error())
	}
	if _, ok := err.(*gErrors.DuplicateUserError); !ok {
		t.Error("expected *DuplicateUserError")
	}
}

func TestNewBadRequestError(t *testing.T) {
	err := gErrors.NewBadRequestError("field %q is required", "email")
	want := `field "email" is required`
	if err.Error() != want {
		t.Errorf("want %q, got %q", want, err.Error())
	}
	if _, ok := err.(*gErrors.BadRequestError); !ok {
		t.Error("expected *BadRequestError")
	}
}

func TestNewConflictError(t *testing.T) {
	err := gErrors.NewConflictError("resource %s conflicts", "foo")
	want := "resource foo conflicts"
	if err.Error() != want {
		t.Errorf("want %q, got %q", want, err.Error())
	}
	if _, ok := err.(*gErrors.ConflictError); !ok {
		t.Error("expected *ConflictError")
	}
}

func TestSentinelVars(t *testing.T) {
	if _, ok := gErrors.ErrUnauthorized.(*gErrors.UnauthorizedError); !ok {
		t.Error("ErrUnauthorized: expected *UnauthorizedError")
	}
	if _, ok := gErrors.ErrNotFound.(*gErrors.NotFoundError); !ok {
		t.Error("ErrNotFound: expected *NotFoundError")
	}
	if _, ok := gErrors.ErrDuplicateEntity.(*gErrors.DuplicateUserError); !ok {
		t.Error("ErrDuplicateEntity: expected *DuplicateUserError")
	}
	if _, ok := gErrors.ErrBadRequest.(*gErrors.BadRequestError); !ok {
		t.Error("ErrBadRequest: expected *BadRequestError")
	}
}
