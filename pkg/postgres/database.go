package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/logger"
)

const (
	// TODO: Test for optimality
	maxWriteConnections = 5
	maxReadConnections  = 15
	// Idle connection timeout, in seconds
	// Closes a connection after a period of inactivity, which saves on memory and
	// causes the sqlite -wal and -shm files to be automatically deleted.
	dbConnTimeout = 30 * time.Second
)

var appSchemaVersion uint = 30

//go:embed migrations/*.sql
var migrationsBox embed.FS

type storeRepository struct {
	Blobs          *BlobStore
	File           *FileStore
	Folder         *FolderStore
	Image          *ImageStore
	Gallery        *GalleryStore
	GalleryChapter *GalleryChapterStore
	Audio          *AudioStore
	Story          *StoryStore
	Scene          *SceneStore
	SceneMarker    *SceneMarkerStore
	VoiceActor     *VoiceActorStore
	Performer      *PerformerStore
	User           *UserStore
	UserUIConfig   *UserUIConfigStore
	SavedFilter    *SavedFilterStore
	Studio         *StudioStore
	Tag            *TagStore
	Group          *GroupStore
}

type Database struct {
	*storeRepository

	readDB  *sqlx.DB
	writeDB *sqlx.DB
	dbPath  string

	schemaVersion uint

	// resetMu drains active database users and excludes new ones throughout reset.
	resetMu sync.RWMutex

	lockChan chan struct{}
}

func NewDatabase() *Database {
	fileStore := NewFileStore()
	folderStore := NewFolderStore()
	galleryStore := NewGalleryStore(fileStore, folderStore)
	blobStore := NewBlobStore(database.BlobStoreOptions{})
	performerStore := NewPerformerStore(blobStore)
	studioStore := NewStudioStore(blobStore)
	tagStore := NewTagStore(blobStore)

	r := &storeRepository{}
	*r = storeRepository{
		Blobs:          blobStore,
		File:           fileStore,
		Folder:         folderStore,
		Scene:          NewSceneStore(r, blobStore),
		Audio:          NewAudioStore(r, blobStore),
		Story:          NewStoryStore(r),
		User:           NewUserStore(),
		UserUIConfig:   NewUserUIConfigStore(),
		SceneMarker:    NewSceneMarkerStore(),
		Image:          NewImageStore(r),
		Gallery:        galleryStore,
		GalleryChapter: NewGalleryChapterStore(),
		VoiceActor:     &VoiceActorStore{},
		Performer:      performerStore,
		Studio:         studioStore,
		Tag:            tagStore,
		Group:          NewGroupStore(blobStore),
		SavedFilter:    NewSavedFilterStore(),
	}

	ret := &Database{
		storeRepository: r,
		lockChan:        make(chan struct{}, 1),
	}

	return ret
}

func (db *Database) DatabaseBackend() database.DatabaseType {
	return database.PostgresBackend
}

func (db *Database) SetBlobStoreOptions(options database.BlobStoreOptions) {
	*db.storeRepository.Blobs = *NewBlobStore(options)
}

// Ready returns an error if the database is not ready to begin transactions.
func (db *Database) Ready() error {
	db.resetMu.RLock()
	defer db.resetMu.RUnlock()

	return db.ready()
}

// ready is called with resetMu held.
func (db *Database) ready() error {
	if db.readDB == nil || db.writeDB == nil {
		return database.ErrDatabaseNotInitialized
	}

	if db.needsMigration() {
		return &database.MigrationNeededError{CurrentSchemaVersion: db.schemaVersion, RequiredSchemaVersion: appSchemaVersion}
	}
	return nil
}

// Open initializes the database. If the database is new, then it
// performs a full migration to the latest schema version. Otherwise, any
// necessary migrations must be run separately using RunMigrations.
// Returns true if the database is new.
func (db *Database) Open(ctx context.Context, dbPath string) error {
	db.lock()
	defer db.unlock()

	db.dbPath, _ = strings.CutPrefix(dbPath, string(database.PostgresBackend)+":")

	if err := db.checkEncoding(ctx); err != nil {
		return err
	}

	databaseSchemaVersion, err := db.getDatabaseSchemaVersion()
	if err != nil {
		return fmt.Errorf("getting database schema version: %w", err)
	}

	db.schemaVersion = databaseSchemaVersion

	isNew := databaseSchemaVersion == 0

	if isNew {
		// new database, just run the migrations
		if err := db.RunAllMigrations(ctx); err != nil {
			return fmt.Errorf("error running initial schema migrations: %w", err)
		}
	} else {
		if databaseSchemaVersion > appSchemaVersion {
			return &database.MismatchedSchemaVersionError{
				CurrentSchemaVersion:  databaseSchemaVersion,
				RequiredSchemaVersion: appSchemaVersion,
			}
		}

		// the upgrade isn't done until a follow-up step that failed has run
		if err := db.runPendingPostMigrations(ctx); err != nil {
			return err
		}

	}

	if err := db.initialise(); err != nil {
		return err
	}

	if isNew {
		// optimize database after migration
		err = db.Optimise(context.Background())
		if err != nil {
			logger.Warnf("error while performing post-migration optimisation: %v", err)
		}
	}

	return nil
}

// checkEncoding refuses a database that isn't UTF8. Translated regular expressions name characters
// by their Unicode values, so on another encoding they could match the wrong rows with no error.
func (db *Database) checkEncoding(ctx context.Context) error {
	conn, err := db.open(false)
	if err != nil {
		return err
	}
	defer conn.Close()

	var encoding string
	if err := conn.GetContext(ctx, &encoding, "SELECT getdatabaseencoding()"); err != nil {
		return fmt.Errorf("checking database encoding: %w", err)
	}
	if encoding != "UTF8" {
		return fmt.Errorf("the database uses the %s encoding, but Stash needs UTF8: create it with createdb -E UTF8 -T template0", encoding)
	}
	return nil
}

// lock locks the database for writing. This method will block until the lock is acquired.
func (db *Database) lock() {
	db.lockChan <- struct{}{}
}

// unlock unlocks the database
func (db *Database) unlock() {
	// will block the caller if the lock is not held, so check first
	select {
	case <-db.lockChan:
		return
	default:
		panic("database is not locked")
	}
}

func (db *Database) Close() error {
	db.lock()
	defer db.unlock()

	if db.readDB != nil {
		if err := db.readDB.Close(); err != nil {
			return err
		}

		db.readDB = nil
	}
	if db.writeDB != nil {
		if err := db.writeDB.Close(); err != nil {
			return err
		}

		db.writeDB = nil
	}

	return nil
}

func (db *Database) open(writable bool) (conn *sqlx.DB, err error) {
	cfg, err := pgx.ParseConfig(db.dbPath)

	if err != nil {
		return nil, fmt.Errorf("db.Open(): %w", err)
	}

	// these only apply to one session, so every connection in the pool needs them
	var settings []string
	if !writable {
		settings = append(settings, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY;")
	}

	sqlDB := stdlib.OpenDB(*cfg, stdlib.OptionAfterConnect(func(ctx context.Context, c *pgx.Conn) error {
		for _, setting := range settings {
			if _, err := c.Exec(ctx, setting); err != nil {
				return fmt.Errorf("conn.Exec(): %w", err)
			}
		}
		return nil
	}))

	return sqlx.NewDb(sqlDB, "pgx"), nil
}

func (db *Database) initialise() error {
	if err := db.openReadDB(); err != nil {
		return fmt.Errorf("opening read database: %w", err)
	}
	if err := db.openWriteDB(); err != nil {
		return fmt.Errorf("opening write database: %w", err)
	}

	if !db.needsMigration() {
		db.stampAppVersion()
	}

	return nil
}

// stampAppVersion records this build as the one that last opened the database,
// once it's at the current schema version, so that a later migration can read
// it. Before then the stamp is the older build's, which the migration needs.
// Failing to write it only loses the record, so it doesn't stop Stash, and it
// waits at most database.AppVersionStampTimeout.
func (db *Database) stampAppVersion() {
	if err := db.stampAppVersionTx(); err != nil {
		logger.Warnf("%v", err)
	}
}

// stampAppVersionTx sets the time limit on the server too. Cancelling only the
// client's context can leave the statement waiting on the server, where it
// would still write once the other session lets go.
func (db *Database) stampAppVersionTx() error {
	ctx, cancel := context.WithTimeout(context.Background(), database.AppVersionStampTimeout+time.Second)
	defer cancel()

	tx, err := db.writeDB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("recording the Stash version: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	limit := database.AppVersionStampTimeout.Milliseconds()
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL lock_timeout = %d; SET LOCAL statement_timeout = %d", limit, limit)); err != nil {
		return fmt.Errorf("recording the Stash version: %w", err)
	}
	if _, err := database.StampAppVersion(ctx, tx, database.CurrentAppVersion(), time.Now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("recording the Stash version: %w", err)
	}
	return nil
}

func (db *Database) openReadDB() error {
	const writable = false
	var err error
	db.readDB, err = db.open(writable)
	if err != nil {
		return err
	}
	db.readDB.SetMaxOpenConns(maxReadConnections)
	db.readDB.SetMaxIdleConns(maxReadConnections)
	db.readDB.SetConnMaxIdleTime(dbConnTimeout)
	return nil
}

func (db *Database) openWriteDB() error {
	const writable = true
	var err error
	db.writeDB, err = db.open(writable)
	if err != nil {
		return err
	}
	db.writeDB.SetMaxOpenConns(maxWriteConnections)
	db.writeDB.SetMaxIdleConns(maxWriteConnections)
	db.writeDB.SetConnMaxIdleTime(dbConnTimeout)
	return nil
}

func (db *Database) Remove() (err error) {
	_, err = db.writeDB.Exec(`
DO $$
DECLARE 
    r record;
BEGIN
    FOR r IN SELECT quote_ident(tablename) AS tablename, quote_ident(schemaname) AS schemaname FROM pg_tables WHERE schemaname = 'public'
    LOOP
        RAISE INFO 'Dropping table %.%', r.schemaname, r.tablename;
        EXECUTE format('DROP TABLE IF EXISTS %I.%I CASCADE', r.schemaname, r.tablename);
    END LOOP;
END$$;
`)

	return err
}

func (db *Database) Reset() error {
	db.resetMu.Lock()
	defer db.resetMu.Unlock()

	databasePath := db.dbPath
	accounts, err := db.readAccounts()
	if err != nil {
		return fmt.Errorf("[reset DB] reading accounts: %w", err)
	}

	if err := db.Remove(); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}

	if err := db.Open(context.Background(), databasePath); err != nil {
		return fmt.Errorf("[reset DB] unable to initialize: %w", err)
	}

	if err := db.restoreAccounts(accounts); err != nil {
		return fmt.Errorf("[reset DB] restoring accounts: %w", err)
	}

	return nil
}

func (db *Database) Anonymise(outPath string) error {
	anon, err := NewAnonymiser(db, outPath)

	if err != nil {
		return err
	}

	return anon.Anonymise(context.Background())
}

func (db *Database) AppSchemaVersion() uint {
	return appSchemaVersion
}

func (db *Database) DatabasePath() string {
	return db.dbPath
}

func (db *Database) DatabaseBackupPath(backupDirectoryPath string) string {
	fn := fmt.Sprintf("%s.%d.%s.dump", "postgres", db.schemaVersion, time.Now().Format("20060102_150405"))

	if backupDirectoryPath != "" {
		return filepath.Join(backupDirectoryPath, fn)
	}

	return fn
}

func (db *Database) AnonymousDatabasePath(backupDirectoryPath string) string {
	fn := fmt.Sprintf("%s.anonymous.%d.%s", "postgres", db.schemaVersion, time.Now().Format("20060102_150405"))

	if backupDirectoryPath != "" {
		return filepath.Join(backupDirectoryPath, fn)
	}

	return fn
}

func (db *Database) Version() uint {
	return db.schemaVersion
}

func (db *Database) Optimise(ctx context.Context) error {
	logger.Info("Optimising database")

	err := db.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("performing optimization: %w", err)
	}

	err = db.Vacuum(ctx)
	if err != nil {
		return fmt.Errorf("performing vacuum: %w", err)
	}

	return nil
}

// Vacuum runs a VACUUM on stash's tables, rebuilding them into a minimal amount of disk space.
func (db *Database) Vacuum(ctx context.Context) error {
	tables, err := ownTables(ctx, db.writeDB)
	if err != nil || tables == "" {
		return err
	}
	_, err = db.writeDB.ExecContext(ctx, "VACUUM (FULL, ANALYZE, VERBOSE) "+tables)
	return err
}

// Analyze runs an ANALYZE on the database to improve query performance.
func (db *Database) Analyze(ctx context.Context) error {
	return analyze(ctx, db.writeDB)
}

// RefreshStatistics does nothing on PostgreSQL: its autovacuum already
// re-analyses tables as their rows change.
func (db *Database) RefreshStatistics(ctx context.Context) error {
	return nil
}

// ownTablesQuery lists stash's own tables, the ones in its schema, ready to
// follow ANALYZE or VACUUM. It is empty if there are none.
//
// #2203: a plain ANALYZE or VACUUM covers the whole database, including the
// catalogs that every database on the server shares, such as pg_authid. A
// lock that a session in another database holds on one of those makes it
// wait, so stash's startup could stall behind someone else's work.
const ownTablesQuery = `SELECT COALESCE(string_agg(format('%I.%I', schemaname, tablename), ', ' ORDER BY tablename), '')
FROM pg_tables WHERE schemaname = current_schema()`

func ownTables(ctx context.Context, db *sqlx.DB) (string, error) {
	var tables string
	if err := db.GetContext(ctx, &tables, ownTablesQuery); err != nil {
		return "", fmt.Errorf("listing tables: %w", err)
	}
	return tables, nil
}

// analyze runs an ANALYZE on stash's tables to improve query performance.
func analyze(ctx context.Context, db *sqlx.DB) error {
	tables, err := ownTables(ctx, db)
	if err != nil || tables == "" {
		return err
	}
	_, err = db.ExecContext(ctx, "ANALYZE "+tables)
	return err
}

func (db *Database) ExecSQL(ctx context.Context, query string, args []interface{}) (*int64, *int64, error) {
	wrapper := dbWrapperType{}

	result, err := wrapper.Exec(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}

	var rowsAffected *int64
	ra, err := result.RowsAffected()
	if err == nil {
		rowsAffected = &ra
	}

	return rowsAffected, nil, nil
}

func (db *Database) QuerySQL(ctx context.Context, query string, args []interface{}) ([]string, [][]interface{}, error) {
	wrapper := dbWrapperType{}

	rows, err := wrapper.QueryxContext(ctx, query, args...)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var ret [][]interface{}

	for rows.Next() {
		row, err := rows.SliceScan()
		if err != nil {
			return nil, nil, err
		}
		ret = append(ret, row)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return cols, ret, nil
}

func getBasenameSQL(sql string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("basename(%s, '\\')", sql)
	}

	return fmt.Sprintf("basename(%s)", sql)
}
