package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sliceutil"
	"github.com/stashapp/stash/pkg/sqlite"
)

type schema93Migrator struct {
	migrator
}

func post93(ctx context.Context, db *sqlx.DB) error {
	logger.Info("Running post-migration for schema version 93")

	m := schema93Migrator{
		migrator: migrator{
			db: db,
		},
	}

	if err := m.migrate(ctx); err != nil {
		return fmt.Errorf("migrating group aliases: %w", err)
	}

	return nil
}

// migrate splits the aliases string that migration 90 copied into a single
// alias entry, in the same way as migration 42 did for performer aliases.
// Groups with more than one alias already have a list and are left alone.
func (m *schema93Migrator) migrate(ctx context.Context) error {
	logger.Info("Migrating group aliases")

	const (
		limit    = 1000
		logEvery = 10000
	)

	lastID := 0
	count := 0

	for {
		gotSome := false

		if err := m.withTxn(ctx, func(tx *sqlx.Tx) error {
			query := "SELECT `group_id`, MAX(`alias`) FROM `group_aliases`"

			if lastID != 0 {
				query += fmt.Sprintf(" WHERE `group_id` > %d ", lastID)
			}

			query += fmt.Sprintf(" GROUP BY `group_id` HAVING COUNT(*) = 1 ORDER BY `group_id` LIMIT %d", limit)

			rows, err := tx.Query(query)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var (
					id      int
					aliases string
				)

				err := rows.Scan(&id, &aliases)
				if err != nil {
					return err
				}

				lastID = id
				gotSome = true
				count++

				if err := m.migrateGroupAliases(tx, id, aliases); err != nil {
					return err
				}
			}

			return rows.Err()
		}); err != nil {
			return err
		}

		if !gotSome {
			break
		}

		if count%logEvery == 0 {
			logger.Infof("Migrated %d rows", count)
		}
	}

	return nil
}

func (m *schema93Migrator) migrateGroupAliases(tx *sqlx.Tx, id int, aliases string) error {
	// split aliases by , or /
	aliasList := strings.FieldsFunc(aliases, func(r rune) bool {
		return strings.ContainsRune(",/", r)
	})

	// trim whitespace from each alias, dropping any left empty
	var trimmed []string
	for _, alias := range aliasList {
		if alias = strings.TrimSpace(alias); alias != "" {
			trimmed = append(trimmed, alias)
		}
	}

	if len(trimmed) < 2 {
		// existing value is fine
		return nil
	}

	// remove duplicates
	trimmed = sliceutil.AppendUniques(nil, trimmed)

	// delete the existing row
	if _, err := tx.Exec("DELETE FROM `group_aliases` WHERE `group_id` = ?", id); err != nil {
		return err
	}

	// insert aliases into table
	for _, alias := range trimmed {
		_, err := tx.Exec("INSERT INTO `group_aliases` (`group_id`, `alias`) VALUES (?, ?)", id, alias)
		if err != nil {
			return err
		}
	}

	return nil
}

func init() {
	sqlite.RegisterPostMigration(93, post93)
}
