package migrations

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type migrator struct {
	db *sqlx.DB
}

func (m *migrator) withTxn(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// a panic occurred, rollback and repanic
			_ = tx.Rollback()
			panic(p)
		}

		if err != nil {
			// something went wrong, rollback
			_ = tx.Rollback()
		} else {
			// all good, commit
			err = tx.Commit()
		}
	}()

	err = fn(tx)
	return err
}

// A post migration can run again after it has finished, if Stash stopped
// before it was recorded as done (#2221). These tell a step what it has
// already done.

// schemaObjectExists reports whether the database has a table or index of
// that name.
func (m *migrator) schemaObjectExists(ctx context.Context, kind, name string) (bool, error) {
	var exists bool
	err := m.db.GetContext(ctx, &exists, "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = ? AND name = ?)", kind, name)
	return exists, err
}

// columnExists reports whether the table has a column of that name.
func (m *migrator) columnExists(ctx context.Context, table, column string) (bool, error) {
	var exists bool
	err := m.db.GetContext(ctx, &exists, "SELECT EXISTS (SELECT 1 FROM pragma_table_info(?) WHERE name = ?)", table, column)
	return exists, err
}

func (m *migrator) execAll(stmts []string) error {
	for _, stmt := range stmts {
		if _, err := m.db.Exec(stmt); err != nil {
			return fmt.Errorf("executing statement %s: %w", stmt, err)
		}
	}

	return nil
}
