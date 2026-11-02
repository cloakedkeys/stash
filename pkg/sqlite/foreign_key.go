package sqlite

import (
	"errors"

	"github.com/mattn/go-sqlite3"
)

// isForeignKeyError reports whether err is the database refusing a row that
// points at something that doesn't exist.
func isForeignKeyError(err error) bool {
	var se sqlite3.Error
	return errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintForeignKey
}
