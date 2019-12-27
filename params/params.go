package params

import (
	"fmt"
	"regexp"
	"time"

	"gopherbin/util"

	zxcvbn "github.com/nbutton23/zxcvbn-go"
)

// From: https://www.alexedwards.net/blog/validation-snippets-for-go#email-validation
var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

// Teams holds information about a team
type Teams struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Owner   Users   `json:"owner"`
	Members []Users `json:"members"`
}

// Users holds information about a particular user
type Users struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	FullName    string    `json:"full_name"`
	Password    string    `json:"password"`
	Enabled     bool      `json:"enabled"`
	IsAdmin     bool      `json:"is_admin"`
	IsSuperUser bool      `json:"is_superuser"`
}

// NewUserParams holds the needed information to create
// a new user
type NewUserParams struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
}

// Validate validates the object in order to determine
// if the minimum required fields have proper values (email
// is valid, password is of a decent strength etc).
func (u NewUserParams) Validate() error {
	passwordStenght := zxcvbn.PasswordStrength(u.Password, nil)
	if passwordStenght.Score < 4 {
		return fmt.Errorf("the password is too weak, please use a stronger password")
	}
	if len(u.Email) > 254 || !rxEmail.MatchString(u.Email) {
		return fmt.Errorf("invalid email address %s", u.Email)
	}

	if len(u.FullName) == 0 {
		return fmt.Errorf("full name may not be empty")
	}
	return nil
}

// UpdateUserPayload defines fields that may be updated
// on a user entry
type UpdateUserPayload struct {
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Password *string `json:"password,omitempty"`
	FullName *string `json:"full_name,omitempty"`
}

// Paste holds information about a paste
type Paste struct {
	ID        int64     `json:"id"`
	PasteID   string    `json:"paste_id"`
	Data      string    `json:"data"`
	Language  string    `json:"language"`
	Name      string    `json:"name"`
	Expires   time.Time `json:"expires"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"created_at"`
}

// PasteListResult holds results for a paste list request
type PasteListResult struct {
	Total  int64   `json:"total"`
	Page   int64   `json:"page"`
	Pastes []Paste `json:"pastes"`
}

// PasswordLoginParams holds information used during
// password authentication, that will be passed to a
// password login function
type PasswordLoginParams struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ID returns a xxhash (int64) of the username
func (p PasswordLoginParams) ID() int64 {
	if p.Username == "" {
		return 0
	}
	userID, err := util.HashString(p.Username)
	if err != nil {
		return 0
	}
	return int64(userID)
}
