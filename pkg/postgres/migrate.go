package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	postgresmig "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/database/migrationguard"
	"github.com/stashapp/stash/pkg/logger"
)

func (db *Database) needsMigration() bool {
	return db.schemaVersion != appSchemaVersion
}

type Migrator struct {
	db     *Database
	conn   *sqlx.DB
	m      *migrate.Migrate
	source source.Driver
	driver migratedb.Driver

	// unlockUpgrade releases the lock that RunMigration takes the first time
	// it is called and holds until Close, so that a second Stash waits for
	// the whole upgrade rather than running part of it (#2213)
	unlockUpgrade func()
	// waitedForUpgrade is set when another Stash held that lock first. This
	// one then only takes up what the other did, and runs no migration
	// itself.
	waitedForUpgrade bool
}

func NewMigrator(db *Database) (*Migrator, error) {
	m := &Migrator{
		db: db,
	}

	const writable = true
	var err error
	m.conn, err = m.db.open(writable)
	if err != nil {
		return nil, err
	}

	m.conn.SetMaxOpenConns(maxReadConnections)
	m.conn.SetMaxIdleConns(maxReadConnections)
	m.conn.SetConnMaxIdleTime(dbConnTimeout)

	m.m, err = m.getMigrate()
	if err == nil {
		err = m.resetFailedMigration()
	}

	// if error encountered, close the connection
	if err != nil {
		m.Close()
	}

	return m, err
}

func (m *Migrator) Close() {
	if m.unlockUpgrade != nil {
		m.unlockUpgrade()
		m.unlockUpgrade = nil
	}
	if m.m != nil {
		m.m.Close()
		m.m = nil
	}
	// closing m.m closes the connection too, but not if it failed to start
	if m.conn != nil {
		_ = m.conn.Close()
	}
}

// KeepUpgradeLock hands over the lock that RunMigration took, so that Close
// doesn't release it. The upgrade task keeps it until it has put a backup
// back, or a second Stash could take the upgrade as done while the restore
// runs (review of #2232). The returned function releases it, and does
// nothing if there is no lock.
func (m *Migrator) KeepUpgradeLock() (release func()) {
	unlock := m.unlockUpgrade
	m.unlockUpgrade = nil
	if unlock == nil {
		return func() {}
	}
	return unlock
}

func (m *Migrator) CurrentSchemaVersion() uint {
	databaseSchemaVersion, _, _ := m.m.Version()
	return databaseSchemaVersion
}

func (m *Migrator) RequiredSchemaVersion() uint {
	return appSchemaVersion
}

func (m *Migrator) getMigrate() (*migrate.Migrate, error) {
	migrations, err := iofs.New(migrationsBox, "migrations")
	if err != nil {
		return nil, err
	}

	driver, err := postgresmig.WithInstance(m.conn.DB, &postgresmig.Config{})
	if err != nil {
		return nil, err
	}
	m.source = migrations
	m.driver = driver

	// use sqlite3Driver so that migration has access to durationToTinyInt
	return migrate.NewWithInstance(
		"iofs",
		cleanMarkSource{migrations},
		"postgres",
		driver,
	)
}

// cleanMarkSource adds, to the end of each migration file, the statement that
// marks its version finished (#2202). The file runs as one transaction, so
// the mark is kept together with the file's changes or not at all, and a
// version that is still marked unfinished always means none of its changes
// were kept. golang-migrate marks the version finished again in a separate
// transaction afterwards, which changes nothing.
//
// A version with post migrations also records, in the same transaction, that
// they are still to run (#2200). runPostMigrations removes the record once
// they have finished.
type cleanMarkSource struct {
	source.Driver
}

func (s cleanMarkSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	r, identifier, err := s.Driver.ReadUp(version)
	if err != nil {
		return r, identifier, err
	}
	// on a new line, in case the file ends in a comment
	mark := "\n;"
	if len(postMigrations[version]) > 0 {
		// migration 30 adds the table for every database; migrations before it
		// that need it create it themselves
		mark += fmt.Sprintf("CREATE TABLE IF NOT EXISTS %[1]s (version INTEGER PRIMARY KEY);\nINSERT INTO %[1]s (version) VALUES (%[2]d) ON CONFLICT DO NOTHING;\n", pendingPostMigrationsTable, version)
	}
	mark += fmt.Sprintf("UPDATE %s SET dirty = false WHERE version = %d;\n", postgresmig.DefaultMigrationsTable, version)
	return readCloser{io.MultiReader(r, strings.NewReader(mark)), r}, identifier, nil
}

// pendingPostMigrationsTable lists the schema versions whose post migrations
// haven't finished (migration 30).
const pendingPostMigrationsTable = "pending_post_migrations"

// runPostMigrations runs the post migrations of version, then removes the
// record that they are still to run. If one fails, the record stays, so they
// run again before the database is next used (runPendingPostMigrations).
func (m *Migrator) runPostMigrations(ctx context.Context, version uint) error {
	fns := postMigrations[version]
	if len(fns) == 0 {
		return nil
	}
	if err := m.runCustomMigrations(ctx, fns); err != nil {
		return err
	}
	_, err := m.conn.ExecContext(ctx, "DELETE FROM "+pendingPostMigrationsTable+" WHERE version = $1", version)
	return err
}

// lockPostMigrations takes the lock that is held from before a migration's
// SQL until its post migrations have finished, and while post migrations
// that failed run again. A record in pending_post_migrations is written when
// the SQL commits, which also releases golang-migrate's own lock, so without
// this a second Stash process could run the same post migration while the
// first is still running it (review of #2220). It is a session lock on one
// connection of its own, with a key of its own. The returned function
// releases it; waited says whether another Stash held it first.
//
// The connection has a pool of its own, so the lock can outlive the
// migrator's connections (KeepUpgradeLock). Closing that pool ends the
// session, which releases the lock if unlocking failed.
func (m *Migrator) lockPostMigrations(ctx context.Context) (unlock func(), waited bool, err error) {
	const writable = true
	lockDB, err := m.db.open(writable)
	if err != nil {
		return nil, false, err
	}
	lockDB.SetMaxOpenConns(1)
	conn, err := lockDB.Connx(ctx)
	if err != nil {
		_ = lockDB.Close()
		return nil, false, err
	}
	fail := func(err error) (func(), bool, error) {
		_ = conn.Close()
		_ = lockDB.Close()
		return nil, false, err
	}
	var dbName, schema string
	if err := conn.QueryRowxContext(ctx, "SELECT current_database(), current_schema()").Scan(&dbName, &schema); err != nil {
		return fail(err)
	}
	id, err := migratedb.GenerateAdvisoryLockId(dbName, schema, pendingPostMigrationsTable)
	if err != nil {
		return fail(err)
	}
	var got bool
	if err := conn.GetContext(ctx, &got, "SELECT pg_try_advisory_lock($1)", id); err != nil {
		return fail(err)
	}
	if !got {
		logger.Info("Waiting until no other Stash is upgrading this database.")
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", id); err != nil {
			// the lock may have been granted as the wait was cut off; closing
			// the pool ends the session
			return fail(err)
		}
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", id)
		_ = conn.Close()
		_ = lockDB.Close()
	}, !got, nil
}

// pendingPostMigrations lists the versions whose post migrations failed last
// time, oldest first. A newer build's own are left for it.
func (m *Migrator) pendingPostMigrations(ctx context.Context) ([]uint, error) {
	var exists bool
	if err := m.conn.GetContext(ctx, &exists, "SELECT to_regclass($1) IS NOT NULL", pendingPostMigrationsTable); err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	var versions []uint
	if err := m.conn.SelectContext(ctx, &versions, "SELECT version FROM "+pendingPostMigrationsTable+" WHERE version <= $1 ORDER BY version", appSchemaVersion); err != nil {
		return nil, err
	}
	return versions, nil
}

// runPendingPostMigrations runs again the post migrations that failed last
// time, oldest first (#2200). Until they have finished, the upgrade isn't
// done: the database isn't used, and no later migration runs. A database
// with none takes no lock.
func (m *Migrator) runPendingPostMigrations(ctx context.Context) error {
	versions, err := m.pendingPostMigrations(ctx)
	if err != nil || len(versions) == 0 {
		return err
	}
	if m.unlockUpgrade != nil {
		// RunMigration holds the lock already
		return m.runPendingPostMigrationsLocked(ctx)
	}
	unlock, _, err := m.lockPostMigrations(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return m.runPendingPostMigrationsLocked(ctx)
}

// runPendingPostMigrationsLocked is runPendingPostMigrations with the lock
// held. It reads the records again, as another Stash that held the lock may
// have finished them.
//
// Each post migration changes nothing when run again after it has finished,
// so a record left by a failure to remove it is harmless. A record also
// waits through a downgrade: if an older build is used in between, running
// the post migration of version 26 then rewrites the filters saved meanwhile
// too.
func (m *Migrator) runPendingPostMigrationsLocked(ctx context.Context) error {
	versions, err := m.pendingPostMigrations(ctx)
	if err != nil {
		return err
	}
	for _, version := range versions {
		logger.Warnf("The follow-up step of the database upgrade to schema version %d didn't finish last time. Running it again.", version)
		if err := m.runCustomMigrations(ctx, postMigrations[version]); err != nil {
			return fmt.Errorf("the follow-up step of the database upgrade to schema version %d failed again, so Stash can't use the database until it succeeds. Fix the cause and start Stash again, or restore the database from a backup made before the upgrade: %w", version, err)
		}
		if _, err := m.conn.ExecContext(ctx, "DELETE FROM "+pendingPostMigrationsTable+" WHERE version = $1", version); err != nil {
			return err
		}
		logger.Infof("The follow-up step of the database upgrade to schema version %d has finished.", version)
	}
	return nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

// resetFailedMigration sets the schema version back to the last finished one
// when a migration failed part way (#2195).
//
// golang-migrate marks the new version "dirty" before running a migration's
// SQL, and the SQL itself clears the mark (cleanMarkSource), so a dirty
// version is one whose SQL failed or was cut off. The driver sends each
// migration file as a single query, and PostgreSQL runs a query of several
// statements as one transaction (none of the files has its own BEGIN or
// COMMIT), so a failed migration leaves none of its changes behind: the
// database is still at the previous version. Setting the version back lets
// the migration run again, rather than counting it as done.
//
// Builds before #2202 cleared the mark in a transaction of its own, so a
// database they upgraded can, rarely, be dirty with the changes kept. The
// warning says what to do if the upgrade then fails.
func (m *Migrator) resetFailedMigration() error {
	version, dirty, err := m.driver.Version()
	if err != nil || !dirty {
		return err
	}

	// a migration that is running holds this lock until it has finished, and
	// its version is dirty until then, so wait for it and look again
	logger.Infof("The database is marked as part way through the upgrade to schema version %d. Waiting until no other upgrade of it is running.", version)
	if err := m.driver.Lock(); err != nil {
		return err
	}
	version, dirty, err = m.driver.Version()
	if err == nil && dirty {
		previous := migratedb.NilVersion
		prev, prevErr := m.source.Prev(uint(version))
		switch {
		case prevErr == nil:
			previous = int(prev)
		case !errors.Is(prevErr, fs.ErrNotExist):
			err = fmt.Errorf("finding the schema version before %d: %w", version, prevErr)
		}
		if err == nil {
			logger.Warnf("The upgrade of the database to schema version %[1]d didn't finish. Setting the schema version back to %[2]d so that the upgrade runs again. If it then fails because something it creates already exists, an older build of Stash may have kept the changes of version %[1]d without recording them; if so, record them with \"UPDATE schema_migrations SET version = %[1]d, dirty = false\" and start Stash again.", version, max(previous, 0))
			err = m.driver.SetVersion(previous, false)
		}
	}
	if unlockErr := m.driver.Unlock(); err == nil {
		err = unlockErr
	}
	return err
}

func (m *Migrator) RunMigration(ctx context.Context, newVersion uint) error {
	// held until Close, so through the rest of the upgrade and its post
	// migrations; the version is read after taking it, so a migration another
	// Stash ran meanwhile shows
	if m.unlockUpgrade == nil {
		unlock, waited, err := m.lockPostMigrations(ctx)
		if err != nil {
			return err
		}
		m.unlockUpgrade = unlock
		m.waitedForUpgrade = waited
	}

	databaseSchemaVersion, dirty, _ := m.m.Version()
	m.db.schemaVersion = databaseSchemaVersion

	// A Stash that waited for another takes up only an upgrade the other
	// finished: every version in place, none part way through its SQL, and
	// no follow-up step left to run (#2213, review of #2232).
	if m.waitedForUpgrade {
		pending, err := m.pendingPostMigrations(ctx)
		if err != nil {
			return err
		}
		if dirty || len(pending) > 0 || newVersion > databaseSchemaVersion {
			return fmt.Errorf("%w: it is at schema version %d. Start Stash again to finish the upgrade", database.ErrUpgradeUnfinishedElsewhere, databaseSchemaVersion)
		}
	}

	// another Stash ran it, after this one read the version
	if newVersion <= databaseSchemaVersion {
		logger.Infof("Schema version %d is already in place: another Stash upgraded the database.", newVersion)
		return nil
	}

	if newVersion != databaseSchemaVersion+1 {
		return fmt.Errorf("invalid migration version %d, expected %d", newVersion, databaseSchemaVersion+1)
	}

	// a post migration that failed last time comes before the next migration
	if err := m.runPendingPostMigrationsLocked(ctx); err != nil {
		return err
	}

	// run pre migrations as needed
	if err := m.runCustomMigrations(ctx, preMigrations[newVersion]); err != nil {
		return fmt.Errorf("running pre migrations for schema version %d: %w", newVersion, err)
	}

	if err := m.m.Steps(1); err != nil {
		// Without the lock this migrator ran nothing, so a finished version
		// is another one's work, and so are its post migrations.
		if errors.Is(err, migrate.ErrLockTimeout) || errors.Is(err, migrate.ErrLocked) || errors.Is(err, migratedb.ErrLocked) {
			return err
		}
		// The SQL marks its version finished in its own transaction
		// (cleanMarkSource), so if that is recorded, only golang-migrate's
		// own, later, write of the same mark failed: the migration is done
		// and its post migrations still have to run.
		if version, dirty, verr := m.driver.Version(); verr != nil || dirty || version != int(newVersion) {
			// migration failed
			return err
		}
		logger.Warnf("Schema version %d was applied, but recording it afterwards failed, which changes nothing: %v", newVersion, err)
	}

	// run post migrations as needed
	if err := m.runPostMigrations(ctx, newVersion); err != nil {
		return fmt.Errorf("running post migrations for schema version %d: %w", newVersion, err)
	}

	// update the schema version
	m.db.schemaVersion, _, _ = m.m.Version()

	return nil
}

func (m *Migrator) runCustomMigrations(ctx context.Context, fns []customMigrationFunc) error {
	for _, fn := range fns {
		if err := m.runCustomMigration(ctx, fn); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrator) runCustomMigration(ctx context.Context, fn customMigrationFunc) error {
	if err := fn(ctx, m.conn); err != nil {
		return err
	}

	return nil
}

func (m *Migrator) PostMigrate(ctx context.Context) error {
	// optimise the database
	var err error
	logger.Info("Running database analyze")

	// don't use Optimize/vacuum as this adds a significant amount of time
	// to the migration
	err = analyze(ctx, m.conn)

	if err != nil {
		return fmt.Errorf("error optimising database: %s", err)
	}

	return nil
}

func (db *Database) runPendingPostMigrations(ctx context.Context) error {
	m, err := NewMigrator(db)
	if err != nil {
		return err
	}
	defer m.Close()
	return m.runPendingPostMigrations(ctx)
}

func (db *Database) getDatabaseSchemaVersion() (uint, error) {
	m, err := NewMigrator(db)
	if err != nil {
		return 0, err
	}
	defer m.Close()

	ret, _, _ := m.m.Version()
	return ret, nil
}

func (db *Database) ReInitialise() error {
	return db.initialise()
}

// RunAllMigrations runs all migrations to bring the database up to the current schema version
func (db *Database) RunAllMigrations(ctx context.Context) error {

	m, err := NewMigrator(db)
	if err != nil {
		return err
	}
	defer m.Close()

	databaseSchemaVersion, _, _ := m.m.Version()
	stepNumber := appSchemaVersion - databaseSchemaVersion
	if stepNumber != 0 {
		// a stop waits for this to finish, so the database isn't left half built
		kind := migrationguard.Update
		if databaseSchemaVersion == 0 {
			kind = migrationguard.Build
		}
		end, err := migrationguard.Begin(kind)
		if err != nil {
			return err
		}
		defer end()

		logger.Infof("Migrating database from version %d to %d", databaseSchemaVersion, appSchemaVersion)

		// run each migration individually, and run custom migrations as needed
		var i uint = 1
		for ; i <= stepNumber; i++ {
			newVersion := databaseSchemaVersion + i
			if err := m.RunMigration(ctx, newVersion); err != nil {
				return err
			}
		}
	}

	return nil
}
