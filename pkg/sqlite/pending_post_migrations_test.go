//go:build integration

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingTestVersion is the migration the tests here give a follow-up step.
// It has none of its own, so the test's step is the only one.
const pendingTestVersion uint = 108

// flakyPostMigration replaces the post migrations of version, for the test,
// with one that fails while fail is true and counts its calls.
type flakyPostMigration struct {
	fail  bool
	calls int
}

func addFlakyPostMigration(t *testing.T, version uint) *flakyPostMigration {
	t.Helper()
	saved, had := postMigrations[version]
	t.Cleanup(func() {
		if had {
			postMigrations[version] = saved
		} else {
			delete(postMigrations, version)
		}
	})
	f := &flakyPostMigration{fail: true}
	postMigrations[version] = []customMigrationFunc{func(ctx context.Context, conn *sqlx.DB) error {
		f.calls++
		if f.fail {
			return errors.New("the follow-up step broke")
		}
		return nil
	}}
	return f
}

// endBuildAt makes the build's schema version version, for the test.
func endBuildAt(t *testing.T, version uint) {
	t.Helper()
	saved := appSchemaVersion
	appSchemaVersion = version
	t.Cleanup(func() { appSchemaVersion = saved })
}

func migrateTo(t *testing.T, m *Migrator, version uint) {
	t.Helper()
	for v := m.CurrentSchemaVersion() + 1; v <= version; v++ {
		require.NoErrorf(t, m.RunMigration(context.Background(), v), "migrating to %d", v)
	}
}

// pendingPostMigrationRows reads the table, or gives nil when it isn't there.
func pendingPostMigrationRows(t *testing.T, path string) []uint {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", "file:"+filepath.ToSlash(path))
	require.NoError(t, err)
	defer conn.Close()
	var exists bool
	require.NoError(t, conn.Get(&exists, "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'pending_post_migrations')"))
	if !exists {
		return nil
	}
	var versions []uint
	require.NoError(t, conn.Select(&versions, "SELECT version FROM pending_post_migrations ORDER BY version"))
	return versions
}

// A follow-up step that didn't finish, here because it failed and the backup
// wasn't put back (as after a crash, or a restore that failed), runs again on
// the next start before the database counts as ready. If it fails again,
// Stash stops with a message saying so; once it succeeds, it doesn't run
// again (#2221).
func TestUnfinishedPostMigrationRunsOnNextStart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	target := pendingTestVersion
	endBuildAt(t, target)
	post := addFlakyPostMigration(t, target)

	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, target-1)
	require.ErrorContains(t, m.RunMigration(ctx, target), "the follow-up step broke")
	assert.Equal(t, target, m.CurrentSchemaVersion(), "the version counts as done")
	m.Close()
	assert.Equal(t, []uint{target}, pendingPostMigrationRows(t, path))

	// it fails again: Stash doesn't start, and says why
	again := NewDatabase()
	err = again.Open(ctx, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "follow-up step of the database upgrade to schema version 108 failed again")
	assert.Contains(t, err.Error(), "the follow-up step broke")
	assert.Equal(t, 2, post.calls)

	// it succeeds: the database is ready
	post.fail = false
	fixed := NewDatabase()
	require.NoError(t, fixed.Open(ctx, path))
	require.NoError(t, fixed.Ready())
	require.NoError(t, fixed.Close())
	assert.Equal(t, 3, post.calls)
	assert.Empty(t, pendingPostMigrationRows(t, path))

	// and doesn't run again
	later := NewDatabase()
	require.NoError(t, later.Open(ctx, path))
	require.NoError(t, later.Close())
	assert.Equal(t, 3, post.calls)
}

// Trying the upgrade again without restarting runs the unfinished follow-up
// step before the next migration.
func TestUnfinishedPostMigrationRunsBeforeNextMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	target := pendingTestVersion
	post := addFlakyPostMigration(t, target)

	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	migrateTo(t, m, target-1)
	require.Error(t, m.RunMigration(ctx, target))

	post.fail = false
	require.NoError(t, m.RunMigration(ctx, target+1))
	assert.Equal(t, 2, post.calls)
	assert.Empty(t, pendingPostMigrationRows(t, path))
}

// A new database, which gets no backup, whose build stops at a failed
// follow-up step runs it on the next start.
func TestUnfinishedPostMigrationOnNewDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	target := pendingTestVersion
	endBuildAt(t, target)
	post := addFlakyPostMigration(t, target)

	db := NewDatabase()
	require.ErrorContains(t, db.Open(ctx, path), "the follow-up step broke")
	_ = db.Close()
	assert.Equal(t, []uint{target}, pendingPostMigrationRows(t, path))

	post.fail = false
	again := NewDatabase()
	require.NoError(t, again.Open(ctx, path))
	defer again.Close()
	require.NoError(t, again.Ready())
	assert.Equal(t, 2, post.calls)
	assert.Empty(t, pendingPostMigrationRows(t, path))
}

// The record is written in the same transaction as the migration's SQL, so a
// migration whose SQL fails leaves no record behind.
func TestPendingPostMigrationRollsBackWithSQL(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	target := pendingTestVersion
	addFlakyPostMigration(t, target)

	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	migrateTo(t, m, target-1)
	// the table is there, so a record that wasn't rolled back would show;
	// migration 108 indexes scenes_tags, so without it its SQL fails
	_, err = m.conn.Exec("CREATE TABLE IF NOT EXISTS pending_post_migrations (version INTEGER PRIMARY KEY); DROP TABLE scenes_tags")
	require.NoError(t, err)
	require.Error(t, m.RunMigration(ctx, target))
	assert.Empty(t, pendingPostMigrationRows(t, path))
}

// A database that reaches migration 110 without the table, as every database
// upgraded before it does, gets it.
func TestPendingPostMigrationsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	migrateTo(t, m, 109)
	_, err = m.conn.Exec("DROP TABLE IF EXISTS pending_post_migrations")
	require.NoError(t, err)
	migrateTo(t, m, 110)

	var exists bool
	require.NoError(t, m.conn.Get(&exists, "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'pending_post_migrations')"))
	assert.True(t, exists)
	assert.Empty(t, pendingPostMigrationRows(t, path))
}
