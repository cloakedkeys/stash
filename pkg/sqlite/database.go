package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
)

const (
	maxWriteConnections = 1
	// Number of database read connections to use
	// The same value is used for both the maximum and idle limit,
	// to prevent opening connections on the fly which has a notieable performance penalty.
	// Fewer connections use less memory, more connections increase performance,
	// but have diminishing returns.
	// 10 was found to be a good tradeoff.
	maxReadConnections = 10
	// Idle connection timeout, in seconds
	// Closes a connection after a period of inactivity, which saves on memory and
	// causes the sqlite -wal and -shm files to be automatically deleted.
	dbConnTimeout = 30 * time.Second

	// environment variable to set the cache size
	cacheSizeEnv = "STASH_SQLITE_CACHE_SIZE"

	// how long a connection waits for another's lock before giving up
	busyTimeout = 5 * time.Second
	// the same, while recording which version opened the database, which
	// shouldn't hold up opening it
	stampBusyTimeout = time.Second
)

var appSchemaVersion uint = 107

//go:embed migrations/*.sql
var migrationsBox embed.FS

type storeRepository struct {
	Blobs          *BlobStore
	File           *FileStore
	Folder         *FolderStore
	Audio          *AudioStore
	Story          *StoryStore
	Image          *ImageStore
	Gallery        *GalleryStore
	GalleryChapter *GalleryChapterStore
	Scene          *SceneStore
	SceneMarker    *SceneMarkerStore
	Performer      *PerformerStore
	VoiceActor     *VoiceActorStore
	SavedFilter    *SavedFilterStore
	Studio         *StudioStore
	Tag            *TagStore
	Group          *GroupStore
	User           *UserStore
	UserUIConfig   *UserUIConfigStore
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

	// writes counts committed write transactions. fullAnalyzeWrites is what it
	// was when the last successful full ANALYZE started, or -1 if there hasn't
	// been one. While the two match, nothing has been written since, so the
	// statistics are current.
	writes            atomic.Int64
	fullAnalyzeWrites atomic.Int64
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
		SceneMarker:    NewSceneMarkerStore(),
		Audio:          NewAudioStore(r, blobStore),
		Story:          NewStoryStore(r),
		Image:          NewImageStore(r),
		Gallery:        galleryStore,
		GalleryChapter: NewGalleryChapterStore(),
		Performer:      performerStore,
		VoiceActor:     &VoiceActorStore{},
		Studio:         studioStore,
		Tag:            tagStore,
		Group:          NewGroupStore(blobStore),
		SavedFilter:    NewSavedFilterStore(),
		User:           NewUserStore(),
		UserUIConfig:   NewUserUIConfigStore(),
	}

	ret := &Database{
		storeRepository: r,
		lockChan:        make(chan struct{}, 1),
	}
	// no full ANALYZE yet, so the first refresh always runs
	ret.fullAnalyzeWrites.Store(-1)

	return ret
}

func (db *Database) DatabaseBackend() database.DatabaseType {
	return database.SqliteBackend
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
		return &database.MigrationNeededError{
			CurrentSchemaVersion:  db.schemaVersion,
			RequiredSchemaVersion: appSchemaVersion,
		}
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

	db.dbPath, _ = strings.CutPrefix(dbPath, string(database.SqliteBackend)+":")

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

		// if migration is needed, then still open the connection
		// but don't allow most operations to run until the migration is complete
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

	db.removeWAL()

	return nil
}

// removeWAL has SQLite delete the -wal and -shm files that closing the pools
// left behind. SQLite deletes them when the last connection to the database
// closes, but only if that connection can write. Until the database is
// upgraded nothing uses the write pool, so the last connection is a read-only
// one and the files stay (#2111). Closing a writable connection, which doesn't
// run PRAGMA optimize, removes them. If another connection still has the
// database open, SQLite leaves them alone.
func (db *Database) removeWAL() {
	if db.dbPath == "" {
		return
	}
	if exists, _ := fsutil.FileExists(db.dbPath + "-wal"); !exists {
		return
	}
	// never create a database: the file may have been deleted while Stash ran
	if exists, _ := fsutil.FileExists(db.dbPath); !exists {
		return
	}

	// mode=rw can't create the file either. Don't wait for another program's
	// lock: if it has the database, the files are its to keep, so give up at
	// once, as PRAGMA optimize on close does (#2082).
	conn, err := sqlx.Open(sqlite3DriverNoOptimize, "file:"+db.dbPath+"?mode=rw&_journal=WAL&_busy_timeout=0")
	if err != nil {
		logger.Warnf("removing the database's temporary files: %v", err)
		return
	}
	// opening the connection sets WAL mode, which opens the -wal file
	if err := conn.Ping(); err != nil {
		logger.Warnf("removing the database's temporary files: %v", err)
	}
	if err := conn.Close(); err != nil {
		logger.Warnf("removing the database's temporary files: %v", err)
	}
}

func (db *Database) open(disableForeignKeys bool, writable bool) (*sqlx.DB, error) {
	return db.openWithDriver(sqlite3Driver, disableForeignKeys, writable)
}

func (db *Database) openWithDriver(driverName string, disableForeignKeys bool, writable bool) (*sqlx.DB, error) {
	// https://github.com/mattn/go-sqlite3
	// 5s timeout is the default.
	url := "file:" + db.dbPath + fmt.Sprintf("?_journal=WAL&_sync=NORMAL&_busy_timeout=%d", busyTimeout.Milliseconds())
	if !disableForeignKeys {
		url += "&_fk=true"
	}

	if writable {
		url += "&_txlock=immediate"
	} else {
		url += "&mode=ro"
	}

	// #5155 - set the cache size if the environment variable is set
	// default is -2000 which is 2MB
	if cacheSize := os.Getenv(cacheSizeEnv); cacheSize != "" {
		url += "&_cache_size=" + cacheSize
	}

	conn, err := sqlx.Open(driverName, url)
	if err != nil {
		return nil, fmt.Errorf("db.Open(): %w", err)
	}

	return conn, nil
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
	ctx, cancel := context.WithTimeout(context.Background(), database.AppVersionStampTimeout)
	defer cancel()
	if err := db.stampAppVersionOn(ctx); err != nil {
		logger.Warnf("%v", err)
	}
}

// stampAppVersionOn records the version on one write connection, which waits
// only stampBusyTimeout for another connection's write lock. A write is only
// needed the first time a build opens the database.
func (db *Database) stampAppVersionOn(ctx context.Context) error {
	conn, err := db.writeDB.Connx(ctx)
	if err != nil {
		return fmt.Errorf("recording the Stash version: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", stampBusyTimeout.Milliseconds())); err != nil {
		return fmt.Errorf("recording the Stash version: %w", err)
	}
	defer func() {
		// the usual wait again for the connection's next user, or a new connection
		if _, err := conn.ExecContext(context.Background(), fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeout.Milliseconds())); err != nil {
			retireConnections()
		}
	}()

	_, err = database.StampAppVersion(ctx, conn, database.CurrentAppVersion(), time.Now())
	return err
}

func (db *Database) openReadDB() error {
	return db.openReadDBWithDriver(sqlite3Driver)
}

func (db *Database) openReadDBWithDriver(driverName string) error {
	const (
		disableForeignKeys = false
		writable           = false
	)
	var err error
	db.readDB, err = db.openWithDriver(driverName, disableForeignKeys, writable)
	if err != nil {
		return err
	}
	db.readDB.SetMaxOpenConns(maxReadConnections)
	db.readDB.SetMaxIdleConns(maxReadConnections)
	db.readDB.SetConnMaxIdleTime(dbConnTimeout)
	return nil
}

func (db *Database) openWriteDB() error {
	return db.openWriteDBWithDriver(sqlite3Driver)
}

func (db *Database) openWriteDBWithDriver(driverName string) error {
	const (
		disableForeignKeys = false
		writable           = true
	)
	var err error
	db.writeDB, err = db.openWithDriver(driverName, disableForeignKeys, writable)
	if err != nil {
		return err
	}
	db.writeDB.SetMaxOpenConns(maxWriteConnections)
	db.writeDB.SetMaxIdleConns(maxWriteConnections)
	db.writeDB.SetConnMaxIdleTime(dbConnTimeout)
	return nil
}

func (db *Database) Remove() error {
	databasePath := db.dbPath
	err := db.Close()

	if err != nil {
		return fmt.Errorf("error closing database: %w", err)
	}

	err = os.Remove(databasePath)
	if err != nil {
		return fmt.Errorf("error removing database: %w", err)
	}

	// remove the -shm, -wal files ( if they exist )
	walFiles := []string{databasePath + "-shm", databasePath + "-wal"}
	for _, wf := range walFiles {
		if exists, _ := fsutil.FileExists(wf); exists {
			err = os.Remove(wf)
			if err != nil {
				return fmt.Errorf("error removing database: %w", err)
			}
		}
	}

	return nil
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

	if err := db.Open(context.Background(), databasePath); err != nil {
		return fmt.Errorf("[reset DB] unable to initialize: %w", err)
	}

	if err := db.restoreAccounts(accounts); err != nil {
		return fmt.Errorf("[reset DB] restoring accounts: %w", err)
	}

	return nil
}

// Backup the database. If db is nil, then uses the existing database
// connection.
func (db *Database) Backup(backupPath string) (err error) {
	thisDB := db.writeDB
	if thisDB == nil {
		thisDB, err = sqlx.Connect(sqlite3Driver, "file:"+db.dbPath+"?_fk=true")
		if err != nil {
			return fmt.Errorf("open database %s failed: %w", db.dbPath, err)
		}
		defer thisDB.Close()
	}

	// if backup path is not in the same directory as the database,
	// then backup to the same directory first, then move to the final location.
	// This is to prevent errors if the backup directory is over a network share.
	dbDir := filepath.Dir(db.dbPath)
	moveAfter := filepath.Dir(backupPath) != dbDir
	vacuumOut := backupPath
	if moveAfter {
		vacuumOut = filepath.Join(dbDir, filepath.Base(backupPath))
	}

	logger.Infof("Backing up database into: %s", vacuumOut)
	_, err = thisDB.Exec(`VACUUM INTO "` + vacuumOut + `"`)
	if err != nil {
		return fmt.Errorf("vacuum failed: %w", err)
	}

	if moveAfter {
		logger.Infof("Moving database backup to: %s", backupPath)
		err = fsutil.SafeMove(vacuumOut, backupPath)
		if err != nil {
			return fmt.Errorf("moving database backup failed: %w", err)
		}
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

// RestoreFromBackup replaces the database file with the backup, which is moved
// into its place. The database is closed first: Windows won't replace a file
// that's open (#2117), and an old -wal file left beside the restored database
// would be applied to it. If it was open, it's opened again afterwards. If the
// backup can't be moved into place, it's left where it was.
func (db *Database) RestoreFromBackup(backupPath string) error {
	db.resetMu.Lock()
	defer db.resetMu.Unlock()

	logger.Infof("Restoring backup database %s into %s", backupPath, db.dbPath)

	wasOpen := db.readDB != nil || db.writeDB != nil

	err := db.Close()
	if err != nil {
		// a pool that failed to close can't be used again either
		db.dropPools()
		err = fmt.Errorf("closing the database: %w", err)
	} else {
		err = db.checkClosed()
	}
	if err == nil {
		err = moveIntoPlace(backupPath, db.dbPath)
		if err == nil {
			if wasOpen {
				if err := db.Open(context.Background(), db.dbPath); err != nil {
					return fmt.Errorf("%w (from %s): %w", database.ErrRestoredNotReopened, backupPath, err)
				}
			}
			return nil
		}
	}
	err = fmt.Errorf("restoring the backup %s, which is left as it was: %w", backupPath, err)

	// back to the database as it was
	if wasOpen {
		if openErr := db.Open(context.Background(), db.dbPath); openErr != nil {
			err = fmt.Errorf("%w; opening the database again also failed: %v", err, openErr)
		}
	}
	return err
}

// renameFile is os.Rename. Tests replace it to stand in for a move between
// drives, and syncFile and syncDir to stand in for a failing disk.
var (
	renameFile = os.Rename
	// Windows only syncs a file opened for writing
	syncFile = func(path string) error { return syncPath(path, os.O_RDWR) }
	syncDir  = func(dir string) error {
		// Windows can't sync a directory, and doesn't need to
		if runtime.GOOS == "windows" {
			return nil
		}
		return syncPath(dir, os.O_RDONLY)
	}
)

// syncPath writes the file or directory at path to disk.
func syncPath(path string, flag int) error {
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// moveIntoPlace moves the backup over the database file. A rename can't move
// a file to another drive or volume (#2123), so when it fails, the backup is
// copied beside the database, and the copy renamed over it. That way the
// database is still replaced in one step, and a failed copy leaves it alone.
// fsutil.SafeMove can't be used, because its copy won't replace a file. The
// backup is removed once it's in place, as a move would.
func moveIntoPlace(backupPath, dbPath string) error {
	renameErr := renameFile(backupPath, dbPath)
	if renameErr == nil {
		return nil
	}

	tmp := dbPath + ".restoring"
	// left by an earlier restore that stopped part way
	_ = os.Remove(tmp)
	if err := fsutil.CopyFile(backupPath, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("copying it failed: %w; moving it failed first: %v", err, renameErr)
	}
	// on disk before it replaces the database, so a power cut can't leave a
	// half-written database
	if err := syncFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("saving a copy of it to disk failed: %w; moving it failed first: %v", err, renameErr)
	}
	if err := renameFile(tmp, dbPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replacing the database with a copy of it failed: %w; moving it failed first: %v", err, renameErr)
	}
	// and the rename too. The database is already restored, so if this fails,
	// keep the backup in case the rename is lost.
	if err := syncDir(filepath.Dir(dbPath)); err != nil {
		logger.Warnf("The backup %s was restored, but saving the change to disk failed, so the backup is kept: %v", backupPath, err)
		return nil
	}
	if err := os.Remove(backupPath); err != nil {
		logger.Warnf("The backup %s was restored, but couldn't be removed: %v", backupPath, err)
	}
	return nil
}

// dropPools closes whatever Close left open after an error, ignoring further
// errors, so that the pools can be opened again.
func (db *Database) dropPools() {
	db.lock()
	defer db.unlock()

	for _, pool := range []**sqlx.DB{&db.readDB, &db.writeDB} {
		if *pool != nil {
			_ = (*pool).Close()
			*pool = nil
		}
	}
}

// checkClosed returns an error if the database's -wal or -shm file is still
// there after closing it, which means another connection still has it open.
// Restoring over it then would mix the backup with that connection's changes.
func (db *Database) checkClosed() error {
	for _, f := range []string{db.dbPath + "-wal", db.dbPath + "-shm"} {
		if exists, _ := fsutil.FileExists(f); exists {
			return fmt.Errorf("the database is still open elsewhere: %s is still there", f)
		}
	}
	return nil
}

func (db *Database) AppSchemaVersion() uint {
	return appSchemaVersion
}

func (db *Database) DatabasePath() string {
	return db.dbPath
}

func (db *Database) DatabaseBackupPath(backupDirectoryPath string) string {
	fn := fmt.Sprintf("%s.%d.%s", filepath.Base(db.dbPath), db.schemaVersion, time.Now().Format("20060102_150405"))

	if backupDirectoryPath != "" {
		return filepath.Join(backupDirectoryPath, fn)
	}

	return fn
}

func (db *Database) AnonymousDatabasePath(backupDirectoryPath string) string {
	fn := fmt.Sprintf("%s.anonymous.%d.%s", filepath.Base(db.dbPath), db.schemaVersion, time.Now().Format("20060102_150405"))

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

// Vacuum runs a VACUUM on the database, rebuilding the database file into a minimal amount of disk space.
func (db *Database) Vacuum(ctx context.Context) error {
	_, err := db.writeDB.ExecContext(ctx, "VACUUM")
	return err
}

// Analyze runs an ANALYZE on the database to improve query performance.
func (db *Database) Analyze(ctx context.Context) error {
	return db.fullAnalyze(ctx, db.writeDB)
}

// fullAnalyze runs a full ANALYZE on conn and records it.
func (db *Database) fullAnalyze(ctx context.Context, conn *sqlx.DB) error {
	// Counted before it starts: a write committed while it runs makes the
	// count move, which only costs an extra refresh later.
	writes := db.writes.Load()
	err := analyze(ctx, conn)
	if err == nil {
		db.fullAnalyzeWrites.Store(writes)
	}
	// open connections would keep planning with the old statistics
	retireConnections()
	return err
}

// statisticsCurrent reports whether a full ANALYZE has run and nothing has
// been written since.
func (db *Database) statisticsCurrent() bool {
	return db.fullAnalyzeWrites.Load() == db.writes.Load()
}

// analyze runs an ANALYZE on the database to improve query performance.
func analyze(ctx context.Context, db *sqlx.DB) error {
	_, err := db.ExecContext(ctx, "ANALYZE")
	return err
}

// refreshStatisticsLimit is how many rows of each index a statistics refresh
// samples. It's the same limit the automatic optimize uses on close.
const refreshStatisticsLimit = 1000

// RefreshStatistics brings the query planner's statistics up to date with a
// quick, sampled ANALYZE. The automatic optimize only re-analyses a table when
// its row count changes a lot, so statistics go stale when existing rows change,
// such as images being given studios. On millions of rows it takes milliseconds.
//
// If a full ANALYZE, such as from Tasks -> Optimise Database or a migration,
// has finished and nothing has been written since, the statistics are already
// up to date, and a sampled ANALYZE would only make them less exact, so
// nothing is done.
func (db *Database) RefreshStatistics(ctx context.Context) error {
	db.resetMu.RLock()
	defer db.resetMu.RUnlock()

	if err := db.ready(); err != nil {
		return err
	}

	if db.statisticsCurrent() {
		return nil
	}

	// analysis_limit belongs to the connection, so hold one. It's retired
	// before going back to the pool, so a later full ANALYZE isn't sampled.
	conn, err := db.writeDB.Connx(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer retireConnections()

	_, err = conn.ExecContext(ctx, fmt.Sprintf("PRAGMA analysis_limit=%d; ANALYZE;", refreshStatisticsLimit))
	return err
}

// flushWAL flushes the Write-Ahead Log (WAL) to the main database file.
// It also truncates the WAL file to 0 bytes.
func flushWAL(ctx context.Context, db *sqlx.DB) error {
	_, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
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

	var lastInsertId *int64
	li, err := result.LastInsertId()
	if err == nil {
		lastInsertId = &li
	}

	return rowsAffected, lastInsertId, nil
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
