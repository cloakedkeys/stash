//go:build integration

package sqlite

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/fsutil"
)

var errCrossDevice = errors.New("invalid cross-device link")

// renameAcrossDrives makes renames from dir fail, as a rename to another
// drive or volume does, until the test ends. With all set, every rename fails.
func renameAcrossDrives(t *testing.T, dir string, all bool) {
	t.Helper()
	old := renameFile
	renameFile = func(oldpath, newpath string) error {
		if all || filepath.Dir(oldpath) == dir {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errCrossDevice}
		}
		return old(oldpath, newpath)
	}
	t.Cleanup(func() { renameFile = old })
}

// A backup on another drive or volume is restored by copying it (#2123).
func TestRestoreFromAnotherDrive(t *testing.T) {
	path := createSchema100Database(t)
	failMigration(t, oldSchemaVersion+2)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	backupDir := t.TempDir()
	backupPath := db.DatabaseBackupPath(backupDir)
	upgradeUntilFailure(t, db, backupPath)
	backup := contents(t, backupPath)

	renameAcrossDrives(t, backupDir, false)
	require.NoError(t, db.RestoreFromBackup(backupPath))

	exists, _ := fsutil.FileExists(backupPath)
	assert.False(t, exists, "the backup should have been moved")
	exists, _ = fsutil.FileExists(path + ".restoring")
	assert.False(t, exists, "the copy was left behind")
	assert.Equal(t, backup, contents(t, path), "the database isn't the backup")
	assert.Equal(t, "ok", integrity(t, path))
	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, uint(oldSchemaVersion), version)
	assert.False(t, dirty)

	assert.Equal(t, uint(oldSchemaVersion), db.Version())
	var needed *database.MigrationNeededError
	require.ErrorAs(t, db.Ready(), &needed)
}

// If even the copy can't be put in place, the backup and the database are
// both left as they were, with nothing left beside them.
func TestRestoreFromAnotherDriveFailsCleanly(t *testing.T) {
	path := createSchema100Database(t)
	failMigration(t, oldSchemaVersion+2)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	backupDir := t.TempDir()
	backupPath := db.DatabaseBackupPath(backupDir)
	upgradeUntilFailure(t, db, backupPath)
	backup := readFile(t, backupPath)
	require.NoError(t, db.Close())
	failed := readFile(t, path)
	require.NoError(t, db.ReInitialise())

	renameAcrossDrives(t, backupDir, true)
	err := db.RestoreFromBackup(backupPath)
	require.ErrorIs(t, err, errCrossDevice)
	assert.ErrorContains(t, err, "left as it was")

	assert.True(t, bytes.Equal(backup, readFile(t, backupPath)), "the backup changed")
	assert.True(t, bytes.Equal(failed, readFile(t, path)), "the database changed")
	exists, _ := fsutil.FileExists(path + ".restoring")
	assert.False(t, exists, "the copy was left behind")
	var needed *database.MigrationNeededError
	require.ErrorAs(t, db.Ready(), &needed, "the database should be open again")
}

// A copy left by an earlier restore that stopped part way doesn't get in the way.
func TestRestoreFromAnotherDriveReplacesOldCopy(t *testing.T) {
	path := createSchema100Database(t)
	backupDir := t.TempDir()
	backupPath := filepath.Join(backupDir, "backup.sqlite")
	require.NoError(t, os.WriteFile(backupPath, readFile(t, path), 0o600))
	backup := readFile(t, backupPath)
	require.NoError(t, os.WriteFile(path+".restoring", []byte("partial"), 0o600))

	db := NewDatabase()
	db.dbPath = path
	renameAcrossDrives(t, backupDir, false)
	require.NoError(t, db.RestoreFromBackup(backupPath))
	assert.True(t, bytes.Equal(backup, readFile(t, path)))
	exists, _ := fsutil.FileExists(path + ".restoring")
	assert.False(t, exists)
}
