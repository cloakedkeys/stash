//go:build integration

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Closing a database while another connection is writing doesn't wait the busy
// timeout for PRAGMA optimize (#2082).
func TestCloseDoesNotWaitForWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stash-go.sqlite")
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	require.NoError(t, db.Close())

	// open it again, as Stash does when it starts, and use both pools
	db = NewDatabase()
	require.NoError(t, db.Open(ctx, path))
	const query = "SELECT (SELECT COUNT(*) FROM scenes) + (SELECT COUNT(*) FROM performers) + (SELECT COUNT(*) FROM tags)"
	var n int
	require.NoError(t, db.readDB.Get(&n, query))
	require.NoError(t, db.writeDB.Get(&n, query))

	holdWriteLock(t, path)

	started := time.Now()
	require.NoError(t, db.Close())
	elapsed := time.Since(started)
	t.Logf("closed in %v", elapsed)
	assert.Less(t, elapsed, busyTimeout/2, "waited %v", elapsed)
}

// createUnanalysedDatabase makes a database whose tables have indexes but no
// statistics, so PRAGMA optimize analyses them.
func createUnanalysedDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unanalysed.sqlite")
	conn, err := sql.Open("sqlite3", "file:"+path+"?_journal=WAL")
	require.NoError(t, err)
	defer conn.Close()
	for _, stmt := range []string{
		"CREATE TABLE a (id INTEGER PRIMARY KEY, v INTEGER)",
		"CREATE INDEX a_v ON a (v)",
		"CREATE TABLE b (id INTEGER PRIMARY KEY, v INTEGER)",
		"CREATE INDEX b_v ON b (v)",
		"INSERT INTO a (v) VALUES (1), (2), (3)",
		"INSERT INTO b (v) VALUES (1), (2), (3)",
	} {
		_, err := conn.Exec(stmt)
		require.NoError(t, err, stmt)
	}
	require.Zero(t, statRows(t, path))
	return path
}

// statRows counts the rows in sqlite_stat1, using the plain go-sqlite3 driver,
// which doesn't optimize on close.
func statRows(t *testing.T, path string) int {
	t.Helper()
	conn, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	require.NoError(t, err)
	defer conn.Close()
	var tables int
	require.NoError(t, conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'sqlite_stat1'").Scan(&tables))
	if tables == 0 {
		return 0
	}
	var rows int
	require.NoError(t, conn.QueryRow("SELECT COUNT(*) FROM sqlite_stat1").Scan(&rows))
	return rows
}

// closeAfterQuery opens a connection, queries both tables and closes it.
func closeAfterQuery(t *testing.T, driverName string, dsn string) {
	t.Helper()
	conn, err := sqlx.Open(driverName, dsn)
	require.NoError(t, err)
	var n int
	require.NoError(t, conn.Get(&n, "SELECT (SELECT COUNT(*) FROM a WHERE v = 1) + (SELECT COUNT(*) FROM b WHERE v = 1)"))
	require.NoError(t, conn.Close())
}

// When nothing else is writing, closing a connection still runs PRAGMA
// optimize, which fills in the missing statistics.
func TestOptimizeOnClose(t *testing.T) {
	path := createUnanalysedDatabase(t)
	closeAfterQuery(t, sqlite3Driver, "file:"+path+"?_journal=WAL&_txlock=immediate")
	assert.Positive(t, statRows(t, path), "no statistics after closing")
}

// The version check's driver and read-only connections don't optimize on close.
func TestNoOptimizeOnClose(t *testing.T) {
	for _, test := range []struct {
		name, driverName, params string
	}{
		{"version check driver", sqlite3DriverNoOptimize, "_journal=WAL"},
		{"query only", sqlite3Driver, "_journal=WAL&_query_only=1"},
		{"read only", sqlite3Driver, "_journal=WAL&mode=ro"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := createUnanalysedDatabase(t)
			closeAfterQuery(t, test.driverName, "file:"+path+"?"+test.params)
			assert.Zero(t, statRows(t, path))
		})
	}
}

// The read pool's connections don't attempt PRAGMA optimize when they close,
// because it always fails on a read-only connection. The write pool's do.
func TestReadPoolSkipsOptimize(t *testing.T) {
	ctx := context.Background()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, filepath.Join(t.TempDir(), "stash-go.sqlite")))
	t.Cleanup(func() { _ = db.Close() })

	noOptimize := func(pool *sqlx.DB) bool {
		conn, err := pool.Conn(ctx)
		require.NoError(t, err)
		defer conn.Close()
		var ret bool
		require.NoError(t, conn.Raw(func(driverConn any) error {
			ret = driverConn.(*CustomSQLiteConn).noOptimize
			return nil
		}))
		return ret
	}

	assert.True(t, noOptimize(db.readDB), "read pool")
	assert.False(t, noOptimize(db.writeDB), "write pool")
}

func TestReadOnlyDSN(t *testing.T) {
	for dsn, want := range map[string]bool{
		"file:x.db?_journal=WAL&mode=ro":       true,
		"file:x.db?mode=ro&_busy_timeout=5000": true,
		"file:x.db?mode=rw":                    false,
		"file:x.db?_txlock=immediate":          false,
		"file:x.db":                            false,
		"x.db?mode=ro":                         false, // go-sqlite3 doesn't pass mode to SQLite
		"x.db?_query_only=true":                true,
		"file:x.db?_query_only=ON":             true,
		"file:x.db?_query_only=0":              false,
	} {
		assert.Equal(t, want, readOnlyDSN(dsn), dsn)
	}
}
