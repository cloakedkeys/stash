//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type appVersionRow struct {
	Version          string         `db:"version"`
	GitHash          string         `db:"githash"`
	OpenedAt         time.Time      `db:"opened_at"`
	PreviousVersion  sql.NullString `db:"previous_version"`
	PreviousGitHash  sql.NullString `db:"previous_githash"`
	PreviousOpenedAt sql.NullTime   `db:"previous_opened_at"`
}

func readAppVersionRow(t *testing.T, path string) appVersionRow {
	t.Helper()
	conn, err := sqlx.Open("pgx", path)
	require.NoError(t, err)
	defer conn.Close()
	var rows []appVersionRow
	require.NoError(t, conn.Select(&rows, "SELECT version, githash, opened_at, previous_version, previous_githash, previous_opened_at FROM app_version"))
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

// Each time a database is opened at the current schema version, the build
// that opened it is recorded, and the version before is kept (#2057).
func TestAppVersionStamp(t *testing.T) {
	ctx := context.Background()
	path := migrationFixture(t)
	// the migrations' post-migrations read the config
	config.InitializeEmpty()
	current := database.CurrentAppVersion()

	// a new database
	start := time.Now().Add(-time.Second)
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	row := readAppVersionRow(t, path)
	assert.Equal(t, current.Version, row.Version)
	assert.Equal(t, current.GitHash, row.GitHash)
	assert.True(t, row.OpenedAt.After(start), "opened at %v", row.OpenedAt)
	assert.False(t, row.PreviousVersion.Valid)

	// as if an older build with the same schema version had opened it last
	older := database.AppVersion{Version: "v0.33.0", GitHash: "aaaaaaa"}
	olderAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, database.StampAppVersion(ctx, db.writeDB, older, olderAt))
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
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	row = readAppVersionRow(t, path)
	assert.Equal(t, current.Version, row.Version)
	assert.Equal(t, current.GitHash, row.GitHash)
	assert.Equal(t, sql.NullString{String: "v0.33.0", Valid: true}, row.PreviousVersion)
	assert.Equal(t, sql.NullString{String: "aaaaaaa", Valid: true}, row.PreviousGitHash)
	assert.True(t, row.PreviousOpenedAt.Valid && row.PreviousOpenedAt.Time.Equal(olderAt), "previous opened at %v", row.PreviousOpenedAt)
	firstOpen := row.OpenedAt
	require.NoError(t, db.Close())

	// opened again by the same build: only the time changes
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())
	again := readAppVersionRow(t, path)
	assert.Equal(t, row.PreviousVersion, again.PreviousVersion)
	assert.False(t, again.OpenedAt.Before(firstOpen))
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
