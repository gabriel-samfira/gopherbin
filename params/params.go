package params

import "time"

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

// UpdateUserPayload defines fields that may be updated
// on a user entry
type UpdateUserPayload struct {
	IsAdmin  *bool   `json:"is_admin,omitempty"`
	Password *string `json:"password,omitempty"`
	FullName *string `json:"full_name,omitempty"`
}

// Paste holds information about a paste
type Paste struct {
	ID        string    `json:"id"`
	Data      string    `json:"data"`
	Name      string    `json:"name"`
	Expires   time.Time `json:"expires"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"created_at"`
}
