//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/require"
)

func imageStudioIndexStat(t *testing.T, db *Database) string {
	t.Helper()
	var stat string
	require.NoError(t, db.readDB.Get(&stat, "SELECT stat FROM sqlite_stat1 WHERE idx = 'index_images_on_studio_id'"))
	return stat
}

func imageStat4Rows(t *testing.T, db *Database) int {
	t.Helper()
	var n int
	require.NoError(t, db.readDB.Get(&n, "SELECT COUNT(*) FROM sqlite_stat4 WHERE tbl = 'images'"))
	return n
}

// imageStudioCountPlan plans a studio's image count on a read connection, as
// the Studios page does.
func imageStudioCountPlan(t *testing.T, db *Database) string {
	t.Helper()
	var plan []struct {
		ID     int    `db:"id"`
		Parent int    `db:"parent"`
		Unused int    `db:"notused"`
		Detail string `db:"detail"`
	}
	require.NoError(t, db.readDB.Select(&plan, "EXPLAIN QUERY PLAN SELECT COUNT(*) FROM images WHERE studio_id = 7"))
	var details []string
	for _, step := range plan {
		details = append(details, step.Detail)
	}
	return strings.Join(details, "\n")
}

const lookUpStudio = "SEARCH images USING COVERING INDEX index_images_on_studio_id (studio_id=?)"

// write runs statements in a write transaction, as the app does.
func write(t *testing.T, db *Database, sql string) {
	t.Helper()
	require.NoError(t, txn.WithTxn(context.Background(), db, func(ctx context.Context) error {
		_, err := dbWrapper.Exec(ctx, sql)
		return err
	}))
}

const addStudiosAndImages = `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<200)
INSERT INTO studios (id, name, created_at, updated_at) SELECT x, 'studio ' || x, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM n;
WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
INSERT INTO images (id, studio_id, created_at, updated_at) SELECT x, CASE WHEN x<=2000 THEN 1 END, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM n;`

func openTestDatabase(t *testing.T) *Database {
	t.Helper()
	db := NewDatabase()
	require.NoError(t, db.Open(context.Background(), filepath.Join(t.TempDir(), "stash.sqlite")))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

// staleImageStatistics gathers statistics while 2,000 of 20,000 images all have
// the same studio, has the read connections load them, then spreads those
// images over 200 studios. The number of images doesn't change, so the
// automatic optimize never notices.
func staleImageStatistics(t *testing.T) *Database {
	t.Helper()
	db := openTestDatabase(t)
	write(t, db, addStudiosAndImages)
	_, err := db.writeDB.Exec("ANALYZE")
	require.NoError(t, err)
	require.Equal(t, "2000 2000", imageStudioIndexStat(t, db), "every image with a studio has the same one")

	write(t, db, "UPDATE images SET studio_id = 1 + id % 200 WHERE studio_id IS NOT NULL")
	require.NotContains(t, imageStudioCountPlan(t, db), lookUpStudio, "stale statistics: the studio index isn't used for lookups")
	return db
}

// #2030: after a refresh, the statistics are right and the connections that
// were already open use them.
func TestRefreshStatistics(t *testing.T) {
	db := staleImageStatistics(t)

	require.NoError(t, db.RefreshStatistics(context.Background()))

	fields := strings.Fields(imageStudioIndexStat(t, db))
	require.Len(t, fields, 2)
	require.Equal(t, "2000", fields[0])
	require.NotEqual(t, "2000", fields[1], "about 10 images per studio, sampled so not exact")
	require.Contains(t, imageStudioCountPlan(t, db), lookUpStudio)

	// The sample limit must not stay on the write connection, or a later full
	// ANALYZE (Tasks -> Optimise Database) would be sampled too.
	var limit int
	require.NoError(t, db.writeDB.Get(&limit, "PRAGMA analysis_limit"))
	require.Equal(t, 0, limit)
}

// The full ANALYZE of Tasks -> Optimise Database used to update the statistics
// without the open connections using them.
func TestAnalyzeReachesOpenConnections(t *testing.T) {
	db := staleImageStatistics(t)

	require.NoError(t, db.Analyze(context.Background()))

	require.Equal(t, "2000 10", imageStudioIndexStat(t, db))
	require.Contains(t, imageStudioCountPlan(t, db), lookUpStudio)
}

// After Tasks -> Optimise Database, the refresh when that task ends must not
// replace its full statistics with sampled ones. Anything written afterwards
// is still refreshed.
func TestRefreshStatisticsKeepsFullAnalyze(t *testing.T) {
	ctx := context.Background()
	db := staleImageStatistics(t)
	require.NoError(t, db.Analyze(ctx))
	full := imageStat4Rows(t, db)
	require.NotZero(t, full, "a full ANALYZE keeps detailed samples")

	require.NoError(t, db.RefreshStatistics(ctx))
	require.Equal(t, full, imageStat4Rows(t, db), "a sampled ANALYZE would have removed them")
	require.Equal(t, "2000 10", imageStudioIndexStat(t, db))

	write(t, db, "UPDATE images SET studio_id = 1 WHERE studio_id = 2")
	require.NoError(t, db.RefreshStatistics(ctx))
	require.Zero(t, imageStat4Rows(t, db), "refreshed after the write")
}

// Import with "Reset" starts a new database, which runs a full ANALYZE while
// it's empty, then fills it. The refresh at the end must still run.
func TestRefreshStatisticsAfterReset(t *testing.T) {
	ctx := context.Background()
	db := openTestDatabase(t)
	require.NoError(t, db.Reset())

	write(t, db, addStudiosAndImages)
	require.NoError(t, db.RefreshStatistics(ctx))

	// skipped, there would be no statistics for the index at all
	var n int
	require.NoError(t, db.readDB.Get(&n, "SELECT COUNT(*) FROM sqlite_stat1 WHERE idx = 'index_images_on_studio_id'"))
	require.Equal(t, 1, n)
}

func TestRefreshStatisticsNotOpen(t *testing.T) {
	require.Error(t, NewDatabase().RefreshStatistics(context.Background()))
}
