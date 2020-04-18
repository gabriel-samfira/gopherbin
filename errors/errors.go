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
	ErrUnauthorized = fmt.Errorf("Unauthorized")
	// ErrNotFound is returned if an object is not found in
	// the database.
	ErrNotFound = fmt.Errorf("not found")
	// ErrInvalidSession is returned when a session is invalid
	ErrInvalidSession = fmt.Errorf("invalid session")
	// ErrDuplicateUser is returned when creating a user, if the
	// user already exists.
	ErrDuplicateUser = fmt.Errorf("duplicate user")
	// ErrBadRequest is returned is a malformed request is sent
	ErrBadRequest = fmt.Errorf("invalid request")
)
