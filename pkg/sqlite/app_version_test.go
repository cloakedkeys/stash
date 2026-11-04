//go:build integration

package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type appVersionRow struct {
	Version               string         `db:"version"`
	GitHash               string         `db:"githash"`
	FirstOpenedAt         time.Time      `db:"first_opened_at"`
	PreviousVersion       sql.NullString `db:"previous_version"`
	PreviousGitHash       sql.NullString `db:"previous_githash"`
	PreviousFirstOpenedAt sql.NullTime   `db:"previous_first_opened_at"`
}

func readAppVersionRow(t *testing.T, path string) appVersionRow {
	t.Helper()
	conn, err := sqlx.Open(sqlite3Driver, "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer conn.Close()
	var rows []appVersionRow
	require.NoError(t, conn.Select(&rows, "SELECT version, githash, first_opened_at, previous_version, previous_githash, previous_first_opened_at FROM app_version"))
	require.Len(t, rows, 1)
	return rows[0]
}

func openAt(t *testing.T, path string, schemaVersion uint) (*Database, error) {
	t.Helper()
	saved := appSchemaVersion
	appSchemaVersion = schemaVersion
	t.Cleanup(func() { appSchemaVersion = saved })
	db := NewDatabase()
	err := db.Open(context.Background(), path)
	appSchemaVersion = saved
	return db, err
}

// fileState is the database file's contents and modified time.
func fileState(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	return fmt.Sprintf("%x %v", sha256.Sum256(data), info.ModTime())
}

type warningLogger struct {
	logger.LoggerImpl
	mu       sync.Mutex
	warnings []string
}

func (l *warningLogger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

func (l *warningLogger) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.warnings, "\n")
}

func captureWarnings(t *testing.T) *warningLogger {
	t.Helper()
	previous := logger.Logger
	base := previous
	if base == nil {
		base = &logger.BasicLogger{}
	}
	l := &warningLogger{LoggerImpl: base}
	logger.Logger = l
	t.Cleanup(func() { logger.Logger = previous })
	return l
}

// The build that opens a database at the current schema version is recorded
// the first time it does, and the version before is kept (#2057).
func TestAppVersionStamp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	current := database.CurrentAppVersion()

	// a new database
	start := time.Now().Add(-time.Second)
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())
	row := readAppVersionRow(t, path)
	assert.Equal(t, current.Version, row.Version)
	assert.Equal(t, current.GitHash, row.GitHash)
	assert.True(t, row.FirstOpenedAt.After(start), "first opened at %v", row.FirstOpenedAt)
	assert.False(t, row.PreviousVersion.Valid)

	// opened again by the same build: nothing is written, so a backup tool
	// that copies changed files has nothing to copy
	before := fileState(t, path)
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())
	assert.Equal(t, row, readAppVersionRow(t, path))
	assert.Equal(t, before, fileState(t, path))

	// as if an older build with the same schema version had opened it last
	older := database.AppVersion{Version: "v0.33.0", GitHash: "aaaaaaa"}
	olderAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	wrote, err := database.StampAppVersion(ctx, db.writeDB, older, olderAt)
	require.NoError(t, err)
	assert.True(t, wrote)
	require.NoError(t, db.Close())

	// a newer build, whose schema is one ahead, opens it: the database needs a
	// migration, so the older build's stamp stays for the migration to read
	newer, err := openAt(t, path, appSchemaVersion+1)
	require.NoError(t, err)
	m, err := NewMigrator(newer)
	require.NoError(t, err)
	previous, err := database.PreviousAppVersion(ctx, m.conn)
	m.Close()
	require.NoError(t, err)
	assert.Equal(t, "v0.33.0", previous)
	assert.Equal(t, "v0.33.0", readAppVersionRow(t, path).Version)
	require.NoError(t, newer.Close())

	// an older build, whose schema is one behind, is refused before it reads
	// anything, and changes nothing
	_, err = openAt(t, path, appSchemaVersion-1)
	var mismatched *database.MismatchedSchemaVersionError
	require.True(t, errors.As(err, &mismatched), "error %v", err)
	assert.Equal(t, "v0.33.0", readAppVersionRow(t, path).Version)

	// this build opens it: it's stamped, and the older build is kept
	start = time.Now().Add(-time.Second)
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())
	row = readAppVersionRow(t, path)
	assert.Equal(t, current.Version, row.Version)
	assert.Equal(t, current.GitHash, row.GitHash)
	assert.True(t, row.FirstOpenedAt.After(start), "first opened at %v", row.FirstOpenedAt)
	assert.Equal(t, sql.NullString{String: "v0.33.0", Valid: true}, row.PreviousVersion)
	assert.Equal(t, sql.NullString{String: "aaaaaaa", Valid: true}, row.PreviousGitHash)
	assert.True(t, row.PreviousFirstOpenedAt.Valid && row.PreviousFirstOpenedAt.Time.Equal(olderAt), "previous first opened at %v", row.PreviousFirstOpenedAt)
}

// Another connection holding the write lock delays recording the version by
// stampBusyTimeout, not the usual busy timeout, and only costs a warning.
func TestAppVersionStampWaitsBriefly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	t.Cleanup(func() { _ = db.Close() })

	// another build opened it last, so this build needs to write
	_, err := database.StampAppVersion(ctx, db.writeDB, database.AppVersion{Version: "v0.33.0", GitHash: "aaaaaaa"}, time.Now())
	require.NoError(t, err)

	other, err := sqlx.Open(sqlite3Driver, "file:"+path+"?_journal=WAL")
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	lock, err := other.Conn(ctx)
	require.NoError(t, err)
	_, err = lock.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = lock.ExecContext(ctx, "ROLLBACK")
		_ = lock.Close()
	})

	warnings := captureWarnings(t)
	started := time.Now()
	db.stampAppVersion()
	elapsed := time.Since(started)

	assert.Less(t, elapsed, busyTimeout-time.Second, "waited %v", elapsed)
	assert.Contains(t, warnings.all(), "recording the Stash version")
	assert.Equal(t, "v0.33.0", readAppVersionRow(t, path).Version)

	// the connection is back to the usual wait
	var timeout int64
	require.NoError(t, db.writeDB.Get(&timeout, "PRAGMA busy_timeout"))
	assert.Equal(t, busyTimeout.Milliseconds(), timeout)
}

// A post-migration can read the previous version. A database from before the
// version was recorded gives "".
func TestAppVersionDuringMigration(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = filepath.Join(t.TempDir(), "stash-go.sqlite")
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	for version := uint(1); version < appSchemaVersion; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}

	saved := postMigrations[appSchemaVersion]
	t.Cleanup(func() { postMigrations[appSchemaVersion] = saved })
	var previous string
	var readErr error
	ran := false
	postMigrations[appSchemaVersion] = append(append([]customMigrationFunc{}, saved...), func(ctx context.Context, conn *sqlx.DB) error {
		ran = true
		previous, readErr = database.PreviousAppVersion(ctx, conn)
		return readErr
	})
	require.NoError(t, m.RunMigration(ctx, appSchemaVersion))
	require.True(t, ran)
	require.NoError(t, readErr)
	assert.Equal(t, "", previous)
}
