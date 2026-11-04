//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
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

func readAppVersionRow(t *testing.T, dsn string) appVersionRow {
	t.Helper()
	conn, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer conn.Close()
	var rows []appVersionRow
	require.NoError(t, conn.Select(&rows, "SELECT version, githash, first_opened_at, previous_version, previous_githash, previous_first_opened_at FROM app_version"))
	require.Len(t, rows, 1)
	return rows[0]
}

func openAt(t *testing.T, dsn string, schemaVersion uint) (*Database, error) {
	t.Helper()
	saved := appSchemaVersion
	appSchemaVersion = schemaVersion
	t.Cleanup(func() { appSchemaVersion = saved })
	db := NewDatabase()
	err := db.Open(context.Background(), dsn)
	appSchemaVersion = saved
	return db, err
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
	dsn := migrationFixture(t)
	// the migrations' post-migrations read the config
	config.InitializeEmpty()
	current := database.CurrentAppVersion()

	// a new database
	start := time.Now().Add(-time.Second)
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	require.NoError(t, db.Close())
	row := readAppVersionRow(t, dsn)
	assert.Equal(t, current.Version, row.Version)
	assert.Equal(t, current.GitHash, row.GitHash)
	assert.True(t, row.FirstOpenedAt.After(start), "first opened at %v", row.FirstOpenedAt)
	assert.False(t, row.PreviousVersion.Valid)

	// opened again by the same build: the row is left as it was
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	require.NoError(t, db.Close())
	assert.Equal(t, row, readAppVersionRow(t, dsn))

	// as if an older build with the same schema version had opened it last
	older := database.AppVersion{Version: "v0.33.0", GitHash: "aaaaaaa"}
	olderAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	wrote, err := database.StampAppVersion(ctx, db.writeDB, older, olderAt)
	require.NoError(t, err)
	assert.True(t, wrote)
	require.NoError(t, db.Close())

	// a newer build, whose schema is one ahead, opens it: the database needs a
	// migration, so the older build's stamp stays for the migration to read
	newer, err := openAt(t, dsn, appSchemaVersion+1)
	require.NoError(t, err)
	m, err := NewMigrator(newer)
	require.NoError(t, err)
	previous, err := database.PreviousAppVersion(ctx, m.conn)
	m.Close()
	require.NoError(t, err)
	assert.Equal(t, "v0.33.0", previous)
	assert.Equal(t, "v0.33.0", readAppVersionRow(t, dsn).Version)
	require.NoError(t, newer.Close())

	// an older build, whose schema is one behind, is refused before it reads
	// anything, and changes nothing
	_, err = openAt(t, dsn, appSchemaVersion-1)
	var mismatched *database.MismatchedSchemaVersionError
	require.True(t, errors.As(err, &mismatched), "error %v", err)
	assert.Equal(t, "v0.33.0", readAppVersionRow(t, dsn).Version)

	// this build opens it: it's stamped, and the older build is kept
	start = time.Now().Add(-time.Second)
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	require.NoError(t, db.Close())
	row = readAppVersionRow(t, dsn)
	assert.Equal(t, current.Version, row.Version)
	assert.Equal(t, current.GitHash, row.GitHash)
	assert.True(t, row.FirstOpenedAt.After(start), "first opened at %v", row.FirstOpenedAt)
	assert.Equal(t, sql.NullString{String: "v0.33.0", Valid: true}, row.PreviousVersion)
	assert.Equal(t, sql.NullString{String: "aaaaaaa", Valid: true}, row.PreviousGitHash)
	assert.True(t, row.PreviousFirstOpenedAt.Valid && row.PreviousFirstOpenedAt.Time.Equal(olderAt), "previous first opened at %v", row.PreviousFirstOpenedAt)
}

// Another session holding the row makes opening wait at most
// database.AppVersionStampTimeout to record the version, with a warning.
func TestAppVersionStampWaitsBriefly(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, dsn))
	// another build opened it last, so the next open needs to write
	_, err := database.StampAppVersion(ctx, db.writeDB, database.AppVersion{Version: "v0.33.0", GitHash: "aaaaaaa"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, db.Close())

	other, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	lock, err := other.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = lock.ExecContext(ctx, "SELECT * FROM app_version WHERE id = 1 FOR UPDATE")
	require.NoError(t, err)

	warnings := captureWarnings(t)
	db = NewDatabase()
	opened := make(chan error, 1)
	started := time.Now()
	go func() { opened <- db.Open(ctx, dsn) }()

	var openErr error
	select {
	case openErr = <-opened:
	case <-time.After(database.AppVersionStampTimeout + 10*time.Second):
	}
	elapsed := time.Since(started)
	// let a hung open finish, so the test doesn't leave it behind
	require.NoError(t, lock.Rollback())
	if elapsed >= database.AppVersionStampTimeout+10*time.Second {
		<-opened
		t.Fatalf("opening waited more than %v for the locked row", elapsed)
	}
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, openErr)
	assert.Less(t, elapsed, database.AppVersionStampTimeout+3*time.Second, "waited %v", elapsed)
	assert.Contains(t, warnings.all(), "recording the Stash version")
	assert.Equal(t, "v0.33.0", readAppVersionRow(t, dsn).Version)
	require.NoError(t, db.Ready())
}

// A post-migration can read the previous version. A database from before the
// version was recorded gives "".
func TestAppVersionDuringMigration(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase()
	db.dbPath = migrationFixture(t)
	config.InitializeEmpty()
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
