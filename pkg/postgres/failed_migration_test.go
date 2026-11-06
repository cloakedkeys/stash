//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"

	postgresmig "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appVersionMigration is the schema version that creates app_version. The
// tests here make it fail, so they don't depend on which migration is last.
const appVersionMigration uint = 27

func readMigrationVersion(t *testing.T, dsn string) (version int, dirty bool) {
	t.Helper()
	conn, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	return version, dirty
}

// A migration that fails is not counted as done when Stash starts again: the
// version goes back to the one before, and the migration runs again (#2195).
func TestFailedMigrationRunsAgain(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	last := appVersionMigration
	for version := uint(1); version < last; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}

	// a table already called app_version makes the migration that creates it fail
	_, err = m.conn.Exec("CREATE TABLE app_version (blocker INTEGER)")
	require.NoError(t, err)
	require.Error(t, m.RunMigration(ctx, last))
	m.Close()
	version, dirty := readMigrationVersion(t, dsn)
	require.Equal(t, int(last), version)
	require.True(t, dirty, "golang-migrate leaves the failed version dirty")

	// starting again doesn't count the failed migration as done
	restarted := NewDatabase()
	require.NoError(t, restarted.Open(ctx, dsn))
	assert.Equal(t, last-1, restarted.Version())
	var needed *database.MigrationNeededError
	require.ErrorAs(t, restarted.Ready(), &needed)
	assert.Equal(t, last-1, needed.CurrentSchemaVersion)
	require.NoError(t, restarted.Close())
	version, dirty = readMigrationVersion(t, dsn)
	assert.Equal(t, int(last-1), version)
	assert.False(t, dirty)

	// once the cause is gone the migration runs again and finishes
	m, err = NewMigrator(restarted)
	require.NoError(t, err)
	defer m.Close()
	_, err = m.conn.Exec("DROP TABLE app_version")
	require.NoError(t, err)
	require.NoError(t, m.RunMigration(ctx, last))
	_, err = m.conn.Exec("SELECT first_opened_at FROM app_version")
	require.NoError(t, err)
	version, dirty = readMigrationVersion(t, dsn)
	assert.Equal(t, int(last), version)
	assert.False(t, dirty)
}

// Trying the upgrade again without restarting (the migrate job builds a new
// migrator each time) doesn't count the failed migration as done either.
func TestFailedMigrationNewMigrator(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	last := appVersionMigration
	for version := uint(1); version < last; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	_, err = m.conn.Exec("CREATE TABLE app_version (blocker INTEGER)")
	require.NoError(t, err)
	require.Error(t, m.RunMigration(ctx, last))
	m.Close()

	m, err = NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	assert.Equal(t, last-1, m.CurrentSchemaVersion())
}

// A migration whose first schema version fails leaves no version, so a new
// database is built from the start next time.
func TestFailedFirstMigration(t *testing.T) {
	dsn := migrationFixture(t)
	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	_, err = m.conn.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (1, true)")
	require.NoError(t, err)
	m.Close()

	m, err = NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	assert.Equal(t, uint(0), m.CurrentSchemaVersion())
	conn, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer conn.Close()
	var rows int
	require.NoError(t, conn.Get(&rows, "SELECT count(*) FROM schema_migrations"))
	assert.Equal(t, 0, rows)
}

// Setting the version back is only safe because a failed migration keeps
// none of its changes: the driver, set up as getMigrate sets it up, sends
// each file as one query, which PostgreSQL runs as one transaction.
func TestFailedMigrationKeepsNoChanges(t *testing.T) {
	dsn := migrationFixture(t)
	db := NewDatabase()
	db.dbPath = dsn
	conn, err := db.open(true)
	require.NoError(t, err)
	defer conn.Close()
	driver, err := postgresmig.WithInstance(conn.DB, &postgresmig.Config{})
	require.NoError(t, err)
	defer driver.Close()

	err = driver.Run(strings.NewReader("CREATE TABLE kept_by_mistake (id INTEGER);\nINSERT INTO kept_by_mistake VALUES (1);\nSELECT 1/0;"))
	require.Error(t, err)
	var exists bool
	require.NoError(t, conn.Get(&exists, "SELECT to_regclass('kept_by_mistake') IS NOT NULL"))
	assert.False(t, exists, "the first statements of a failed migration were kept")
}
