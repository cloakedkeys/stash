package sqlite

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	sqlite3mig "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/database/migrationguard"
	"github.com/stashapp/stash/pkg/logger"
)

func (db *Database) needsMigration() bool {
	return db.schemaVersion != appSchemaVersion
}

type Migrator struct {
	db   *Database
	conn *sqlx.DB
	m    *migrate.Migrate
}

func NewMigrator(db *Database) (*Migrator, error) {
	return newMigrator(db, sqlite3Driver)
}

func newMigrator(db *Database, driverName string) (*Migrator, error) {
	m := &Migrator{
		db: db,
	}

	const disableForeignKeys = true
	const writable = true
	var err error
	m.conn, err = m.db.openWithDriver(driverName, disableForeignKeys, writable)
	if err != nil {
		return nil, err
	}

	m.conn.SetMaxOpenConns(maxReadConnections)
	m.conn.SetMaxIdleConns(maxReadConnections)
	m.conn.SetConnMaxIdleTime(dbConnTimeout)

	m.m, err = m.getMigrate()

	// if error encountered, close the connection
	if err != nil {
		m.Close()
	}

	return m, err
}

func (m *Migrator) Close() {
	if m.m != nil {
		m.m.Close()
		m.m = nil
	}
	// closing m.m closes the connection too, but not if it failed to start
	if m.conn != nil {
		_ = m.conn.Close()
	}
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

	driver, err := sqlite3mig.WithInstance(m.conn.DB, &sqlite3mig.Config{})
	if err != nil {
		return nil, err
	}

	// use sqlite3Driver so that migration has access to durationToTinyInt
	return migrate.NewWithInstance(
		"iofs",
		pendingMarkSource{migrations},
		m.db.dbPath,
		driver,
	)
}

// pendingMarkSource adds, to the end of the file of each migration that has
// post migrations, the statement that records they are still to run (#2221).
// The driver runs the whole file in one transaction, so the record is kept
// together with the file's changes or not at all. runPostMigrations removes
// it once they have finished; until then they run again before the database
// is used (runPendingPostMigrations), even after a crash or a backup that
// couldn't be put back.
type pendingMarkSource struct {
	source.Driver
}

func (s pendingMarkSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	r, identifier, err := s.Driver.ReadUp(version)
	if err != nil || len(postMigrations[version]) == 0 {
		return r, identifier, err
	}
	// on a new line, in case the file ends in a comment; migration 110 adds the
	// table for every database, and migrations before it that need it create
	// it themselves
	mark := fmt.Sprintf("\n;CREATE TABLE IF NOT EXISTS `%[1]s` (`version` INTEGER PRIMARY KEY);\nINSERT OR IGNORE INTO `%[1]s` (`version`) VALUES (%[2]d);\n", pendingPostMigrationsTable, version)
	return readCloser{io.MultiReader(r, strings.NewReader(mark)), r}, identifier, nil
}

type readCloser struct {
	io.Reader
	io.Closer
}

// pendingPostMigrationsTable lists the schema versions whose post migrations
// haven't finished (migration 110).
const pendingPostMigrationsTable = "pending_post_migrations"

// runPostMigrations runs the post migrations of version, then removes the
// record that they are still to run. If one fails, the record stays, so they
// run again before the database is next used, unless the migrate job puts
// back the backup it made first, which takes the record away too.
func (m *Migrator) runPostMigrations(ctx context.Context, version uint) error {
	fns := postMigrations[version]
	if len(fns) == 0 {
		return nil
	}
	if err := m.runCustomMigrations(ctx, fns); err != nil {
		return err
	}
	_, err := m.conn.ExecContext(ctx, "DELETE FROM `"+pendingPostMigrationsTable+"` WHERE `version` = ?", version)
	return err
}

// pendingPostMigrations lists the versions whose post migrations didn't
// finish, oldest first. A newer build's own are left for it.
func pendingPostMigrations(ctx context.Context, conn *sqlx.DB) ([]uint, error) {
	var exists bool
	if err := conn.GetContext(ctx, &exists, "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)", pendingPostMigrationsTable); err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	var versions []uint
	if err := conn.SelectContext(ctx, &versions, "SELECT `version` FROM `"+pendingPostMigrationsTable+"` WHERE `version` <= ? ORDER BY `version`", appSchemaVersion); err != nil {
		return nil, err
	}
	return versions, nil
}

// runPendingPostMigrations runs again the post migrations that didn't finish
// last time, oldest first (#2221). Until they have finished, the upgrade
// isn't done: the database isn't used, and no later migration runs.
func (m *Migrator) runPendingPostMigrations(ctx context.Context) error {
	versions, err := pendingPostMigrations(ctx, m.conn)
	if err != nil {
		return err
	}
	for _, version := range versions {
		logger.Warnf("The follow-up step of the database upgrade to schema version %d didn't finish last time. Running it again.", version)
		if err := m.runCustomMigrations(ctx, postMigrations[version]); err != nil {
			return fmt.Errorf("the follow-up step of the database upgrade to schema version %d failed again, so Stash can't use the database until it succeeds. Fix the cause and start Stash again, or restore the database from a backup made before the upgrade: %w", version, err)
		}
		if _, err := m.conn.ExecContext(ctx, "DELETE FROM `"+pendingPostMigrationsTable+"` WHERE `version` = ?", version); err != nil {
			return err
		}
		logger.Infof("The follow-up step of the database upgrade to schema version %d has finished.", version)
	}
	return nil
}

func (m *Migrator) RunMigration(ctx context.Context, newVersion uint) error {
	databaseSchemaVersion, _, _ := m.m.Version()

	if newVersion != databaseSchemaVersion+1 {
		return fmt.Errorf("invalid migration version %d, expected %d", newVersion, databaseSchemaVersion+1)
	}

	// a post migration that didn't finish last time comes before the next migration
	if err := m.runPendingPostMigrations(ctx); err != nil {
		return err
	}

	// run pre migrations as needed
	if err := m.runCustomMigrations(ctx, preMigrations[newVersion]); err != nil {
		return fmt.Errorf("running pre migrations for schema version %d: %w", newVersion, err)
	}

	if err := m.m.Steps(1); err != nil {
		// migration failed
		return err
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
	err = m.db.fullAnalyze(ctx, m.conn)

	if err == nil {
		logger.Debug("Flushing WAL")
		err = flushWAL(ctx, m.conn)
	}

	if err != nil {
		return fmt.Errorf("error optimising database: %s", err)
	}

	return nil
}

// getDatabaseSchemaVersion only reads the version, apart from creating the
// empty version table in a new database, so its connection doesn't run PRAGMA
// optimize on close, which would wait the busy timeout while another
// connection holds the write lock (#2071).
func (db *Database) getDatabaseSchemaVersion() (uint, error) {
	m, err := newMigrator(db, sqlite3DriverNoOptimize)
	if err != nil {
		return 0, err
	}
	defer m.Close()

	ret, _, _ := m.m.Version()
	return ret, nil
}

// runPendingPostMigrations runs the post migrations that didn't finish last
// time. It looks with a connection that only reads, as
// getDatabaseSchemaVersion does, and opens a writable one only if there are
// any.
func (db *Database) runPendingPostMigrations(ctx context.Context) error {
	reader, err := newMigrator(db, sqlite3DriverNoOptimize)
	if err != nil {
		return err
	}
	versions, err := pendingPostMigrations(ctx, reader.conn)
	reader.Close()
	if err != nil || len(versions) == 0 {
		return err
	}

	m, err := NewMigrator(db)
	if err != nil {
		return err
	}
	defer m.Close()
	return m.runPendingPostMigrations(ctx)
}

func (db *Database) ReInitialise() error {
	_ = db.Close()
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
