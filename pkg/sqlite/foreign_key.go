package sqlite

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mattn/go-sqlite3"
)

// isForeignKeyError reports whether err is the database refusing a row that
// points at something that doesn't exist.
func isForeignKeyError(err error) bool {
	var se sqlite3.Error
	return errors.As(err, &se) && se.ExtendedCode == sqlite3.ErrConstraintForeignKey
}

// isUniqueError reports whether err is the database refusing a row that is
// already there.
func isUniqueError(err error) bool {
	var se sqlite3.Error
	return errors.As(err, &se) && (se.ExtendedCode == sqlite3.ErrConstraintUnique || se.ExtendedCode == sqlite3.ErrConstraintPrimaryKey)
}

// isPrimaryKeyError reports whether err is the database refusing a row of table
// whose primary key, the columns cols, is already there. A UNIQUE rule on other
// columns doesn't match. SQLite enforces a primary key of several columns as a
// UNIQUE index, so the message, which names the columns, tells which rule it was.
func isPrimaryKeyError(err error, table string, cols ...string) bool {
	var se sqlite3.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.ExtendedCode {
	case sqlite3.ErrConstraintPrimaryKey:
		return true
	case sqlite3.ErrConstraintUnique:
		named := make([]string, len(cols))
		for i, c := range cols {
			named[i] = table + "." + c
		}
		return strings.HasSuffix(se.Error(), "constraint failed: "+strings.Join(named, ", "))
	}
	return false
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

// groupRelationExistsError replaces the database's own primary key error for a
// containing or sub-group row of group id that is already there. repeated says
// whether the list gave groupID before; otherwise the two were already linked.
// idIsContaining says whether groupID is being added as a sub-group of id.
func groupRelationExistsError(groupID int, id int, idIsContaining bool, repeated bool) error {
	switch {
	case repeated:
		return fmt.Errorf("group with id %d is listed more than once", groupID)
	case idIsContaining:
		return fmt.Errorf("group with id %d is already a sub-group of group with id %d", groupID, id)
	default:
		return fmt.Errorf("group with id %d is already a containing group of group with id %d", groupID, id)
	}
}
