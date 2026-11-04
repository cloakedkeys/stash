package postgres

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// foreignKeyViolation is PostgreSQL's code for a row that points at something
// that doesn't exist.
const foreignKeyViolation = "23503"

// uniqueViolation is PostgreSQL's code for a row that is already there.
const uniqueViolation = "23505"

// isForeignKeyError reports whether err is the database refusing a row that
// points at something that doesn't exist.
func isForeignKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

// isUniqueError reports whether err is the database refusing a row that is
// already there.
func isUniqueError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// joinColumnKind names what a join table's column points at, as the API's errors
// do: "tag" for tag_id, and for the tag hierarchy's parent_id and child_id.
func joinColumnKind(col interface{}) string {
	kind := strings.TrimSuffix(fmt.Sprint(col), "_id")
	if kind == "parent" || kind == "child" {
		kind = "tag"
	}
	return strings.ReplaceAll(kind, "_", " ")
}

// foreignKeyNotFoundError says which item a join row pointed at, such as
// "tag with id 999 not found", the wording FindMany and the API use. col is the
// join table's column for that item.
func foreignKeyNotFoundError(col interface{}, id int) error {
	return fmt.Errorf("%s with id %d not found", joinColumnKind(col), id)
}

// joinInsertError replaces the database's own error, which shows its command,
// for a join row that points at a missing item or that a list gave twice.
func joinInsertError(err error, col interface{}, id int) error {
	switch {
	case isForeignKeyError(err):
		return foreignKeyNotFoundError(col, id)
	case isUniqueError(err):
		return fmt.Errorf("%s with id %d is listed more than once", joinColumnKind(col), id)
	}
	return err
}
