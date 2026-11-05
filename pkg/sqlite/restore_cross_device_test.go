//go:build integration

package sqlite

import (
	"bytes"
	"errors"
	"fmt"
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

// stubSync replaces syncFile and syncDir until the test ends. Each records
// what it was asked to sync, in order with the renames, then calls fn, or the
// real one if fn is nil.
func stubSync(t *testing.T, events *[]string, fileFn, dirFn func(string) error) {
	t.Helper()
	oldFile, oldDir, oldRename := syncFile, syncDir, renameFile
	syncFile = func(path string) error {
		*events = append(*events, "sync file "+filepath.Base(path))
		if fileFn != nil {
			return fileFn(path)
		}
		return oldFile(path)
	}
	syncDir = func(dir string) error {
		*events = append(*events, "sync dir "+dir)
		if dirFn != nil {
			return dirFn(dir)
		}
		return oldDir(dir)
	}
	renameFile = func(oldpath, newpath string) error {
		err := oldRename(oldpath, newpath)
		*events = append(*events, "rename "+filepath.Base(oldpath)+" ok="+fmt.Sprint(err == nil))
		return err
	}
	t.Cleanup(func() { syncFile, syncDir, renameFile = oldFile, oldDir, oldRename })
}

// backupOnAnotherDrive makes a database and a copy of it as a backup in
// another folder, from which renames fail as they do across drives.
func backupOnAnotherDrive(t *testing.T) (path, backupPath string) {
	t.Helper()
	path = createSchema100Database(t)
	backupDir := t.TempDir()
	backupPath = filepath.Join(backupDir, "backup.sqlite")
	require.NoError(t, os.WriteFile(backupPath, readFile(t, path), 0o600))
	renameAcrossDrives(t, backupDir, false)
	return path, backupPath
}

// The copy is written to disk before it replaces the database, and the
// folder after, both before the backup is removed.
func TestRestoreFromAnotherDriveSyncs(t *testing.T) {
	path, backupPath := backupOnAnotherDrive(t)
	var events []string
	stubSync(t, &events, nil, nil)

	db := NewDatabase()
	db.dbPath = path
	require.NoError(t, db.RestoreFromBackup(backupPath))

	assert.Equal(t, []string{
		"rename backup.sqlite ok=false",
		"sync file stash-go.sqlite.restoring",
		"rename stash-go.sqlite.restoring ok=true",
		"sync dir " + filepath.Dir(path),
	}, events)
	exists, _ := fsutil.FileExists(backupPath)
	assert.False(t, exists, "the backup should have been removed")
}

// If the copy can't be written to disk, that's a failed copy: the copy is
// removed, and the backup and database are left as they were.
func TestRestoreFromAnotherDriveCopySyncFails(t *testing.T) {
	path, backupPath := backupOnAnotherDrive(t)
	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	_, err := db.writeDB.Exec("UPDATE tags SET name = 'changed'")
	require.NoError(t, err)
	backup := readFile(t, backupPath)
	require.NoError(t, db.Close())
	before := readFile(t, path)
	require.NoError(t, db.ReInitialise())

	errSync := errors.New("forced sync failure")
	var events []string
	stubSync(t, &events, func(string) error { return errSync }, nil)

	err = db.RestoreFromBackup(backupPath)
	require.ErrorIs(t, err, errSync)
	assert.ErrorContains(t, err, "left as it was")
	assert.NotContains(t, events, "rename stash-go.sqlite.restoring ok=true")

	assert.True(t, bytes.Equal(backup, readFile(t, backupPath)), "the backup changed")
	assert.True(t, bytes.Equal(before, readFile(t, path)), "the database changed")
	exists, _ := fsutil.FileExists(path + ".restoring")
	assert.False(t, exists, "the copy was left behind")
	var name string
	require.NoError(t, db.readDB.Get(&name, "SELECT name FROM tags"))
	assert.Equal(t, "changed", name, "the database should be open again, as it was")
}

// If the folder can't be written to disk after the database is replaced, the
// restore has happened, so it succeeds, but the backup is kept.
func TestRestoreFromAnotherDriveDirSyncFailsKeepsBackup(t *testing.T) {
	path, backupPath := backupOnAnotherDrive(t)
	backup := readFile(t, backupPath)
	var events []string
	stubSync(t, &events, nil, func(string) error { return errors.New("forced sync failure") })

	db := NewDatabase()
	db.dbPath = path
	require.NoError(t, db.RestoreFromBackup(backupPath))

	assert.True(t, bytes.Equal(backup, readFile(t, path)), "the backup isn't in place")
	assert.True(t, bytes.Equal(backup, readFile(t, backupPath)), "the backup should be kept")
}
