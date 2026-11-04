//go:build integration

package sqlite

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

const randomSortStudios = 200

func randomSortDatabase(t *testing.T) *Database {
	t.Helper()
	_ = config.InitializeEmpty()
	db := openTestDatabase(t)
	write(t, db, fmt.Sprintf(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<%d)
INSERT INTO studios (id, name, created_at, updated_at) SELECT x, 'studio ' || x, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM n;`, randomSortStudios))
	return db
}

// queryStudioIDs runs the Studios page query with the given sort, one page.
func queryStudioIDs(t *testing.T, db *Database, sortBy string, page, perPage int) []int {
	t.Helper()
	var ids []int
	require.NoError(t, txn.WithReadTxn(context.Background(), db, func(ctx context.Context) error {
		studios, _, err := db.Studio().Query(ctx, nil, &models.FindFilterType{Sort: &sortBy, Page: &page, PerPage: &perPage})
		if err != nil {
			return err
		}
		for _, s := range studios {
			ids = append(ids, s.ID)
		}
		return nil
	}))
	return ids
}

func isIDOrder(ids []int) bool {
	return sort.IntsAreSorted(ids)
}

// #2110: the random sort gave id order whatever the seed.
func TestRandomSortSeedsGiveDifferentOrders(t *testing.T) {
	db := randomSortDatabase(t)

	a := queryStudioIDs(t, db, "random_12345", 1, randomSortStudios)
	b := queryStudioIDs(t, db, "random_67890", 1, randomSortStudios)
	require.Len(t, a, randomSortStudios)
	require.Len(t, b, randomSortStudios)

	assert.False(t, isIDOrder(a), "seed 12345 gave id order")
	assert.False(t, isIDOrder(b), "seed 67890 gave id order")
	assert.NotEqual(t, a, b, "two seeds gave the same order")
	assert.ElementsMatch(t, a, b, "both orders hold every studio once")

	// "random" with no seed picks one; it must not be id order either.
	assert.False(t, isIDOrder(queryStudioIDs(t, db, "random", 1, randomSortStudios)))
}

// The same seed gives the same order every time, and pages of it join up into
// the whole order, so paging through a shuffled list neither repeats nor skips.
func TestRandomSortSameSeedIsStable(t *testing.T) {
	db := randomSortDatabase(t)
	const sortBy = "random_26819649"

	whole := queryStudioIDs(t, db, sortBy, 1, randomSortStudios)
	require.Len(t, whole, randomSortStudios)
	assert.Equal(t, whole, queryStudioIDs(t, db, sortBy, 1, randomSortStudios), "a second run gave another order")

	const perPage = 30
	var paged []int
	for page := 1; len(paged) < randomSortStudios; page++ {
		got := queryStudioIDs(t, db, sortBy, page, perPage)
		require.NotEmpty(t, got)
		paged = append(paged, got...)
	}
	assert.Equal(t, whole, paged)
}

// upstreamRandomKey works out upstream's key,
// mod((n+seed)*(n+seed)*52959209 + (n+seed)*1047483763, 2147483647),
// step by step the way SQLite does: in whole numbers while they fit in 64
// bits, as reals once they don't, and mod() as fmod on reals.
func upstreamRandomKey(id int64, seed uint64) float64 {
	a := id + int64(seed%1e8)
	sq := a * a // a is at most about 10^8, so this fits
	lin := a * 1047483763

	var sum float64
	if sq <= math.MaxInt64/52959209 {
		prod := sq * 52959209
		if prod <= math.MaxInt64-lin {
			sum = float64(prod + lin)
		} else {
			sum = float64(prod) + float64(lin)
		}
	} else {
		sum = float64(sq)*52959209 + float64(lin)
	}
	return math.Mod(sum, 2147483647)
}

// The key SQLite sorts by is upstream's, so a seed gives the same order as in
// upstream Stash.
func TestRandomSortKeyMatchesUpstream(t *testing.T) {
	db := openTestDatabase(t)

	for _, seed := range []uint64{0, 1, 12345, 26819649, 99999999, 123456789012} {
		// small ids keep the sum in whole numbers for small seeds; the rest
		// go past 64 bits and are worked out as reals
		for _, id := range []int64{1, 2, 7, 150, 4096, 1000003} {
			// the ORDER BY expression, selected instead of sorted by
			orderBy := getRandomSort("x", "ASC", seed)
			expr := orderBy[len(" ORDER BY ") : len(orderBy)-len(" ASC")]

			var got float64
			require.NoError(t, db.readDB.Get(&got, "SELECT "+expr+" FROM (SELECT ? AS id) x", id), "seed %d id %d", seed, id)
			assert.Equal(t, upstreamRandomKey(id, seed), got, "seed %d id %d", seed, id)
			assert.Less(t, got, 2147483647.0, "seed %d id %d: the key is a remainder", seed, id)
		}
	}
}
