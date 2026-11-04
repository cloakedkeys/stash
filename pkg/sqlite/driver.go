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

const (
	sqlite3Driver = "sqlite3ex"

	// sqlite3DriverNoOptimize is sqlite3Driver without the PRAGMA optimize when
	// a connection closes, for connections that only read.
	sqlite3DriverNoOptimize = "sqlite3ex_nooptimize"
)

func init() {
	// register custom driver
	sql.Register(sqlite3Driver, &CustomSQLiteDriver{})
	sql.Register(sqlite3DriverNoOptimize, &CustomSQLiteDriver{noOptimize: true})
}

type CustomSQLiteDriver struct {
	// noOptimize skips the PRAGMA optimize on close. PRAGMA optimize takes the
	// write lock up front when two or more tables might need analysing: a
	// table counts if a query on the connection used it, or if any of its
	// indexes has no sqlite_stat1 row, which is most of the indexes in a new
	// Stash database. So even a connection that queried nothing waits the busy
	// timeout while another connection is writing (#2071).
	noOptimize bool
}

// statisticsGeneration goes up each time ANALYZE refreshes the statistics.
//
// A connection reads the statistics when it opens. On every SQLite version,
// other connections don't reload them after an ANALYZE, because ANALYZE
// doesn't change the schema version. So connections opened before a refresh
// are retired instead of reused, and busy pools that never sit idle long
// enough to close still pick up the new statistics.
//
// SQLite 3.45.0 to 3.49.x, which Stash bundled before #2032, added a second
// reason for the connection that runs the ANALYZE: when an index's statistics
// showed a single value, SQLite marked it "low quality" (bLowQual), stopped
// using it for lookups, and never cleared the mark, even after reloading.
// 3.50.0 removed that, but the first reason still applies.
var statisticsGeneration atomic.Uint64

// retireConnections makes every open connection be replaced by a new one
// the next time it's returned to, or taken from, the pool.
func retireConnections() {
	statisticsGeneration.Add(1)
}

type CustomSQLiteConn struct {
	*sqlite3.SQLiteConn
	generation uint64
	noOptimize bool
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

	return &CustomSQLiteConn{SQLiteConn: conn.(*sqlite3.SQLiteConn), generation: generation, noOptimize: d.noOptimize}, nil
}

func (c *CustomSQLiteConn) Close() error {
	conn := c.SQLiteConn

	if !c.noOptimize {
		_, _ = conn.Exec("PRAGMA analysis_limit=1000; PRAGMA optimize;", []driver.Value{})
	}

	return conn.Close()
}
