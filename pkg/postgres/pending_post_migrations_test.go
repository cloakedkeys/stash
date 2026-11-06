//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/models"
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

// While one Stash is still running a migration's follow-up step, a second
// Stash that opens the database or runs the next migration waits for it and
// then finds nothing left to do, rather than running the same step at the
// same time (review of #2220). The first stops at that migration, short of
// the build's version, so the second, which waited for it, runs no migration
// of its own either and says why (#2213).
func TestPostMigrationNotRunTwiceAtOnce(t *testing.T) {
	expected := map[string]error{"next migration": database.ErrUpgradeUnfinishedElsewhere}
	for name, second := range map[string]func(ctx context.Context, dsn string) error{
		"open": func(ctx context.Context, dsn string) error {
			db := NewDatabase()
			if err := db.Open(ctx, dsn); err != nil {
				return err
			}
			return db.Close()
		},
		"next migration": func(ctx context.Context, dsn string) error {
			db := NewDatabase()
			db.dbPath = dsn
			m, err := NewMigrator(db)
			if err != nil {
				return err
			}
			defer m.Close()
			return m.RunMigration(ctx, appVersionMigration+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dsn := migrationFixture(t)
			config.InitializeEmpty()
			target := appVersionMigration

			saved, had := postMigrations[target]
			t.Cleanup(func() {
				if had {
					postMigrations[target] = saved
				} else {
					delete(postMigrations, target)
				}
			})
			var calls atomic.Int32
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			postMigrations[target] = []customMigrationFunc{func(ctx context.Context, conn *sqlx.DB) error {
				calls.Add(1)
				started <- struct{}{}
				<-release
				return nil
			}}

			first := NewDatabase()
			first.dbPath = dsn
			m, err := NewMigrator(first)
			require.NoError(t, err)
			defer m.Close()
			migrateTo(t, m, target-1)
			firstDone := make(chan error, 1)
			go func() { firstDone <- m.RunMigration(ctx, target) }()
			<-started

			// the first one's SQL has committed and its follow-up step is running
			assert.Equal(t, []uint{target}, pendingPostMigrations(t, dsn))
			secondDone := make(chan error, 1)
			go func() { secondDone <- second(ctx, dsn) }()
			select {
			case err := <-secondDone:
				t.Fatalf("the second Stash didn't wait: %v", err)
			case <-time.After(time.Second):
			}
			assert.Equal(t, int32(1), calls.Load(), "the follow-up step ran twice at once")

			close(release)
			require.NoError(t, <-firstDone)
			// the first holds the lock until it is done with the database
			m.Close()
			if want := expected[name]; want != nil {
				require.ErrorIs(t, <-secondDone, want)
			} else {
				require.NoError(t, <-secondDone)
			}
			assert.Equal(t, int32(1), calls.Load(), "the second Stash ran the follow-up step again")
			assert.Empty(t, pendingPostMigrations(t, dsn))
		})
	}
}

// post15 on a new database, in setup, removes the reserved admin. Run again,
// with it already gone, it changes nothing rather than failing.
func TestPost15SetupRunAgain(t *testing.T) {
	ctx := models.WithSetupContext(context.Background())
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for v := uint(1); v <= 15; v++ {
		require.NoErrorf(t, m.RunMigration(ctx, v), "schema %d", v)
	}
	var users int
	require.NoError(t, m.conn.Get(&users, "SELECT count(*) FROM users"))
	require.Equal(t, 0, users)

	require.NoError(t, post15(ctx, m.conn))
	require.NoError(t, m.conn.Get(&users, "SELECT count(*) FROM users"))
	assert.Equal(t, 0, users)
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
