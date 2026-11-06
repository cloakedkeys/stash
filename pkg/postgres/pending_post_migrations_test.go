//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyPostMigration replaces the post migrations of version, for the test,
// with one that fails while fail is true and counts its calls.
type flakyPostMigration struct {
	fail  bool
	calls []uint
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
		f.calls = append(f.calls, version)
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

func pendingPostMigrations(t *testing.T, dsn string) []uint {
	t.Helper()
	conn, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer conn.Close()
	var versions []uint
	require.NoError(t, conn.Select(&versions, "SELECT version FROM pending_post_migrations ORDER BY version"))
	return versions
}

// A follow-up step that fails runs again on the next start, before the
// database counts as ready. If it fails again, Stash stops with a message
// saying so; once it succeeds, it doesn't run again (#2200).
func TestFailedPostMigrationRunsOnNextStart(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	target := appVersionMigration
	endBuildAt(t, target)
	post := addFlakyPostMigration(t, target)

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, target-1)
	require.ErrorContains(t, m.RunMigration(ctx, target), "the follow-up step broke")
	m.Close()
	version, dirty := readMigrationVersion(t, dsn)
	require.Equal(t, int(target), version)
	require.False(t, dirty)
	assert.Equal(t, []uint{target}, pendingPostMigrations(t, dsn))

	// it fails again: Stash doesn't start, and says why
	again := NewDatabase()
	err = again.Open(ctx, dsn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "follow-up step of the database upgrade to schema version 27 failed again")
	assert.Contains(t, err.Error(), "the follow-up step broke")
	assert.Equal(t, []uint{target, target}, post.calls)
	assert.Equal(t, []uint{target}, pendingPostMigrations(t, dsn))

	// it succeeds: the database is ready
	post.fail = false
	fixed := NewDatabase()
	require.NoError(t, fixed.Open(ctx, dsn))
	require.NoError(t, fixed.Ready())
	require.NoError(t, fixed.Close())
	assert.Equal(t, []uint{target, target, target}, post.calls)
	assert.Empty(t, pendingPostMigrations(t, dsn))

	// and doesn't run again
	later := NewDatabase()
	require.NoError(t, later.Open(ctx, dsn))
	require.NoError(t, later.Close())
	assert.Len(t, post.calls, 3)
}

// Trying the upgrade again without restarting runs the failed follow-up step
// before the next migration.
func TestFailedPostMigrationRunsBeforeNextMigration(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	target := appVersionMigration
	post := addFlakyPostMigration(t, target)

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, target-1)
	require.Error(t, m.RunMigration(ctx, target))
	m.Close()

	post.fail = false
	m, err = NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	next := addFlakyPostMigration(t, target+1)
	next.fail = false
	require.NoError(t, m.RunMigration(ctx, target+1))
	assert.Equal(t, []uint{target, target}, post.calls)
	assert.Equal(t, []uint{target + 1}, next.calls)
	assert.Empty(t, pendingPostMigrations(t, dsn))
}

// A new database whose build stops at a failed follow-up step runs it on the
// next start.
func TestFailedPostMigrationOnNewDatabase(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	target := appVersionMigration
	endBuildAt(t, target)
	post := addFlakyPostMigration(t, target)

	db := NewDatabase()
	require.ErrorContains(t, db.Open(ctx, dsn), "the follow-up step broke")
	assert.Equal(t, []uint{target}, pendingPostMigrations(t, dsn))

	post.fail = false
	again := NewDatabase()
	require.NoError(t, again.Open(ctx, dsn))
	defer again.Close()
	require.NoError(t, again.Ready())
	assert.Len(t, post.calls, 2)
	assert.Empty(t, pendingPostMigrations(t, dsn))
}

// A follow-up step that succeeds leaves nothing pending, and a database that
// reaches migration 30 without the table, as every database upgraded before
// it does, gets it.
func TestPendingPostMigrationsTable(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	migrateTo(t, m, 29)
	// migrations 15 and 26 have follow-up steps, so the table is there and empty
	assert.Empty(t, pendingPostMigrations(t, dsn))

	_, err = m.conn.Exec("DROP TABLE pending_post_migrations")
	require.NoError(t, err)
	require.NoError(t, m.RunMigration(ctx, 30))
	assert.Empty(t, pendingPostMigrations(t, dsn))
}

// post15 runs again when its record wasn't removed, so running it after it
// has finished must leave the admin account as it is.
func TestPost15RunAgainChangesNothing(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	migrateTo(t, m, 16)
	_, err = m.conn.Exec("UPDATE users SET username = 'chosen', password_hash = 'chosen-hash' WHERE id = 1")
	require.NoError(t, err)

	require.NoError(t, post15(ctx, m.conn))
	var username, hash string
	require.NoError(t, m.conn.QueryRow("SELECT username, password_hash FROM users WHERE id = 1").Scan(&username, &hash))
	assert.Equal(t, "chosen", username)
	assert.Equal(t, "chosen-hash", hash)
}
