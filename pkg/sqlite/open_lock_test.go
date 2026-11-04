//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// holdWriteLock makes another connection hold the database's write lock until
// release is called or the test ends.
func holdWriteLock(t *testing.T, path string) (release func()) {
	t.Helper()
	ctx := context.Background()
	other, err := sqlx.Open(sqlite3Driver, "file:"+path+"?_journal=WAL")
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	lock, err := other.Conn(ctx)
	require.NoError(t, err)
	_, err = lock.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	var once sync.Once
	release = func() {
		once.Do(func() {
			_, _ = lock.ExecContext(ctx, "ROLLBACK")
			_ = lock.Close()
		})
	}
	t.Cleanup(release)
	return release
}

// Opening a database while another connection is writing doesn't wait the busy
// timeout, because checking the schema version only reads (#2071).
func TestOpenDoesNotWaitForWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())

	release := holdWriteLock(t, path)

	db = NewDatabase()
	started := time.Now()
	require.NoError(t, db.Open(ctx, path))
	elapsed := time.Since(started)

	assert.Less(t, elapsed, busyTimeout/2, "waited %v", elapsed)
	assert.Equal(t, appSchemaVersion, db.schemaVersion)
	assert.NoError(t, db.ready())

	release()
	require.NoError(t, db.Close())
}

// A database that needs migrating still opens, without waiting, and still says
// it needs migrating.
func TestOpenNeedingMigrationDoesNotWaitForWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())

	release := holdWriteLock(t, path)

	started := time.Now()
	newer, err := openAt(t, path, appSchemaVersion+1)
	elapsed := time.Since(started)
	require.NoError(t, err)
	t.Cleanup(func() {
		release()
		_ = newer.Close()
	})

	assert.Less(t, elapsed, busyTimeout/2, "waited %v", elapsed)
	assert.Equal(t, appSchemaVersion, newer.schemaVersion)

	saved := appSchemaVersion
	appSchemaVersion = saved + 1
	t.Cleanup(func() { appSchemaVersion = saved })
	var migrationNeeded *database.MigrationNeededError
	assert.ErrorAs(t, newer.ready(), &migrationNeeded)
}
