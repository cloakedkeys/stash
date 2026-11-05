//go:build integration

package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/fsutil"
)

// The restore waits for a request that's still using the database, then
// works, instead of finding the database still open and giving up.
func TestRestoreWaitsForRequest(t *testing.T) {
	path := createSchema100Database(t)
	failMigration(t, oldSchemaVersion+2)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	backupPath := db.DatabaseBackupPath(filepath.Dir(path))
	upgradeUntilFailure(t, db, backupPath)

	// a request part way through reading
	ctx, release, err := db.WithDatabase(context.Background())
	require.NoError(t, err)
	rows, err := db.readDB.QueryContext(ctx, "SELECT name FROM tags")
	require.NoError(t, err)
	require.True(t, rows.Next())
	endRequest := func() {
		assert.NoError(t, rows.Err())
		_ = rows.Close()
		release()
	}

	done := make(chan error, 1)
	go func() { done <- db.RestoreFromBackup(backupPath) }()

	select {
	case err := <-done:
		endRequest()
		t.Fatalf("restore finished while a request held the database: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	endRequest()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("restore didn't finish after the request ended")
	}
	assert.Equal(t, uint(oldSchemaVersion), db.Version())
	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, uint(oldSchemaVersion), version)
	assert.False(t, dirty)
}

const closeFailDriver = "sqlite3ex_closefail"

var errForcedClose = errors.New("forced close failure")

func init() {
	sql.Register(closeFailDriver, &closeFailingDriver{})
}

// closeFailingDriver stands in for a driver whose connections fail to close.
// They do close, but report an error.
type closeFailingDriver struct{}

func (d *closeFailingDriver) Open(dsn string) (driver.Conn, error) {
	conn, err := (&CustomSQLiteDriver{}).Open(dsn)
	if err != nil {
		return nil, err
	}
	return closeFailingConn{conn.(*CustomSQLiteConn)}, nil
}

type closeFailingConn struct {
	*CustomSQLiteConn
}

func (c closeFailingConn) Close() error {
	_ = c.CustomSQLiteConn.Close()
	return errForcedClose
}

// If closing the database fails, the restore stops, leaves the backup as it
// was, and opens the database again.
func TestRestoreAfterCloseErrorReopens(t *testing.T) {
	path := createSchema100Database(t)
	backupPath := path + ".backup"
	require.NoError(t, os.WriteFile(backupPath, readFile(t, path), 0o600))
	backup := readFile(t, backupPath)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	// swap in a read pool whose connections fail to close
	require.NoError(t, db.readDB.Close())
	require.NoError(t, db.openReadDBWithDriver(closeFailDriver))
	var n int
	require.NoError(t, db.readDB.Get(&n, "SELECT COUNT(*) FROM tags"))
	failingPool := db.readDB
	writePool := db.writeDB

	err := db.RestoreFromBackup(backupPath)
	require.ErrorIs(t, err, errForcedClose)
	assert.ErrorContains(t, err, "left as it was")
	assert.True(t, bytes.Equal(backup, readFile(t, backupPath)), "the backup changed")

	// open again, with new pools, and the old ones all closed
	assert.NotSame(t, failingPool, db.readDB)
	assert.NotSame(t, writePool, db.writeDB)
	assert.ErrorContains(t, writePool.Ping(), "database is closed", "the old write pool was left open")
	var needed *database.MigrationNeededError
	require.ErrorAs(t, db.Ready(), &needed)
	require.NoError(t, db.readDB.Get(&n, "SELECT COUNT(*) FROM tags"))
	assert.Equal(t, 1, n)
}

// If the backup is put back but the database then can't be opened, the error
// says the backup was restored.
func TestRestoreReopenFailureSaysRestored(t *testing.T) {
	path := createSchema100Database(t)
	backupPath := path + ".backup"
	require.NoError(t, os.WriteFile(backupPath, readFile(t, path), 0o600))
	// a backup from a newer Stash, which this one won't open
	conn, err := sql.Open("sqlite3", "file:"+backupPath)
	require.NoError(t, err)
	_, err = conn.Exec("UPDATE schema_migrations SET version = 999")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	backup := readFile(t, backupPath)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })

	err = db.RestoreFromBackup(backupPath)
	require.ErrorIs(t, err, database.ErrRestoredNotReopened)
	var mismatch *database.MismatchedSchemaVersionError
	assert.ErrorAs(t, err, &mismatch)

	exists, _ := fsutil.FileExists(backupPath)
	assert.False(t, exists, "the backup should have been moved into place")
	assert.True(t, bytes.Equal(backup, readFile(t, path)), "the backup isn't in place")
}
