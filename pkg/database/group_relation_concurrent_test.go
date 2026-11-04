//go:build integration

package database_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// concurrentEdits is the number of edits run at once: the size of the
// PostgreSQL write connection pool.
const concurrentEdits = 5

// runAtOnce runs each edit in its own transaction, all at the same moment, and
// returns their errors. On PostgreSQL every edit's transaction is open before
// any edit starts. SQLite has one write transaction at a time, so there they
// run one after another.
func runAtOnce(edits []func(ctx context.Context) error) []error {
	errs := make([]error, len(edits))
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	waitForOthers := IsPostgresTest() != nil

	ready.Add(len(edits))
	done.Add(len(edits))
	for i, edit := range edits {
		go func() {
			defer done.Done()
			errs[i] = withTxn(func(ctx context.Context) error {
				ready.Done()
				if waitForOthers {
					<-start
				}
				return edit(ctx)
			})
		}()
	}

	if waitForOthers {
		ready.Wait()
		close(start)
	}
	done.Wait()

	return errs
}

// subGroupOrder returns the sub-group ids and order indexes of a containing
// group, in order.
func subGroupOrder(t *testing.T, containingID int) (subIDs []int, orderIndexes []int) {
	t.Helper()
	require.NoError(t, withTxn(func(ctx context.Context) error {
		_, rows, err := db.QuerySQL(ctx, fmt.Sprintf("SELECT sub_id, order_index FROM groups_relations WHERE containing_id = %d ORDER BY order_index", containingID), nil)
		if err != nil {
			return err
		}
		for _, r := range rows {
			subIDs = append(subIDs, toInt(t, r[0]))
			orderIndexes = append(orderIndexes, toInt(t, r[1]))
		}
		return nil
	}))
	return subIDs, orderIndexes
}

func toInt(t *testing.T, v interface{}) int {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return int(n)
	case int32:
		return int(n)
	case int:
		return n
	}
	t.Fatalf("unexpected %T %v", v, v)
	return 0
}

// TestGroupRelationConcurrentEdits runs edits to one containing group's
// sub-groups at the same moment. On PostgreSQL they used to pick the same
// order_index, and all but one were refused by
// index_groups_relations_order_index_unique (#2100). They must all succeed and
// leave the order a one-at-a-time run would.
func TestGroupRelationConcurrentEdits(t *testing.T) {
	var mu sync.Mutex
	var made []int
	track := func(id int) {
		mu.Lock()
		defer mu.Unlock()
		made = append(made, id)
	}
	newGroups := func(n int, name string) []int {
		ret := make([]int, n)
		require.NoError(t, withTxn(func(ctx context.Context) error {
			for i := range ret {
				g := models.NewGroup()
				g.Name = fmt.Sprintf("%s %d", name, i)
				if err := db.Group().Create(ctx, &g); err != nil {
					return err
				}
				ret[i] = g.ID
				track(g.ID)
			}
			return nil
		}))
		return ret
	}
	t.Cleanup(func() {
		_ = withTxn(func(ctx context.Context) error {
			for _, id := range made {
				if err := db.Group().Destroy(ctx, id); err != nil {
					return err
				}
			}
			return nil
		})
	})

	addContaining := func(subID, containing int) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return updateGroupLinks(ctx, subID, true, models.RelationshipUpdateModeAdd, containing)
		}
	}

	t.Run("adds", func(t *testing.T) {
		containing := newGroups(1, "concurrent containing")[0]
		const rounds = 6
		var want []int
		for round := 0; round < rounds; round++ {
			subs := newGroups(concurrentEdits, fmt.Sprintf("concurrent add %d", round))
			edits := []func(ctx context.Context) error{
				// as bulkGroupUpdate's containing_groups ADD does
				addContaining(subs[0], containing),
				addContaining(subs[1], containing),
				// groupUpdate's sub_groups ADD
				func(ctx context.Context) error {
					return updateGroupLinks(ctx, containing, false, models.RelationshipUpdateModeAdd, subs[2])
				},
				// addGroupSubGroups
				func(ctx context.Context) error {
					return db.Group().AddSubGroups(ctx, containing, groupDescriptions(subs[3]), nil)
				},
				// groupCreate with a containing group
				func(ctx context.Context) error {
					g := models.NewGroup()
					g.Name = fmt.Sprintf("concurrent create %d", round)
					g.ContainingGroups = models.NewRelatedGroupDescriptions(groupDescriptions(containing))
					if err := db.Group().Create(ctx, &g); err != nil {
						return err
					}
					track(g.ID)
					return nil
				},
			}

			for i, err := range runAtOnce(edits) {
				assert.NoError(t, err, "round %d edit %d", round, i)
			}
			want = append(want, subs[:4]...)
		}

		subIDs, orderIndexes := subGroupOrder(t, containing)
		require.Len(t, subIDs, rounds*concurrentEdits)
		for i, oi := range orderIndexes {
			// one at a time, each is added at the end: 1, 2, 3 and so on
			assert.Equal(t, i+1, oi, "position %d", i)
		}
		for _, id := range want {
			assert.Contains(t, subIDs, id)
		}
	})

	// PostgreSQL ends one of two transactions that wait for each other with a
	// deadlock error, so these would fail if the locks could be taken in a
	// different order by different edits
	t.Run("overlapping edits", func(t *testing.T) {
		const rounds = 6
		for round := 0; round < rounds; round++ {
			c := newGroups(3, fmt.Sprintf("concurrent overlap containing %d", round))
			subs := newGroups(5, fmt.Sprintf("concurrent overlap sub %d", round))
			edits := []func(ctx context.Context) error{
				// a bulk edit of two groups...
				func(ctx context.Context) error {
					if err := updateGroupLinks(ctx, subs[0], true, models.RelationshipUpdateModeAdd, c[0]); err != nil {
						return err
					}
					return updateGroupLinks(ctx, subs[1], true, models.RelationshipUpdateModeAdd, c[0])
				},
				// ...and an edit of the second group alone, under the same
				// containing group
				func(ctx context.Context) error {
					return updateGroupLinks(ctx, subs[1], true, models.RelationshipUpdateModeRemove, c[0])
				},
				// containing groups given in opposite orders
				func(ctx context.Context) error {
					return updateGroupLinks(ctx, subs[2], true, models.RelationshipUpdateModeAdd, c[1], c[2], c[0])
				},
				func(ctx context.Context) error {
					return updateGroupLinks(ctx, subs[3], true, models.RelationshipUpdateModeAdd, c[0], c[2], c[1])
				},
				func(ctx context.Context) error {
					return db.Group().AddSubGroups(ctx, c[2], groupDescriptions(subs[4]), nil)
				},
			}

			for i, err := range runAtOnce(edits) {
				assert.NoError(t, err, "round %d edit %d", round, i)
			}

			// the removal is a no-op if it came first
			got, orderIndexes := subGroupOrder(t, c[0])
			if len(got) == 4 {
				assert.ElementsMatch(t, []int{subs[0], subs[1], subs[2], subs[3]}, got, "round %d", round)
				assert.Equal(t, []int{1, 2, 3, 4}, orderIndexes, "round %d", round)
			} else {
				assert.ElementsMatch(t, []int{subs[0], subs[2], subs[3]}, got, "round %d", round)
				for j := 1; j < len(orderIndexes); j++ {
					assert.Less(t, orderIndexes[j-1], orderIndexes[j], "round %d", round)
				}
			}

			for i, want := range map[int][]int{1: {subs[2], subs[3]}, 2: {subs[2], subs[3], subs[4]}} {
				got, orderIndexes := subGroupOrder(t, c[i])
				assert.ElementsMatch(t, want, got, "round %d containing %d", round, i)
				for j, oi := range orderIndexes {
					assert.Equal(t, j+1, oi, "round %d containing %d position %d", round, i, j)
				}
			}
		}
	})

	t.Run("reorders, removes and adds", func(t *testing.T) {
		// set up one at a time
		containing := newGroups(1, "concurrent mixed containing")[0]
		initial := newGroups(20, "concurrent mixed sub")
		require.NoError(t, withTxn(func(ctx context.Context) error {
			return updateGroupLinks(ctx, containing, false, models.RelationshipUpdateModeSet, initial...)
		}))
		before, _ := subGroupOrder(t, containing)
		require.Len(t, before, 20)
		first := before[0]

		const rounds = 4
		removed := map[int]bool{}
		var added []int
		for round := 0; round < rounds; round++ {
			moveA, moveB, remove := before[1+round*3], before[2+round*3], before[3+round*3]
			subs := newGroups(2, fmt.Sprintf("concurrent mixed %d", round))
			edits := []func(ctx context.Context) error{
				func(ctx context.Context) error {
					return db.Group().ReorderSubGroups(ctx, containing, []int{moveA}, first, false)
				},
				func(ctx context.Context) error {
					return db.Group().ReorderSubGroups(ctx, containing, []int{moveB}, first, true)
				},
				func(ctx context.Context) error {
					return db.Group().RemoveSubGroups(ctx, containing, []int{remove})
				},
				addContaining(subs[0], containing),
				func(ctx context.Context) error {
					return db.Group().AddSubGroups(ctx, containing, groupDescriptions(subs[1]), nil)
				},
			}

			for i, err := range runAtOnce(edits) {
				assert.NoError(t, err, "round %d edit %d", round, i)
			}
			removed[remove] = true
			added = append(added, subs...)
		}

		after, orderIndexes := subGroupOrder(t, containing)
		var want []int
		for _, id := range before {
			if !removed[id] {
				want = append(want, id)
			}
		}
		want = append(want, added...)
		assert.ElementsMatch(t, want, after)
		for i := 1; i < len(orderIndexes); i++ {
			assert.Less(t, orderIndexes[i-1], orderIndexes[i])
		}
	})
}
