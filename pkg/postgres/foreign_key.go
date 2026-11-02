package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// foreignKeyViolation is PostgreSQL's code for a row that points at something
// that doesn't exist.
const foreignKeyViolation = "23503"

// isForeignKeyError reports whether err is the database refusing a row that
// points at something that doesn't exist.
func isForeignKeyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}
