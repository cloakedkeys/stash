//go:build integration

package sqlite

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawExec runs statements on the database file with a connection of its own.
func rawExec(t *testing.T, path string, query string, args ...interface{}) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", "file:"+filepath.ToSlash(path))
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Exec(query, args...)
	require.NoError(t, err)
}

// upgradeOnStart starts Stash on the database and upgrades it, as the
// upgrade task does.
func upgradeOnStart(t *testing.T, path string) {
	t.Helper()
	ctx := context.Background()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	defer db.Close()
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for m.CurrentSchemaVersion() < m.RequiredSchemaVersion() {
		require.NoErrorf(t, m.RunMigration(ctx, m.CurrentSchemaVersion()+1), "migrating to %d", m.CurrentSchemaVersion()+1)
	}
}

func tagNames(t *testing.T, path string) []string {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", "file:"+filepath.ToSlash(path))
	require.NoError(t, err)
	defer conn.Close()
	var names []string
	require.NoError(t, conn.Select(&names, "SELECT name FROM tags ORDER BY name"))
	return names
}

// Stash stops after a migration's SQL was saved but before the version was
// marked finished. On the next start the upgrade carries on from that
// version, with nothing lost (#2230).
func TestStoppedBeforeMarkedFinishedCarriesOn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	// its SQL can't run twice, so running it again doesn't hide a lost mark
	const target uint = 107
	endBuildAt(t, target+1)

	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, target-1)
	_, err = m.conn.Exec("INSERT INTO tags (name, created_at, updated_at) VALUES ('kept', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')")
	require.NoError(t, err)

	// golang-migrate's own write that marks the version finished never
	// happens, as if Stash had stopped just before it
	_, err = m.conn.Exec("CREATE TRIGGER stop_before_finished BEFORE INSERT ON schema_migrations WHEN NEW.dirty = 0 BEGIN SELECT RAISE(ABORT, 'stopped'); END")
	require.NoError(t, err)
	require.Error(t, m.RunMigration(ctx, target))
	m.Close()
	rawExec(t, path, "DROP TRIGGER stop_before_finished")

	upgradeOnStart(t, path)
	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, target+1, version)
	assert.False(t, dirty)
	assert.Equal(t, []string{"kept"}, tagNames(t, path))
}

// A second Stash starts while the first is running a migration's SQL. It
// must not set the version back once the first has saved it (#2230).
func TestStartDuringMigrationKeepsIt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	const target uint = 107
	endBuildAt(t, target+1)

	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, target-1)
	m.Close()

	// the first Stash, as golang-migrate runs a migration: the version
	// marked dirty, then the SQL, with its mark, in one write transaction
	// that isn't saved yet
	rawExec(t, path, "UPDATE schema_migrations SET version = ?, dirty = 1", target)
	r, _, err := markSource{mustSource(t)}.ReadUp(target)
	require.NoError(t, err)
	sqlText, err := io.ReadAll(r)
	_ = r.Close()
	require.NoError(t, err)

	first, err := sqlx.Open(sqlite3Driver, "file:"+filepath.ToSlash(path)+"?_journal=WAL&_busy_timeout=5000")
	require.NoError(t, err)
	defer first.Close()
	conn, err := first.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, string(sqlText))
	require.NoError(t, err)

	second := make(chan error, 1)
	go func() {
		db := NewDatabase()
		db.dbPath = path
		m, err := NewMigrator(db)
		if err == nil {
			m.Close()
		}
		second <- err
	}()

	// let the second Stash read the version and wait for the write lock
	time.Sleep(500 * time.Millisecond)
	_, err = conn.ExecContext(ctx, "COMMIT")
	require.NoError(t, err)
	require.NoError(t, <-second)

	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, target, version)
	assert.False(t, dirty)
}

func mustSource(t *testing.T) source.Driver {
	t.Helper()
	s, err := iofs.New(migrationsBox, "migrations")
	require.NoError(t, err)
	return s
}

// Stash stops while a migration's SQL is running, so none of it was saved
// and the version is left marked part way. On the next start that migration
// runs again and the upgrade finishes (#2230). The last migration of the
// build is one case: the database must not count as up to date.
func TestStoppedDuringMigrationRunsItAgain(t *testing.T) {
	const indexes109 = "SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN ('index_tags_relations_on_child_id', 'index_tag_stash_ids_on_tag_id')"
	for _, target := range []uint{109, appSchemaVersion} {
		t.Run(fmt.Sprintf("stopped during %d", target), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "stash-go.sqlite")

			db := NewDatabase()
			db.dbPath = path
			m, err := NewMigrator(db)
			require.NoError(t, err)
			migrateTo(t, m, target-1)
			m.Close()
			// what golang-migrate leaves when Stash stops during the SQL: the
			// version marked part way, none of the SQL's changes kept
			rawExec(t, path, "UPDATE schema_migrations SET version = ?, dirty = 1", target)

			upgradeOnStart(t, path)
			version, dirty, _ := schemaState(t, path)
			assert.Equal(t, appSchemaVersion, version)
			assert.False(t, dirty)
			// migration 109's changes are there, whichever was stopped
			conn, err := sqlx.Open("sqlite3", "file:"+filepath.ToSlash(path))
			require.NoError(t, err)
			defer conn.Close()
			var n int
			require.NoError(t, conn.Get(&n, indexes109))
			assert.Equal(t, 2, n, "migration 109's indexes")
		})
	}
}
