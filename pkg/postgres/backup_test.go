//go:build integration

package postgres

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
)

func TestClientToolEnv(t *testing.T) {
	env, dbName, err := clientToolEnv("postgres://user:secret@example.com:1234/stash?sslmode=verify-full")
	require.NoError(t, err)
	assert.Equal(t, "stash", dbName)
	assert.Subset(t, env, []string{"PGHOST=example.com", "PGPORT=1234", "PGUSER=user", "PGPASSWORD=secret", "PGSSLMODE=verify-full"})

	env, dbName, err = clientToolEnv("host=example.com port=1234 user=user password=secret dbname=stash sslmode=disable")
	require.NoError(t, err)
	assert.Equal(t, "stash", dbName)
	assert.Subset(t, env, []string{"PGHOST=example.com", "PGPASSWORD=secret", "PGSSLMODE=disable"})
}

func TestBackupToolNotFound(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		dsn = "postgres://user@localhost/stash"
	}

	t.Setenv("PATH", t.TempDir())

	db := &Database{dbPath: dsn, lockChan: make(chan struct{}, 1)}
	backupPath := filepath.Join(t.TempDir(), "backup")

	require.ErrorIs(t, db.Backup(backupPath), database.ErrBackupToolNotFound)
	assert.NoFileExists(t, backupPath, "no backup file claims to exist")
	assert.NoFileExists(t, backupPath+".tmp")

	require.NoError(t, os.WriteFile(backupPath, []byte("backup"), 0o600))
	require.ErrorIs(t, db.RestoreFromBackup(backupPath), database.ErrBackupToolNotFound)
}

func TestBackupAndRestore(t *testing.T) {
	dsn := os.Getenv("PGSQL_TEST")
	if dsn == "" {
		t.Skip("PGSQL_TEST is required")
	}
	if _, err := osexec.LookPath(pgDumpTool); err != nil {
		t.Skip("pg_dump is required")
	}

	_ = config.InitializeEmpty()

	ctx := context.Background()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	t.Cleanup(func() {
		_, _ = db.writeDB.Exec("DROP SCHEMA IF EXISTS " + restoreHoldingSchema + " CASCADE")
		require.NoError(t, db.Remove())
		require.NoError(t, db.Close())
	})

	runSQL := func(q string) {
		t.Helper()
		_, err := db.writeDB.Exec(q)
		require.NoError(t, err)
	}
	countTags := func() int {
		t.Helper()
		var count int
		require.NoError(t, db.readDB.Get(&count, "SELECT COUNT(*) FROM tags"))
		return count
	}
	tableExists := func(table string) bool {
		t.Helper()
		var exists bool
		require.NoError(t, db.readDB.Get(&exists, "SELECT to_regclass('public."+table+"') IS NOT NULL"))
		return exists
	}

	runSQL("INSERT INTO tags (name, created_at, updated_at) VALUES ('in the backup', now(), now())")
	require.Equal(t, 1, countTags())

	backupPath := filepath.Join(t.TempDir(), "backup.dump")
	require.NoError(t, db.Backup(backupPath))
	info, err := os.Stat(backupPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())
	assert.NoFileExists(t, backupPath+".tmp")

	// changes after the backup, including one like a migration failing part way
	runSQL("INSERT INTO tags (name, created_at, updated_at) VALUES ('after the backup', now(), now())")
	runSQL("CREATE TABLE added_by_failed_migration (id integer)")
	require.Equal(t, 2, countTags())

	require.NoError(t, db.RestoreFromBackup(backupPath))
	assert.Equal(t, 1, countTags(), "the database is back to what the backup holds")
	assert.False(t, tableExists("added_by_failed_migration"))
	assert.FileExists(t, backupPath, "the backup is kept")
	assert.Equal(t, appSchemaVersion, db.Version())

	// sequences continue from where they were, so new rows can be added
	runSQL("INSERT INTO tags (name, created_at, updated_at) VALUES ('after the restore', now(), now())")
	require.Equal(t, 2, countTags())

	// a backup that can't be restored leaves the database as it was
	badPath := filepath.Join(t.TempDir(), "bad.dump")
	require.NoError(t, os.WriteFile(badPath, []byte("not a backup"), 0o600))
	require.Error(t, db.RestoreFromBackup(badPath))
	assert.Equal(t, 2, countTags())
	assert.False(t, tableExists(restoreHoldingSchema))
}
