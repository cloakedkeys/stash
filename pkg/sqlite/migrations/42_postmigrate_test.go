//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// the same name with a number in it, twice (#2241)
	"same bracketed name": `
INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES
  (1,'Kim (1)','c1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Kim (1)','c2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'Kim','c3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`,
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

// samePerformerNames inserts count performers called Dup.
func samePerformerNames(count int) string {
	values := make([]string, count)
	for i := range values {
		values[i] = fmt.Sprintf("(%d,'Dup','c%d',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)", i+1, i+1)
	}
	return "INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES " + strings.Join(values, ",") + ";"
}

// Step 42 numbers clashing names in order. With 10 or more clashes, each still
// gets a number of its own, so the unique index can be made (#2238). With
// fewer, the numbers are the ones it has always given.
func TestPost42NumbersManyClashes(t *testing.T) {
	for _, count := range []int{10, 12} {
		t.Run(fmt.Sprintf("%d performers", count), func(t *testing.T) {
			raw := schema41Database(t, samePerformerNames(count))
			applySchema42SQL(t, raw)
			require.NoError(t, post42(context.Background(), raw))

			// the first keeps its name as it is, the others are numbered from 1
			want := []string{`1 "Dup" ""`}
			for id := 2; id <= count; id++ {
				want = append(want, fmt.Sprintf(`%d "Dup" "%d"`, id, id-1))
			}
			require.Equal(t, want, schema42Names(t, raw))
		})
	}
}

// A performer whose name already ends in a number in brackets gets that
// number when the names are split, so the clashing plain names are given
// other numbers, and the unique index can be made (#2239).
func TestPost42SkipsNumbersInNames(t *testing.T) {
	raw := schema41Database(t, `
INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES
  (1,'Kim (1)','c1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Kim','c2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'Kim','c3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (4,'Kim','c4',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (5,'Lee (01)','c5',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (6,'Lee','c6',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (7,'Lee','c7',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
	applySchema42SQL(t, raw)
	require.NoError(t, post42(context.Background(), raw))

	require.Equal(t, []string{
		`1 "Kim" "1"`,
		`2 "Kim" ""`,
		`3 "Kim" "2"`,
		`4 "Kim" "3"`,
		// "01" isn't the same as "1", so 1 is free
		`5 "Lee" "01"`,
		`6 "Lee" ""`,
		`7 "Lee" "1"`,
	}, schema42Names(t, raw))
}

// Two performers with the same name that ends in a number in brackets don't
// both become that name and number: the first is split, the second keeps its
// name with the number it was given as a clash (#2241).
func TestPost42SameBracketedNames(t *testing.T) {
	raw := schema41Database(t, `
INSERT INTO performers(id,name,checksum,created_at,updated_at) VALUES
  (1,'Kim (1)','c1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Kim (1)','c2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'Ann (Smith)','c3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (4,'Ann (Smith)','c4',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (5,'Ann (Smith)','c5',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`)
	applySchema42SQL(t, raw)
	require.NoError(t, post42(context.Background(), raw))

	require.Equal(t, []string{
		`1 "Kim" "1"`,
		`2 "Kim (1)" "1"`,
		`3 "Ann" "Smith"`,
		`4 "Ann (Smith)" "1"`,
		`5 "Ann (Smith)" "2"`,
	}, schema42Names(t, raw))
}
