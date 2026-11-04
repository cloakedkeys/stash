//go:build integration

package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/fsutil"
)

// createSchema100Database makes a genuine schema-100 database, by running the
// real migrations up to 100, with a tag in it.
func createSchema100Database(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	db.dbPath = path
	m, err := NewMigrator(db)
	require.NoError(t, err)
	for v := m.CurrentSchemaVersion() + 1; v <= oldSchemaVersion; v++ {
		require.NoError(t, m.RunMigration(context.Background(), v), "migrating to %d", v)
	}
	_, err = m.conn.Exec("INSERT INTO tags (name, created_at, updated_at) VALUES ('kept', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')")
	require.NoError(t, err)
	m.Close()
	requireNoWAL(t, path)
	return path
}

// failMigration makes the post-migration for version fail, until the test ends.
func failMigration(t *testing.T, version uint) {
	t.Helper()
	old := postMigrations[version]
	fns := append([]customMigrationFunc{}, old...)
	postMigrations[version] = append(fns, func(context.Context, *sqlx.DB) error {
		return errors.New("forced failure")
	})
	t.Cleanup(func() { postMigrations[version] = old })
}

// upgradeUntilFailure runs the upgrade the way the migrate task does: back up,
// migrate one version at a time until one fails, then close the migrator.
func upgradeUntilFailure(t *testing.T, db *Database, backupPath string) {
	t.Helper()
	require.NoError(t, db.Backup(backupPath))

	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	var migrateErr error
	for migrateErr == nil && m.CurrentSchemaVersion() < appSchemaVersion {
		migrateErr = m.RunMigration(context.Background(), m.CurrentSchemaVersion()+1)
	}
	require.ErrorContains(t, migrateErr, "forced failure")
	require.Greater(t, m.CurrentSchemaVersion(), uint(oldSchemaVersion), "the failed upgrade should have changed the database")
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

// contents lists the schema and every row of every table, read with the plain
// go-sqlite3 driver.
func contents(t *testing.T, path string) []string {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer conn.Close()

	// each row as text, each query's rows closed before the next
	query := func(q string) [][]interface{} {
		rows, err := conn.Queryx(q)
		require.NoError(t, err)
		defer rows.Close()
		var ret [][]interface{}
		for rows.Next() {
			row, err := rows.SliceScan()
			require.NoError(t, err)
			ret = append(ret, row)
		}
		require.NoError(t, rows.Err())
		return ret
	}

	var ret []string
	var tables []string
	for _, row := range query("SELECT type, name, COALESCE(sql, '') FROM sqlite_master ORDER BY type, name") {
		ret = append(ret, fmt.Sprintf("%s", row))
		if fmt.Sprintf("%s", row[0]) == "table" {
			tables = append(tables, fmt.Sprintf("%s", row[1]))
		}
	}
	for _, table := range tables {
		for _, row := range query("SELECT * FROM `" + table + "` ORDER BY 1") {
			ret = append(ret, fmt.Sprintf("%s %v", table, row))
		}
	}
	require.NotEmpty(t, tables)
	return ret
}

func integrity(t *testing.T, path string) string {
	t.Helper()
	conn, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer conn.Close()
	var ret string
	require.NoError(t, conn.QueryRow("PRAGMA integrity_check").Scan(&ret))
	return ret
}

// After an upgrade fails, the backup is restored: the database is back at
// version 100, clean, identical to the backup, and open again (#2117).
func TestRestoreAfterFailedUpgrade(t *testing.T) {
	path := createSchema100Database(t)
	failMigration(t, oldSchemaVersion+2)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	backupPath := db.DatabaseBackupPath(filepath.Dir(path))
	upgradeUntilFailure(t, db, backupPath)
	backup := contents(t, backupPath)

	require.NoError(t, db.RestoreFromBackup(backupPath))

	exists, _ := fsutil.FileExists(backupPath)
	assert.False(t, exists, "the backup should have been moved into place")
	requireNoWAL(t, path)
	// opening the database switches it to WAL mode, which changes the
	// header, so compare what's in it
	assert.Equal(t, backup, contents(t, path), "the database isn't identical to the backup")
	assert.Equal(t, "ok", integrity(t, path))

	version, dirty, _ := schemaState(t, path)
	assert.Equal(t, uint(oldSchemaVersion), version)
	assert.False(t, dirty)

	// open again, and still waiting for the upgrade
	assert.Equal(t, uint(oldSchemaVersion), db.Version())
	var needed *database.MigrationNeededError
	require.ErrorAs(t, db.Ready(), &needed)
	var name string
	require.NoError(t, db.readDB.Get(&name, "SELECT name FROM tags"))
	assert.Equal(t, "kept", name)
}

// If another connection still has the database open, the restore stops with a
// clear error, the backup is left untouched and the database is open again.
func TestRestoreWhileOpenElsewhereLeavesBackup(t *testing.T) {
	path := createSchema100Database(t)
	failMigration(t, oldSchemaVersion+2)

	db := openAndRead(t, path)
	t.Cleanup(func() { _ = db.Close() })
	backupPath := db.DatabaseBackupPath(filepath.Dir(path))
	upgradeUntilFailure(t, db, backupPath)
	backup := readFile(t, backupPath)

	other, err := sql.Open("sqlite3", "file:"+path+"?_journal=WAL")
	require.NoError(t, err)
	defer other.Close()
	var n int
	require.NoError(t, other.QueryRow("SELECT COUNT(*) FROM tags").Scan(&n))

	err = db.RestoreFromBackup(backupPath)
	require.Error(t, err)
	assert.ErrorContains(t, err, "still open elsewhere")
	assert.ErrorContains(t, err, "left as it was")

	assert.True(t, bytes.Equal(backup, readFile(t, backupPath)), "the backup changed")
	// open again, still needing the upgrade
	var needed *database.MigrationNeededError
	require.ErrorAs(t, db.Ready(), &needed)
	require.NoError(t, db.readDB.Get(&n, "SELECT COUNT(*) FROM tags"))
}

// Restoring into a database that isn't open leaves it closed.
func TestRestoreUnopenedDatabase(t *testing.T) {
	path := createSchema100Database(t)
	backupPath := path + ".backup"
	require.NoError(t, os.WriteFile(backupPath, readFile(t, path), 0o600))
	backup := readFile(t, backupPath)

	db := NewDatabase()
	db.dbPath = path
	require.NoError(t, db.RestoreFromBackup(backupPath))
	assert.True(t, bytes.Equal(backup, readFile(t, path)), "not restored")
	assert.ErrorIs(t, db.Ready(), database.ErrDatabaseNotInitialized)
}
