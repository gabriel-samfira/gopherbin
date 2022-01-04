// Copyright 2019 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package errors

import "fmt"

var (
	// ErrUnauthorized is returned when a user does not have
	// authorization to perform a request
	ErrUnauthorized = NewUnauthorizedError("Unauthorized")
	// ErrNotFound is returned if an object is not found in
	// the database.
	ErrNotFound = NewNotFoundError("not found")
	// ErrDuplicateUser is returned when creating a user, if the
	// user already exists.
	ErrDuplicateEntity = NewDuplicateUserError("duplicate")
	// ErrBadRequest is returned is a malformed request is sent
	ErrBadRequest = NewBadRequestError("invalid request")
)

type baseError struct {
	msg string
}

func (b *baseError) Error() string {
	return b.msg
}

// NewUnauthorizedError returns a new UnauthorizedError
func NewUnauthorizedError(msg string) error {
	return &UnauthorizedError{
		baseError{
			msg: msg,
		},
	}
}

// UnauthorizedError is returned when a request is unauthorized
type UnauthorizedError struct {
	baseError
}

// NewNotFoundError returns a new NotFoundError
func NewNotFoundError(msg string) error {
	return &NotFoundError{
		baseError{
			msg: msg,
		},
	}
}

// NotFoundError is returned when a resource is not found
type NotFoundError struct {
	baseError
}

// NewDuplicateUserError returns a new DuplicateUserError
func NewDuplicateUserError(msg string) error {
	return &DuplicateUserError{
		baseError{
			msg: msg,
		},
	}
}

// DuplicateUserError is returned when a duplicate user is requested
type DuplicateUserError struct {
	baseError
}

// NewBadRequestError returns a new BadRequestError
func NewBadRequestError(msg string, a ...interface{}) error {
	return &BadRequestError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// BadRequestError is returned when a malformed request is received
type BadRequestError struct {
	baseError
}

// NewConflictError returns a new ConflictError
func NewConflictError(msg string, a ...interface{}) error {
	return &ConflictError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// ConflictError is returned when a conflicting request is made
type ConflictError struct {
	baseError
}
