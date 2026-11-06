package migrations

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sqlite"
)

// pre62 gives a name to performers that have none. Migration 62 makes the
// name required, so the upgrade would otherwise stop (#2244).
func pre62(ctx context.Context, db *sqlx.DB) error {
	m := migrator{db: db}
	return m.withTxn(ctx, func(tx *sqlx.Tx) error {
		var ids []int
		if err := tx.Select(&ids, "SELECT id FROM performers WHERE name IS NULL ORDER BY id"); err != nil {
			return err
		}
		for _, id := range ids {
			name, err := unnamedPerformerName(tx, id)
			if err != nil {
				return err
			}
			logger.Warnf("Performer %d has no name, which is now required. Naming it %q.", id, name)
			if _, err := tx.Exec("UPDATE performers SET name = ? WHERE id = ?", name, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// unnamedPerformerName returns "Unnamed performer <id>", or, if another
// performer already has that name, the first of "Unnamed performer <id>
// (2)", "(3)" and so on that is free, so the unique name index holds.
func unnamedPerformerName(tx *sqlx.Tx, id int) (string, error) {
	base := fmt.Sprintf("Unnamed performer %d", id)
	name := base
	for n := 2; ; n++ {
		var taken bool
		if err := tx.Get(&taken, "SELECT EXISTS (SELECT 1 FROM performers WHERE name = ?)", name); err != nil {
			return "", err
		}
		if !taken {
			return name, nil
		}
		name = fmt.Sprintf("%s (%d)", base, n)
	}
}

func init() {
	sqlite.RegisterPreMigration(62, pre62)
}
