//go:build integration

package postgres

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
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

// migrateTo runs the migrations up to and including version.
func migrateTo(t *testing.T, m *Migrator, version uint) {
	t.Helper()
	for v := uint(1); v <= version; v++ {
		require.NoErrorf(t, m.RunMigration(context.Background(), v), "schema %d", v)
	}
}

// When only golang-migrate's own "finished" write fails after a migration's
// SQL has committed, the migration still counts as done, its post migrations
// run, and the next start doesn't run it again (#2202). Before, the version
// was left unfinished with the changes kept, and every retry failed because
// what it creates already existed.
func TestCleanMarkCommitsWithMigration(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	target := appVersionMigration
	migrateTo(t, m, target-1)

	// golang-migrate records a finished version with an INSERT; the clean mark
	// in the migration's SQL is an UPDATE, so only golang-migrate's write fails
	_, err = m.conn.Exec(`CREATE FUNCTION refuse_finished() RETURNS trigger AS $$
BEGIN
  IF NOT NEW.dirty THEN
    RAISE EXCEPTION 'refusing to record version % as finished', NEW.version;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER refuse_finished BEFORE INSERT ON schema_migrations FOR EACH ROW EXECUTE FUNCTION refuse_finished();`)
	require.NoError(t, err)

	saved := postMigrations[target]
	t.Cleanup(func() { postMigrations[target] = saved })
	ran := false
	postMigrations[target] = append(append([]customMigrationFunc{}, saved...), func(ctx context.Context, conn *sqlx.DB) error {
		ran = true
		return nil
	})

	require.NoError(t, m.RunMigration(ctx, target))
	assert.True(t, ran, "the post migration ran")
	assert.Equal(t, target, db.schemaVersion)
	version, dirty := readMigrationVersion(t, dsn)
	assert.Equal(t, int(target), version)
	assert.False(t, dirty)

	_, err = m.conn.Exec("DROP TRIGGER refuse_finished ON schema_migrations; DROP FUNCTION refuse_finished()")
	require.NoError(t, err)
	m.Close()

	m, err = NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	assert.Equal(t, target, m.CurrentSchemaVersion())
}

// A migration that fails keeps its version marked unfinished: the clean mark
// added to its SQL rolls back with the rest of it.
func TestCleanMarkRollsBackWithMigration(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	target := appVersionMigration
	migrateTo(t, m, target-1)
	_, err = m.conn.Exec("CREATE TABLE app_version (blocker INTEGER)")
	require.NoError(t, err)

	require.Error(t, m.RunMigration(ctx, target))
	version, dirty := readMigrationVersion(t, dsn)
	assert.Equal(t, int(target), version)
	assert.True(t, dirty)
}

// A migrator that gave up waiting for the lock ran nothing, so it stops, even
// when another one has finished the same version meanwhile: that one runs
// the post migrations, not this one too (review of #2210).
func TestLockTimeoutDoesNotRunOthersPostMigration(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	defer m.Close()
	target := appVersionMigration
	migrateTo(t, m, target-1)

	saved := postMigrations[target]
	t.Cleanup(func() { postMigrations[target] = saved })
	ran := false
	postMigrations[target] = []customMigrationFunc{func(ctx context.Context, conn *sqlx.DB) error {
		ran = true
		return nil
	}}

	// another instance holds the lock while it runs the same migration
	other, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	defer other.Close()
	otherConn, err := other.Connx(ctx)
	require.NoError(t, err)
	defer otherConn.Close()
	var dbName, schema string
	require.NoError(t, otherConn.QueryRowxContext(ctx, "SELECT current_database(), current_schema()").Scan(&dbName, &schema))
	lockID, err := migratedb.GenerateAdvisoryLockId(dbName, schema, postgresmig.DefaultMigrationsTable)
	require.NoError(t, err)
	_, err = otherConn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID)
	require.NoError(t, err)

	const lockTimeout = 500 * time.Millisecond
	m.m.LockTimeout = lockTimeout
	result := make(chan error, 1)
	go func() { result <- m.RunMigration(ctx, target) }()

	// the other instance finishes the migration, and lets go of the lock only
	// after this one has given up waiting
	time.Sleep(lockTimeout / 2)
	body, err := fs.ReadFile(migrationsBox, "migrations/27_app_version.up.sql")
	require.NoError(t, err)
	_, err = otherConn.ExecContext(ctx, string(body)+"\n;UPDATE schema_migrations SET version = 27, dirty = false;")
	require.NoError(t, err)
	time.Sleep(lockTimeout)
	_, err = otherConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockID)
	require.NoError(t, err)

	select {
	case err = <-result:
	case <-time.After(30 * time.Second):
		t.Fatal("RunMigration didn't return")
	}
	assert.ErrorIs(t, err, migrate.ErrLockTimeout)
	assert.False(t, ran, "ran the post migrations of a migration another instance ran")
}

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

// When the latest migration fails, the next start doesn't report the database
// as ready at the new version: the symptom of #2195. The build here is made to
// end at the migration the test makes fail.
func TestFailedLatestMigrationNotReady(t *testing.T) {
	ctx := context.Background()
	dsn := migrationFixture(t)
	config.InitializeEmpty()
	saved := appSchemaVersion
	appSchemaVersion = appVersionMigration
	t.Cleanup(func() { appSchemaVersion = saved })

	db := NewDatabase()
	db.dbPath = dsn
	m, err := NewMigrator(db)
	require.NoError(t, err)
	migrateTo(t, m, appVersionMigration-1)
	_, err = m.conn.Exec("CREATE TABLE app_version (blocker INTEGER)")
	require.NoError(t, err)
	require.Error(t, m.RunMigration(ctx, appVersionMigration))
	m.Close()

	restarted := NewDatabase()
	require.NoError(t, restarted.Open(ctx, dsn))
	defer restarted.Close()
	assert.Equal(t, appVersionMigration-1, restarted.Version())
	var needed *database.MigrationNeededError
	require.ErrorAs(t, restarted.Ready(), &needed, "a failed latest migration counts as done")
	assert.Equal(t, appVersionMigration-1, needed.CurrentSchemaVersion)
	assert.Equal(t, appVersionMigration, needed.RequiredSchemaVersion)
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
