package errors

import "fmt"

var (
	// ErrUnauthorized is returned when a user does not have
	// authorization to perform a request
	ErrUnauthorized = fmt.Errorf("Unauthorized")
	// ErrNotFound is returned if an object is not found in
	// the database.
	ErrNotFound = fmt.Errorf("not found")
)
