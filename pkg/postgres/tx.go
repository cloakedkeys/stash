package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
)

const (
	slowLogTime = time.Millisecond * 200
)

type dbReader interface {
	Get(dest interface{}, query string, args ...interface{}) error
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error)
}

func logSQL(start time.Time, query string, args ...interface{}) {
	since := time.Since(start)
	if since >= slowLogTime {
		logger.Debugf("SLOW SQL [%v]: %s, args: %v", since, query, logArgs(args))
	} else {
		logger.Tracef("SQL [%v]: %s, args: %v", since, query, logArgs(args))
	}
}

type dbWrapperType struct{}

var dbWrapper = dbWrapperType{}

func sqlError(err error, sql string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("error executing `%s` [%v]: %w", sql, args, err)
}

func (db *dbWrapperType) Rebind(query string) string {
	return rebindPlaceholders(query)
}

func (db *dbWrapperType) Get(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	query = db.Rebind(query)
	tx, err := getDBReader(ctx)
	if err != nil {
		return sqlError(err, query, args...)
	}

	start := time.Now()
	err = tx.GetContext(ctx, dest, query, args...)
	logSQL(start, query, args...)

	return sqlError(err, query, args...)
}

func (db *dbWrapperType) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	query = db.Rebind(query)
	tx, err := getDBReader(ctx)
	if err != nil {
		return sqlError(err, query, args...)
	}

	start := time.Now()
	err = tx.SelectContext(ctx, dest, query, args...)
	logSQL(start, query, args...)

	return sqlError(err, query, args...)
}

func (db *dbWrapperType) Queryx(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	query = db.Rebind(query)
	tx, err := getDBReader(ctx)
	if err != nil {
		return nil, sqlError(err, query, args...)
	}

	start := time.Now()
	ret, err := tx.QueryxContext(ctx, query, args...)
	logSQL(start, query, args...)

	return ret, sqlError(err, query, args...)
}

func (db *dbWrapperType) QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	query = db.Rebind(query)
	return dbWrapper.Queryx(ctx, query, args...)
}

func (db *dbWrapperType) NamedExec(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	query = db.Rebind(query)
	tx, err := getTx(ctx)
	if err != nil {
		return nil, sqlError(err, query, arg)
	}

	start := time.Now()
	ret, err := tx.NamedExecContext(ctx, query, arg)
	logSQL(start, query, arg)

	return ret, sqlError(err, query, arg)
}

func (db *dbWrapperType) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	query = db.Rebind(query)
	tx, err := getTx(ctx)
	if err != nil {
		return nil, sqlError(err, query, args...)
	}

	start := time.Now()
	ret, err := tx.ExecContext(ctx, query, args...)
	logSQL(start, query, args...)

	return ret, sqlError(err, query, args...)
}
