//go:build integration

package migrations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/require"
)

// rerunSeeds adds data, before the migration to that version, for the steps
// that change existing rows, so that running them twice has something to get
// wrong.
var rerunSeeds = map[uint]string{
	// aliases to split, a duplicate name and a disambiguation in the name
	42: `
INSERT INTO performers(id,name,checksum,aliases,created_at,updated_at) VALUES
  (1,'Dup','c1','one, two/three',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (2,'Dup','c2',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (3,'Jane (Doe)','c3',NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`,
	// a tag image to move to the blobs table
	45: `
INSERT INTO tags(id,name,created_at,updated_at) VALUES (1,'tag',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
INSERT INTO tags_image(tag_id,image) VALUES (1,X'0102030405');`,
	// a saved filter in the old format
	49: `
INSERT INTO saved_filters(id,name,mode,filter) VALUES (1,'old filter','SCENES',
  '{"q":"needle","sortby":"title","sortdir":"desc","perPage":20,"disp":1,"c":["{\"type\":\"rating\",\"modifier\":\"GREATER_THAN\",\"value\":{\"value\":\"60\"}}","{\"type\":\"title\",\"modifier\":\"INCLUDES\",\"value\":\"word\"}"]}');`,
	// one career length that parses and one that doesn't
	78: `
INSERT INTO performers(id,name,career_length,created_at,updated_at) VALUES
  (10,'career','2001 - 2005',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
  (11,'odd career','a long time',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`,
}

// The post49 result for the seeded filter.
const (
	rerun49FindFilter   = `{"q":"needle","page":1,"per_page":20,"sort":"title","direction":"DESC"}`
	rerun49ObjectFilter = `{"rating100":{"modifier":"GREATER_THAN","value":{"value":60}},"title":{"modifier":"INCLUDES","value":"word"}}`
)

// dumpDatabase returns the schema and every row, other than migration
// bookkeeping, in a stable order.
func dumpDatabase(t *testing.T, raw *sqlx.DB) []string {
	t.Helper()
	var objects []struct {
		Type string  `db:"type"`
		Name string  `db:"name"`
		SQL  *string `db:"sql"`
	}
	require.NoError(t, raw.Select(&objects, "SELECT type, name, sql FROM sqlite_master WHERE name NOT IN ('schema_migrations', 'pending_post_migrations') AND name NOT LIKE 'sqlite_%' ORDER BY type, name"))
	var out []string
	for _, o := range objects {
		sql := ""
		if o.SQL != nil {
			sql = *o.SQL
		}
		out = append(out, fmt.Sprintf("%s %s: %s", o.Type, o.Name, sql))
		if o.Type != "table" {
			continue
		}
		out = append(out, dumpTable(t, raw, o.Name)...)
	}
	return out
}

// dumpTable returns the table's rows as text, sorted.
func dumpTable(t *testing.T, raw *sqlx.DB, table string) []string {
	t.Helper()
	rows, err := raw.Queryx("SELECT * FROM `" + table + "`")
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		values, err := rows.SliceScan()
		require.NoError(t, err)
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}
		out = append(out, fmt.Sprintf("%s %v", table, values))
	}
	require.NoError(t, rows.Err())
	sort.Strings(out)
	return out
}

// dumpFiles returns the files next to the database and the config file's
// contents, which some steps change.
func dumpFiles(t *testing.T, dir, configFile string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	return append(out, string(data))
}

// A follow-up step runs again if Stash stops after it has finished but before
// it is recorded as done (#2221). Every step, run a second time on a database
// built the normal way, must succeed and change nothing.
func TestPostMigrationsRunTwice(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.InitializeEmpty()
	configFile := filepath.Join(dir, "config.yml")
	cfg.SetConfigFile(configFile)
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
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	for version := uint(2); version <= m.RequiredSchemaVersion(); version++ {
		if seed, ok := rerunSeeds[version]; ok {
			_, err := raw.Exec(seed)
			require.NoErrorf(t, err, "seeding before schema %d", version)
		}
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)

		dbBefore := dumpDatabase(t, raw)
		filesBefore := dumpFiles(t, dir, configFile)

		// as if Stash had stopped before recording the step as done; versions
		// without a step have nothing to run
		_, err = raw.Exec("CREATE TABLE IF NOT EXISTS `pending_post_migrations` (`version` INTEGER PRIMARY KEY); INSERT INTO `pending_post_migrations` VALUES (?)", version)
		require.NoError(t, err)
		require.NoErrorf(t, m.RunPendingPostMigrations(ctx), "second run of schema %d", version)

		var pending int
		require.NoError(t, raw.Get(&pending, "SELECT count(*) FROM `pending_post_migrations`"))
		require.Zerof(t, pending, "schema %d", version)
		require.Equalf(t, strings.Join(dbBefore, "\n"), strings.Join(dumpDatabase(t, raw), "\n"), "second run of schema %d changed the database", version)
		require.Equalf(t, filesBefore, dumpFiles(t, dir, configFile), "second run of schema %d changed the files", version)

		if version == 49 {
			var findFilter, objectFilter string
			require.NoError(t, raw.QueryRow("SELECT find_filter, object_filter FROM saved_filters WHERE id = 1").Scan(&findFilter, &objectFilter))
			require.JSONEq(t, rerun49FindFilter, findFilter)
			require.JSONEq(t, rerun49ObjectFilter, objectFilter)
		}
	}

	// the seeded rows came through the whole upgrade
	var aliases []string
	require.NoError(t, raw.Select(&aliases, "SELECT alias FROM performer_aliases WHERE performer_id = 1 ORDER BY alias"))
	require.Equal(t, []string{"one", "three", "two"}, aliases)
	var name, disambiguation string
	require.NoError(t, raw.QueryRow("SELECT name, disambiguation FROM performers WHERE id = 3").Scan(&name, &disambiguation))
	require.Equal(t, []string{"Jane", "Doe"}, []string{name, disambiguation})
	var blob []byte
	require.NoError(t, raw.Get(&blob, "SELECT b.blob FROM tags t JOIN blobs b ON b.checksum = t.image_blob WHERE t.id = 1"))
	require.Equal(t, []byte{1, 2, 3, 4, 5}, blob)
	var start, end string
	require.NoError(t, raw.QueryRow("SELECT strftime('%Y', career_start), strftime('%Y', career_end) FROM performers WHERE id = 10").Scan(&start, &end))
	require.Equal(t, []string{"2001", "2005"}, []string{start, end})
	var users int
	require.NoError(t, raw.Get(&users, "SELECT count(*) FROM users"))
	require.Equal(t, 1, users)

	// the step's filter keeps its criteria in a form the app reads
	var objectFilter string
	require.NoError(t, raw.Get(&objectFilter, "SELECT object_filter FROM saved_filters WHERE id = 1"))
	var criteria map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(objectFilter), &criteria))
	require.Contains(t, criteria, "title")
}
