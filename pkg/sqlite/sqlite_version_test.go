package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// #2032: SQLite 3.45.0 to 3.49.x marked an index "low quality" on the
// connection that analysed it while it held a single value, and kept using a
// full scan there after a later ANALYZE showed many values. 3.50.0 removed
// that.
func TestReanalysedIndexIsUsedOnSameConnection(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Open(sqlite3Driver, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1) // the same connection throughout

	var version string
	require.NoError(t, db.GetContext(ctx, &version, "SELECT sqlite_version()"))
	t.Logf("SQLite %s", version)

	_, err = db.ExecContext(ctx, `CREATE TABLE images (id INTEGER PRIMARY KEY, studio_id INTEGER);
CREATE INDEX index_images_on_studio_id ON images (studio_id);
WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000)
INSERT INTO images SELECT x, 1 FROM n;
ANALYZE;`)
	require.NoError(t, err)

	// every image now gets one of 1,000 studios
	_, err = db.ExecContext(ctx, `UPDATE images SET studio_id = id % 1000; ANALYZE;`)
	require.NoError(t, err)

	var plan []struct {
		ID     int    `db:"id"`
		Parent int    `db:"parent"`
		Unused int    `db:"notused"`
		Detail string `db:"detail"`
	}
	require.NoError(t, db.SelectContext(ctx, &plan, "EXPLAIN QUERY PLAN SELECT id FROM images WHERE studio_id = 7"))
	var details []string
	for _, step := range plan {
		details = append(details, step.Detail)
	}
	require.Contains(t, strings.Join(details, "\n"), "USING COVERING INDEX index_images_on_studio_id", "SQLite %s", version)
}
