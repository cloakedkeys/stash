package sqlite

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/stashapp/stash/pkg/logger"
)

// upgradeLockSuffix names the file, next to the database, that Stash locks
// while it upgrades the database, so that a second Stash upgrading the same
// database waits for the whole upgrade rather than running part of it
// (#2213). It is the SQLite counterpart of PostgreSQL's advisory lock
// (#2232).
//
// The file is a small SQLite database that stays empty. The lock is a write
// transaction held open on a connection of its own, so it is SQLite's own
// file lock, which works the same on every platform Stash runs on, and the
// operating system releases it when the process ends, however it ends. The
// file isn't deleted afterwards: another Stash may be waiting on it, and
// would otherwise lock a new file of the same name instead.
const upgradeLockSuffix = ".upgrade-lock"

// upgradeLockRetry is how often a Stash waiting for the lock tries again.
var upgradeLockRetry = 200 * time.Millisecond

// lockUpgrade takes the upgrade lock, waiting while another Stash holds it.
// The returned function releases it; waited says whether another Stash held
// it first.
//
// If the lock file can't be used, for example because the folder is read
// only, Stash goes on without it, as before, with a warning.
func (db *Database) lockUpgrade(ctx context.Context) (unlock func(), waited bool, err error) {
	if db.privateFile {
		return func() {}, false, nil
	}
	path := db.dbPath + upgradeLockSuffix
	lockDB, err := sqlx.Open("sqlite3", "file:"+path+"?_busy_timeout=0")
	if err != nil {
		return lockUnavailable(path, err)
	}
	lockDB.SetMaxOpenConns(1)
	conn, err := lockDB.Connx(ctx)
	if err != nil {
		_ = lockDB.Close()
		return lockUnavailable(path, err)
	}
	release := func() {
		_ = conn.Close()
		_ = lockDB.Close()
	}

	for {
		_, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE")
		if err == nil {
			break
		}
		var se sqlite3.Error
		if !errors.As(err, &se) || (se.Code != sqlite3.ErrBusy && se.Code != sqlite3.ErrLocked) {
			release()
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			return lockUnavailable(path, err)
		}
		if !waited {
			logger.Info("Waiting until no other Stash is upgrading this database.")
			waited = true
		}
		select {
		case <-ctx.Done():
			release()
			return nil, false, ctx.Err()
		case <-time.After(upgradeLockRetry):
		}
	}

	return func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		release()
	}, waited, nil
}

// SetPrivateFile says that the database file is one that only this Stash
// opens, such as a new anonymised export, so that building it takes no
// upgrade lock and leaves no lock file beside it. Call it before Open.
func (db *Database) SetPrivateFile() {
	db.privateFile = true
}

func lockUnavailable(path string, err error) (func(), bool, error) {
	logger.Warnf("Can't use the upgrade lock file %s, so another Stash upgrading this database at the same time won't be waited for: %v", path, err)
	return func() {}, false, nil
}
