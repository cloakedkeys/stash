//go:build integration

package postgres

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
	"github.com/stretchr/testify/require"
)

// pgDump returns the columns and every row of the tables in the test's
// schema, other than migration bookkeeping, in a stable order.
func pgDump(t *testing.T, conn *sqlx.DB) []string {
	t.Helper()
	var columns []string
	require.NoError(t, conn.Select(&columns, `SELECT table_name || '.' || column_name || ' ' || data_type FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name NOT IN ('schema_migrations', 'pending_post_migrations') ORDER BY 1`))
	var tables []string
	require.NoError(t, conn.Select(&tables, `SELECT table_name FROM information_schema.tables
WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' AND table_name NOT IN ('schema_migrations', 'pending_post_migrations') ORDER BY 1`))
	out := columns
	for _, table := range tables {
		var rows []string
		require.NoError(t, conn.Select(&rows, fmt.Sprintf(`SELECT '%[1]s ' || to_jsonb(t)::text FROM %[1]q t`, table)))
		sort.Strings(rows)
		out = append(out, rows...)
	}
	return out
}

// pgDumpFiles returns the files next to the config file and its contents,
// which some steps change.
func pgDumpFiles(t *testing.T, configFile string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(configFile))
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
// its record is removed (#2200). Every step, run a second time on a database
// built the normal way, must succeed and change nothing.
func TestPostMigrationsRunTwice(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	cfg := config.GetInstance()
	configFile := filepath.Join(t.TempDir(), "config.yml")
	cfg.SetConfigFile(configFile)
	// default filters in the config file for post26 to rewrite
	var fileFilter map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(wildcardCases[0].filter), &fileFilter))
	cfg.SetUIConfiguration(map[string]interface{}{
		"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{"mode": "SCENES", "object_filter": fileFilter}},
	})
	require.NoError(t, cfg.Write())

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()

	for version := uint(1); version <= m.RequiredSchemaVersion(); version++ {
		if version == 26 {
			// saved filters and default filters for post26 to rewrite, and some
			// it leaves alone
			for i, c := range wildcardCases {
				_, err := m.conn.Exec(`INSERT INTO saved_filters(id,user_id,name,mode,find_filter,object_filter,ui_options) VALUES($1,1,$2,'SCENES','{"q":"a_b"}'::jsonb,$3::jsonb,'{}'::jsonb)`, i+1, c.name, c.filter)
				require.NoError(t, err)
			}
			for i, f := range untouchedFilters {
				_, err := m.conn.Exec(`INSERT INTO saved_filters(id,user_id,name,mode,object_filter) VALUES($1,1,$2,'SCENES',$3::jsonb)`, 100+i, "untouched "+f, f)
				require.NoError(t, err)
			}
			_, err := m.conn.Exec(`INSERT INTO user_ui_config(user_id,config) VALUES(1,$1)`, `{"defaultFilters":{"scenes":{"mode":"SCENES","object_filter":`+wildcardCases[0].filter+`}}}`)
			require.NoError(t, err)
		}
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)

		dbBefore := pgDump(t, m.conn)
		filesBefore := pgDumpFiles(t, configFile)

		// as if Stash had stopped before removing the record; versions without
		// a step have nothing to run
		_, err := m.conn.Exec("CREATE TABLE IF NOT EXISTS pending_post_migrations (version INTEGER PRIMARY KEY)")
		require.NoError(t, err)
		_, err = m.conn.Exec("INSERT INTO pending_post_migrations VALUES ($1)", version)
		require.NoError(t, err)
		require.NoErrorf(t, m.runPendingPostMigrations(ctx), "second run of schema %d", version)

		var pending int
		require.NoError(t, m.conn.Get(&pending, "SELECT count(*) FROM pending_post_migrations"))
		require.Zerof(t, pending, "schema %d", version)
		require.Equalf(t, strings.Join(dbBefore, "\n"), strings.Join(pgDump(t, m.conn), "\n"), "second run of schema %d changed the database", version)
		require.Equalf(t, filesBefore, pgDumpFiles(t, configFile), "second run of schema %d changed the files", version)
	}

	// the steps did their work the first time
	var username string
	var locked bool
	require.NoError(t, m.conn.QueryRow("SELECT username, locked FROM users WHERE id = 1").Scan(&username, &locked))
	require.Equal(t, "admin", username)
	require.False(t, locked)
	var rewritten int
	require.NoError(t, m.conn.Get(&rewritten, `SELECT count(*) FROM saved_filters WHERE id < 100 AND object_filter::text LIKE '%REGEX%'`))
	require.NotZero(t, rewritten)
	require.True(t, cfg.GetLegacyWildcardsConverted())
}
