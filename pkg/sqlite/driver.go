package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync/atomic"

	"github.com/WithoutPants/sortorder/casefolded"
	sqlite3 "github.com/mattn/go-sqlite3"
)

const sqlite3Driver = "sqlite3ex"

func init() {
	// register custom driver
	sql.Register(sqlite3Driver, &CustomSQLiteDriver{})
}

type CustomSQLiteDriver struct{}

// statisticsGeneration goes up each time ANALYZE refreshes the statistics.
//
// A connection reads the statistics when it opens, and a reload doesn't fully
// replace them: SQLite marks an index "low quality" when its statistics show a
// single value, stops using it for lookups, and never clears that mark on the
// same connection (bLowQual). So connections opened before a refresh are
// retired instead of reused.
//
// This is a workaround for SQLite 3.45.0 to 3.49.x, which have that flag; the
// bundled 3.45.1 is one of them. 3.50.0 removed it, so after an upgrade this
// can go.
var statisticsGeneration atomic.Uint64

// retireConnections makes every open connection be replaced by a new one
// the next time it's returned to, or taken from, the pool.
func retireConnections() {
	statisticsGeneration.Add(1)
}

type CustomSQLiteConn struct {
	*sqlite3.SQLiteConn
	generation uint64
}

func (c *CustomSQLiteConn) current() bool {
	return c.generation == statisticsGeneration.Load()
}

// IsValid implements driver.Validator, checked when the connection goes back
// to the pool.
func (c *CustomSQLiteConn) IsValid() bool {
	return c.current()
}

// ResetSession implements driver.SessionResetter, called before an idle
// connection is reused.
func (c *CustomSQLiteConn) ResetSession(ctx context.Context) error {
	if !c.current() {
		return driver.ErrBadConn
	}
	return nil
}

func (d *CustomSQLiteDriver) Open(dsn string) (driver.Conn, error) {
	sqlite3Driver := &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			funcs := map[string]interface{}{
				"regexp":            regexFn,
				"durationToTinyInt": durationToTinyIntFn,
				"basename":          basenameFn,
				"phash_distance":    phashDistanceFn,
				"lower_unicode":     lowerUnicodeFn,
				"upper_unicode":     upperUnicodeFn,
			}

			for name, fn := range funcs {
				if err := conn.RegisterFunc(name, fn, true); err != nil {
					return fmt.Errorf("error registering function %s: %v", name, err)
				}
			}

			// COLLATE NATURAL_CI - Case insensitive natural sort
			err := conn.RegisterCollation("NATURAL_CI", func(s string, s2 string) int {
				if casefolded.NaturalLess(s, s2) {
					return -1
				} else {
					return 1
				}
			})

			if err != nil {
				return fmt.Errorf("error registering natural sort collation: %v", err)
			}

			return nil
		},
	}

	// read before opening, so a refresh during the open retires this one too
	generation := statisticsGeneration.Load()

	conn, err := sqlite3Driver.Open(dsn)
	if err != nil {
		return nil, err
	}

	return &CustomSQLiteConn{SQLiteConn: conn.(*sqlite3.SQLiteConn), generation: generation}, nil
}

func (c *CustomSQLiteConn) Close() error {
	conn := c.SQLiteConn

	_, _ = conn.Exec("PRAGMA analysis_limit=1000; PRAGMA optimize;", []driver.Value{})

	return conn.Close()
}
