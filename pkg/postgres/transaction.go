package postgres

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

type key int

const (
	txnKey key = iota + 1
	dbKey
	writableKey
	txnReleaseKey
)

// WithDatabase holds off reset until the caller releases the database context.
func (db *Database) WithDatabase(ctx context.Context) (context.Context, func(), error) {
	// An existing reader already holds the reset guard. Do not lock it again:
	// a pending reset would block a nested read lock.
	if reader, _ := getDBReader(ctx); reader != nil {
		return ctx, func() {}, nil
	}

	db.resetMu.RLock()
	if db.readDB == nil {
		db.resetMu.RUnlock()
		return ctx, nil, database.ErrDatabaseNotInitialized
	}

	return context.WithValue(ctx, dbKey, db.readDB), sync.OnceFunc(db.resetMu.RUnlock), nil
}

func (db *Database) Begin(ctx context.Context, writable bool) (ret context.Context, err error) {
	if tx, _ := getTx(ctx); tx != nil {
		// log the stack trace so we can see
		logger.Error(string(debug.Stack()))

		return nil, fmt.Errorf("already in transaction")
	}

	// WithDatabase may already own the guard for this scope.
	release := func() {}
	if reader, _ := getDBReader(ctx); reader == nil {
		db.resetMu.RLock()
		release = sync.OnceFunc(db.resetMu.RUnlock)
	}
	defer func() {
		if err != nil {
			release()
		}
	}()

	if db.readDB == nil {
		return nil, fmt.Errorf("database not ready: %w", database.ErrDatabaseNotInitialized)
	}

	dbtx := db.readDB
	if writable {
		if err := db.ready(); err != nil {
			return nil, fmt.Errorf("database not ready: %w", err)
		}
		dbtx = db.writeDB
	}

	// Refresh database connection
	if err := dbtx.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	tx, err := dbtx.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}

	ctx = context.WithValue(ctx, txnReleaseKey, release)
	ctx = context.WithValue(ctx, writableKey, writable)

	return context.WithValue(ctx, txnKey, tx), nil
}

func (db *Database) Commit(ctx context.Context) error {
	tx, err := getTx(ctx)
	if err != nil {
		return err
	}

	defer db.txnComplete(ctx)

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (db *Database) Rollback(ctx context.Context) error {
	tx, err := getTx(ctx)
	if err != nil {
		return err
	}

	defer db.txnComplete(ctx)

	if err := tx.Rollback(); err != nil {
		return err
	}

	return nil
}

func (db *Database) txnComplete(ctx context.Context) {
	if release, ok := ctx.Value(txnReleaseKey).(func()); ok {
		release()
	}
}

func getTx(ctx context.Context) (*sqlx.Tx, error) {
	tx, ok := ctx.Value(txnKey).(*sqlx.Tx)
	if !ok || tx == nil {
		return nil, fmt.Errorf("not in transaction")
	}
	return tx, nil
}

func getDBReader(ctx context.Context) (dbReader, error) {
	// get transaction first if present
	tx, ok := ctx.Value(txnKey).(*sqlx.Tx)
	if !ok || tx == nil {
		// try to get database if present
		db, ok := ctx.Value(dbKey).(*sqlx.DB)
		if !ok || db == nil {
			return nil, fmt.Errorf("not in transaction")
		}
		return db, nil
	}
	return tx, nil
}

func (db *Database) IsLocked(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// Class 53 — Insufficient Resources
		return pgErr.Code[:2] == "53"
	}
	return false
}

func (db *Database) Repository() models.Repository {
	return models.Repository{
		TxnManager:     models.TxnManager{Manager: db, DatabaseProvider: db},
		Blob:           db.Blobs(),
		File:           db.File(),
		Folder:         db.Folder(),
		Gallery:        db.Gallery(),
		GalleryChapter: db.GalleryChapter(),
		Image:          db.Image(),
		Group:          db.Group(),
		Audio:          db.Audio(),
		Story:          db.Story(),
		UserUIConfig:   db.UserUIConfig(),
		VoiceActor:     db.VoiceActor(),
		Performer:      db.Performer(),
		Scene:          db.Scene(),
		SceneMarker:    db.SceneMarker(),
		Studio:         db.Studio(),
		Tag:            db.Tag(),
		SavedFilter:    db.SavedFilter(),
	}
}
