package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	sqlite3mig "github.com/golang-migrate/migrate/v4/database/sqlite3"
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

	// unlockUpgrade releases the upgrade lock (lockUpgrade), which
	// RunMigration takes the first time it is called and holds until Close,
	// so that a second Stash waits for the whole upgrade rather than running
	// part of it (#2213)
	unlockUpgrade func()
	// waitedForUpgrade is set when another Stash held that lock first. This
	// one then only takes up what the other did, and runs no migration
	// itself.
	waitedForUpgrade bool
}

func NewMigrator(db *Database) (*Migrator, error) {
	m, err := newMigrator(db, sqlite3Driver)
	if err != nil {
		return m, err
	}
	if err := m.resetFailedMigration(); err != nil {
		m.Close()
		return m, err
	}
	return m, nil
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

// KeepUpgradeLock hands over the upgrade lock that RunMigration took, so that
// Close doesn't release it. The upgrade task keeps it until it has put a
// backup back, or a second Stash could take the upgrade as done while the
// restore runs (#2232). The returned function releases it, and does nothing
// if there is no lock.
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

	driver, err := sqlite3mig.WithInstance(m.conn.DB, &sqlite3mig.Config{})
	if err != nil {
		return nil, err
	}
	m.source = migrations
	m.driver = driver

	// use sqlite3Driver so that migration has access to durationToTinyInt
	return migrate.NewWithInstance(
		"iofs",
		markSource{migrations},
		m.db.dbPath,
		driver,
	)
}

// markSource adds, to the end of each migration file, the statement that
// marks its version finished (#2230), and for a migration that has post
// migrations, the statement that records they are still to run (#2221).
//
// golang-migrate marks the version dirty before running the file, and
// clean again in a transaction of its own afterwards. The driver runs the
// whole file in one transaction, so with the mark in it, a version that is
// still dirty always means none of the file's changes were kept
// (resetFailedMigration). golang-migrate's own write of the mark afterwards
// changes nothing.
//
// runPostMigrations removes the record of post migrations once they have
// finished; until then they run again before the database is used
// (RunPendingPostMigrations), even after a crash or a backup that couldn't be
// put back.
type markSource struct {
	source.Driver
}

func (s markSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	r, identifier, err := s.Driver.ReadUp(version)
	if err != nil {
		return r, identifier, err
	}
	// on a new line, in case the file ends in a comment
	mark := "\n;"
	if len(postMigrations[version]) > 0 {
		// migration 110 adds the table for every database, and migrations
		// before it that need it create it themselves
		mark += fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%[1]s` (`version` INTEGER PRIMARY KEY);\nINSERT OR IGNORE INTO `%[1]s` (`version`) VALUES (%[2]d);\n", pendingPostMigrationsTable, version)
	}
	mark += fmt.Sprintf("UPDATE `%s` SET `dirty` = 0 WHERE `version` = %d;\n", sqlite3mig.DefaultMigrationsTable, version)
	return readCloser{io.MultiReader(r, strings.NewReader(mark)), r}, identifier, nil
}

// resetFailedMigration sets the schema version back to the last finished one
// when a migration was stopped part way (#2230).
//
// golang-migrate marks the new version dirty before running a migration's
// SQL, and the SQL itself clears the mark (markSource), so a dirty version is
// one whose SQL failed or was cut off. The SQL runs as one transaction, so it
// left none of its changes behind: the database is still at the previous
// version. Setting the version back lets the migration run again, rather
// than every later upgrade failing because the version is dirty.
//
// Builds before #2230 cleared the mark in a transaction of its own, so a
// database they upgraded can, rarely, be dirty with the changes kept. The
// warning says what to do if the upgrade then fails.
//
// Another Stash may be running that migration right now. It holds the upgrade
// lock from before it marks the version dirty until after its SQL is saved,
// so this waits for that lock, reads the version again, and only sets it back
// if it is still dirty (#2213). Without the lock file, its SQL's write lock
// still covers the time the SQL runs (#2230).
func (m *Migrator) resetFailedMigration() error {
	_, dirty, err := m.driver.Version()
	if err != nil || !dirty {
		return err
	}

	if m.unlockUpgrade == nil {
		unlock, _, err := m.db.lockUpgrade(context.Background())
		if err != nil {
			return err
		}
		defer unlock()
	}

	// the connection begins transactions with BEGIN IMMEDIATE
	tx, err := m.conn.Beginx()
	if err != nil {
		return fmt.Errorf("resetting the schema version: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var version int
	query := fmt.Sprintf("SELECT `version`, `dirty` FROM `%s` LIMIT 1", sqlite3mig.DefaultMigrationsTable)
	switch err := tx.QueryRow(query).Scan(&version, &dirty); {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("reading the schema version: %w", err)
	case !dirty:
		// the other Stash finished the migration
		return nil
	}

	previous := migratedb.NilVersion
	prev, err := m.source.Prev(uint(version))
	switch {
	case err == nil:
		previous = int(prev)
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("finding the schema version before %d: %w", version, err)
	}
	logger.Warnf("The upgrade of the database to schema version %[1]d didn't finish. Setting the schema version back to %[2]d so that the upgrade runs again. If it then fails because something it creates already exists, an older build of Stash may have kept the changes of version %[1]d without recording them; if so, record them with \"UPDATE schema_migrations SET version = %[1]d, dirty = 0\" and start Stash again.", version, max(previous, 0))
	// as golang-migrate's SetVersion does, but in this transaction
	if _, err := tx.Exec(fmt.Sprintf("DELETE FROM `%s`", sqlite3mig.DefaultMigrationsTable)); err != nil {
		return fmt.Errorf("resetting the schema version: %w", err)
	}
	if previous >= 0 {
		if _, err := tx.Exec(fmt.Sprintf("INSERT INTO `%s` (`version`, `dirty`) VALUES (?, 0)", sqlite3mig.DefaultMigrationsTable), previous); err != nil {
			return fmt.Errorf("resetting the schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("resetting the schema version: %w", err)
	}
	m.db.schemaVersion = uint(max(previous, 0))
	return nil
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

// RunPendingPostMigrations runs again the post migrations that didn't finish
// last time, oldest first (#2221). Until they have finished, the upgrade
// isn't done: the database isn't used, and no later migration runs.
//
// Another Stash may be running them right now, as the last part of its
// upgrade, so without the upgrade lock this waits for it and reads the
// records again (#2213).
func (m *Migrator) RunPendingPostMigrations(ctx context.Context) error {
	versions, err := pendingPostMigrations(ctx, m.conn)
	if err != nil || len(versions) == 0 {
		return err
	}
	if m.unlockUpgrade == nil {
		unlock, _, err := m.db.lockUpgrade(ctx)
		if err != nil {
			return err
		}
		defer unlock()
		if versions, err = pendingPostMigrations(ctx, m.conn); err != nil {
			return err
		}
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
	// held until Close, so through the rest of the upgrade and its post
	// migrations; the version is read after taking it, so a migration another
	// Stash ran meanwhile shows (#2213)
	if m.unlockUpgrade == nil {
		unlock, waited, err := m.db.lockUpgrade(ctx)
		if err != nil {
			return err
		}
		m.unlockUpgrade = unlock
		m.waitedForUpgrade = waited
	}

	databaseSchemaVersion, dirty, _ := m.m.Version()
	m.db.schemaVersion = databaseSchemaVersion

	// A Stash that waited for another takes up only an upgrade the other
	// finished: every version in place, none part way through its SQL, and no
	// follow-up step left to run, as on PostgreSQL (#2232).
	if m.waitedForUpgrade {
		pending, err := pendingPostMigrations(ctx, m.conn)
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

	// a post migration that didn't finish last time comes before the next migration
	if err := m.RunPendingPostMigrations(ctx); err != nil {
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
	ret, dirty, _ := m.m.Version()
	m.Close()
	if !dirty {
		return ret, nil
	}

	// a migration that was stopped part way: NewMigrator sets the version back,
	// with a connection that can write (#2230)
	w, err := NewMigrator(db)
	if err != nil {
		return 0, err
	}
	defer w.Close()
	return w.CurrentSchemaVersion(), nil
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
	return m.RunPendingPostMigrations(ctx)
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
