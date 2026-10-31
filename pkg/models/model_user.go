package models

import "time"

type User struct {
	ID         int
	Username   string
	Notes      string
	Roles      Roles
	ApiKeyHash string
	Locked     bool
	// LibraryLimit is nil if the user can see the whole library.
	LibraryLimit *LibraryLimit
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
