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
type cleanMarkSource struct {
	source.Driver
}

func (s cleanMarkSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	r, identifier, err := s.Driver.ReadUp(version)
	if err != nil {
		return r, identifier, err
	}
	// on a new line, in case the file ends in a comment
	mark := fmt.Sprintf("\n;UPDATE %s SET dirty = false WHERE version = %d;\n", postgresmig.DefaultMigrationsTable, version)
	return readCloser{io.MultiReader(r, strings.NewReader(mark)), r}, identifier, nil
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
	databaseSchemaVersion, _, _ := m.m.Version()

	if newVersion != databaseSchemaVersion+1 {
		return fmt.Errorf("invalid migration version %d, expected %d", newVersion, databaseSchemaVersion+1)
	}

	// run pre migrations as needed
	if err := m.runCustomMigrations(ctx, preMigrations[newVersion]); err != nil {
		return fmt.Errorf("running pre migrations for schema version %d: %w", newVersion, err)
	}

	if err := m.m.Steps(1); err != nil {
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
	if err := m.runCustomMigrations(ctx, postMigrations[newVersion]); err != nil {
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
