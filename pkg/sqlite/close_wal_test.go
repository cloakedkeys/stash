//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/fsutil"
)

const oldSchemaVersion = 100

// createDatabase makes a current database and closes it. With an old schema
// version, it's marked as needing an upgrade.
func createDatabase(t *testing.T, version uint) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	require.NoError(t, db.Open(context.Background(), path))
	require.NoError(t, db.Close())

	if version != appSchemaVersion {
		conn, err := sql.Open("sqlite3", "file:"+path)
		require.NoError(t, err)
		_, err = conn.Exec("UPDATE schema_migrations SET version = ?", version)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}
	requireNoWAL(t, path)
	return path
}

// openAndRead opens the database as Stash does when it starts, and reads
// from it, which is all Stash does before the upgrade.
func openAndRead(t *testing.T, path string) *Database {
	t.Helper()
	db := NewDatabase()
	require.NoError(t, db.Open(context.Background(), path))
	var n int
	require.NoError(t, db.readDB.Get(&n, "SELECT COUNT(*) FROM scenes"))
	return db
}

func walFiles(path string) (wal bool, shm bool) {
	wal, _ = fsutil.FileExists(path + "-wal")
	shm, _ = fsutil.FileExists(path + "-shm")
	return
}

func requireNoWAL(t *testing.T, path string) {
	t.Helper()
	wal, shm := walFiles(path)
	require.False(t, wal, "-wal file")
	require.False(t, shm, "-shm file")
}

// schemaState reads the schema version and the statistics with the plain
// go-sqlite3 driver, so reading them changes nothing.
func schemaState(t *testing.T, path string) (version uint, dirty bool, stats int) {
	t.Helper()
	conn, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	require.NoError(t, conn.QueryRow("SELECT COUNT(*) FROM sqlite_stat1").Scan(&stats))
	return
}

// Closing a database that still needs an upgrade removes its -wal and -shm
// files, as it does for an up-to-date one (#2111), and changes nothing in it.
func TestCloseBeforeUpgradeRemovesWAL(t *testing.T) {
	path := createDatabase(t, oldSchemaVersion)
	_, _, statsBefore := schemaState(t, path)

	db := openAndRead(t, path)
	require.Error(t, db.Ready(), "should need an upgrade")
	wal, _ := walFiles(path)
	require.True(t, wal, "reading should have opened the -wal file")

	require.NoError(t, db.Close())

	wal, shm := walFiles(path)
	assert.False(t, wal, "-wal file left behind")
	assert.False(t, shm, "-shm file left behind")

	version, dirty, stats := schemaState(t, path)
	assert.Equal(t, uint(oldSchemaVersion), version)
	assert.False(t, dirty)
	assert.Equal(t, statsBefore, stats, "statistics changed")
}

// The same for a database that's never read before it closes.
func TestCloseBeforeUpgradeWithoutReads(t *testing.T) {
	path := createDatabase(t, oldSchemaVersion)
	db := NewDatabase()
	require.NoError(t, db.Open(context.Background(), path))
	require.NoError(t, db.Close())
	requireNoWAL(t, path)
	version, _, _ := schemaState(t, path)
	assert.Equal(t, uint(oldSchemaVersion), version)
}

// An up-to-date database's files are still removed when it closes.
func TestCloseUpToDateRemovesWAL(t *testing.T) {
	path := createDatabase(t, appSchemaVersion)
	db := openAndRead(t, path)
	var n int
	require.NoError(t, db.writeDB.Get(&n, "SELECT COUNT(*) FROM scenes"))
	require.NoError(t, db.Close())
	requireNoWAL(t, path)
	version, _, _ := schemaState(t, path)
	assert.Equal(t, appSchemaVersion, version)
}

// The same happens to an up-to-date database once the write pool's connection
// has sat idle long enough to close, while the read pool's are still open.
func TestCloseAfterWriteConnectionIdledRemovesWAL(t *testing.T) {
	path := createDatabase(t, appSchemaVersion)
	db := openAndRead(t, path)
	// what the idle timeout does to the write pool
	db.writeDB.SetMaxIdleConns(0)
	require.Zero(t, db.writeDB.Stats().OpenConnections)
	require.Positive(t, db.readDB.Stats().OpenConnections)

	require.NoError(t, db.Close())
	requireNoWAL(t, path)
}

// While another connection still has the database open, closing leaves the
// files for it, and it keeps working.
func TestCloseKeepsWALWhileOpenElsewhere(t *testing.T) {
	path := createDatabase(t, oldSchemaVersion)

	other, err := sql.Open("sqlite3", "file:"+path+"?_journal=WAL")
	require.NoError(t, err)
	defer other.Close()
	other.SetMaxOpenConns(1)
	var n int
	require.NoError(t, other.QueryRow("SELECT COUNT(*) FROM scenes").Scan(&n))

	db := openAndRead(t, path)
	require.NoError(t, db.Close())

	wal, shm := walFiles(path)
	assert.True(t, wal, "-wal file removed while in use")
	assert.True(t, shm, "-shm file removed while in use")

	_, err = other.Exec("INSERT INTO tags (name, created_at, updated_at) VALUES ('x', '2026-01-01', '2026-01-01')")
	require.NoError(t, err)
	require.NoError(t, other.QueryRow("SELECT COUNT(*) FROM tags").Scan(&n))
	assert.Equal(t, 1, n)
}

// Closing a database that was never opened doesn't create one.
func TestCloseUnopenedCreatesNothing(t *testing.T) {
	db := NewDatabase()
	require.NoError(t, db.Close())

	db.dbPath = filepath.Join(t.TempDir(), "stash-go.sqlite")
	require.NoError(t, db.Close())
	exists, _ := fsutil.FileExists(db.dbPath)
	assert.False(t, exists)
}
