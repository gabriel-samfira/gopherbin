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

package common

import (
	"context"

	"gopherbin/params"
)

// UserManager defines an interface for user management
type UserManager interface {
	Create(ctx context.Context, user params.NewUserParams) (params.Users, error)
	Get(ctx context.Context, userID int64) (params.Users, error)
	Update(ctx context.Context, userID int64, update params.UpdateUserPayload) (params.Users, error)
	List(ctx context.Context, page int64, results int64) (paste params.UserListResult, err error)
	Delete(ctx context.Context, userID int64) error
	Enable(ctx context.Context, userID int64) error
	Disable(ctx context.Context, userID int64) error
	Authenticate(ctx context.Context, info params.PasswordLoginParams) (context.Context, error)
	HasSuperUser() bool
	CreateSuperUser(user params.NewUserParams) (params.Users, error)
	// ValidateToken will check if the token identified by tokenID has been
	// blacklisted. This is needed to handle logouts when using JWT...
	ValidateToken(tokenID string) error
	// BlacklistToken will invalidate a JWT token
	BlacklistToken(tokenID string, expiration int64) error
}
