//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/require"
)

// schema42Performers have clashing names, and a name with a disambiguation
// in it that becomes the same as another performer's name: one where the
// disambiguation is a number, and one where it is text.
var schema42Performers = map[string]string{
	"number": `
INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES
  (1,'Jane (2)','c1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Jane','c2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'Jane','c3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (4,'Bob','c4',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (5,'Bob','c5',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`,
	"text": `
INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES
  (1,'Ann (Smith)','c1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Ann','c2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'Ann','c3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (4,'Bob','c4',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (5,'Bob','c5',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`,
}

// schema41Database returns a database built the normal way to schema 41,
// with the performers.
func schema41Database(t *testing.T, performers string) *sqlx.DB {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
	require.NoError(t, cfg.Write())

	dbPath := filepath.Join(dir, "stash.sqlite")
	raw, err := sqlx.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	initial, err := os.ReadFile("1_initial.up.sql")
	require.NoError(t, err)
	_, err = raw.Exec(string(initial) + "\nCREATE TABLE schema_migrations(version uint64, dirty bool); INSERT INTO schema_migrations VALUES(1,false);")
	require.NoError(t, err)

	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, dbPath))
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	for version := uint(2); version <= 41; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	m.Close()
	require.NoError(t, db.Close())

	_, err = raw.Exec(performers)
	require.NoError(t, err)
	return raw
}

func schema42Names(t *testing.T, raw *sqlx.DB) []string {
	t.Helper()
	var rows []struct {
		ID             int            `db:"id"`
		Name           string         `db:"name"`
		Disambiguation sql.NullString `db:"disambiguation"`
	}
	require.NoError(t, raw.Select(&rows, "SELECT id, name, disambiguation FROM performers ORDER BY id"))
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%d %q %q", r.ID, r.Name, r.Disambiguation.String))
	}
	return out
}

// applySchema42SQL runs migration 42's SQL, without its follow-up step.
func applySchema42SQL(t *testing.T, raw *sqlx.DB) {
	t.Helper()
	body, err := os.ReadFile("42_performer_disambig_aliases.up.sql")
	require.NoError(t, err)
	_, err = raw.Exec(string(body))
	require.NoError(t, err)
}

// Step 42 stopped part way, then run again, gives every performer the same
// name and disambiguation as one run that didn't stop (#2231).
func TestPost42StoppedPartWayThenRunAgain(t *testing.T) {
	for name, performers := range schema42Performers {
		t.Run(name, func(t *testing.T) { testPost42StoppedPartWay(t, performers) })
	}
}

func testPost42StoppedPartWay(t *testing.T, performers string) {
	ctx := context.Background()

	clean := schema41Database(t, performers)
	applySchema42SQL(t, clean)
	require.NoError(t, post42(ctx, clean))
	want := schema42Names(t, clean)

	stops := map[string]func(t *testing.T, m *schema42Migrator){
		"after the clashing names": func(t *testing.T, m *schema42Migrator) {
			require.NoError(t, m.migrate(ctx))
			require.NoError(t, m.migrateDuplicatePerformers(ctx))
		},
		"before the indexes": func(t *testing.T, m *schema42Migrator) {
			require.NoError(t, m.migrate(ctx))
			require.NoError(t, m.migrateDuplicatePerformers(ctx))
			require.NoError(t, m.migratePerformersDisam(ctx))
		},
		"between the indexes": func(t *testing.T, m *schema42Migrator) {
			require.NoError(t, m.migrate(ctx))
			require.NoError(t, m.migrateDuplicatePerformers(ctx))
			require.NoError(t, m.migratePerformersDisam(ctx))
			_, err := m.db.Exec("CREATE UNIQUE INDEX `performers_name_disambiguation_unique` on `performers` (`name`, `disambiguation`) WHERE `disambiguation` IS NOT NULL")
			require.NoError(t, err)
		},
	}
	for name, stop := range stops {
		t.Run(name, func(t *testing.T) {
			raw := schema41Database(t, performers)
			applySchema42SQL(t, raw)
			stop(t, &schema42Migrator{migrator: migrator{db: raw}})

			require.NoError(t, post42(ctx, raw), "running step 42 again")
			require.Equal(t, want, schema42Names(t, raw))
		})
	}
}

// Step 42 run again after it finished changes no names.
func TestPost42RunAgainAfterFinishing(t *testing.T) {
	ctx := context.Background()
	raw := schema41Database(t, schema42Performers["number"])
	applySchema42SQL(t, raw)
	require.NoError(t, post42(ctx, raw))
	want := schema42Names(t, raw)

	require.NoError(t, post42(ctx, raw))
	require.Equal(t, want, schema42Names(t, raw))
}
