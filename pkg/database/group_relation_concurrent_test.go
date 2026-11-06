//go:build integration

package database_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/group"
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
		runTxn(t, func(ctx context.Context) error {
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

	// a bulk edit goes through its groups in the order given, so it must not
	// lock them one by one: a create inside two of them, at the same moment,
	// would lock them in another order and deadlock (40P01)
	t.Run("bulk edit in descending order with creates", func(t *testing.T) {
		svc := &group.Service{Repository: db.Group()}
		for round := 0; round < 6; round++ {
			g := newGroups(5, fmt.Sprintf("concurrent bulk desc %d", round))
			leaf := g[0]
			created := func(a, b int) func(ctx context.Context) error {
				return func(ctx context.Context) error {
					n := models.NewGroup()
					n.Name = fmt.Sprintf("concurrent bulk desc new %d", round)
					n.ContainingGroups = models.NewRelatedGroupDescriptions(groupDescriptions(a, b))
					// as groupCreate gives it, with no sub-groups
					n.SubGroups = models.NewRelatedGroupDescriptions([]models.GroupIDDescription{})
					if err := svc.Create(ctx, &models.CreateGroupInput{Group: &n}); err != nil {
						return err
					}
					track(n.ID)
					return nil
				}
			}
			edits := []func(ctx context.Context) error{
				// as bulkGroupUpdate does, one group at a time, in one transaction
				func(ctx context.Context) error {
					for _, id := range []int{g[4], g[3], g[2], g[1]} {
						p := models.NewGroupPartial()
						p.SubGroups = &models.UpdateGroupDescriptions{Groups: groupDescriptions(leaf), Mode: models.RelationshipUpdateModeAdd}
						if _, err := svc.UpdatePartial(ctx, id, p, group.ImageInput{}, group.ImageInput{}); err != nil {
							return err
						}
					}
					return nil
				},
				created(g[4], g[3]),
				created(g[3], g[2]),
				created(g[2], g[1]),
				created(g[4], g[1]),
			}

			for i, err := range runAtOnce(edits) {
				assert.NoError(t, err, "round %d edit %d", round, i)
			}
			for _, id := range g[1:] {
				subs, orderIndexes := subGroupOrder(t, id)
				assert.Len(t, subs, 3, "round %d group %d", round, id)
				assert.Equal(t, []int{1, 2, 3}, orderIndexes, "round %d group %d", round, id)
			}
		}
	})

	// two edits at once that would together put groups inside each other: one
	// must be refused with the usual message, as one at a time (#2113)
	t.Run("loops", func(t *testing.T) {
		svc := &group.Service{Repository: db.Group()}
		containedBy := func(id, containing int) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				p := models.NewGroupPartial()
				p.ContainingGroups = &models.UpdateGroupDescriptions{Groups: groupDescriptions(containing), Mode: models.RelationshipUpdateModeAdd}
				_, err := svc.UpdatePartial(ctx, id, p, group.ImageInput{}, group.ImageInput{})
				return err
			}
		}
		contains := func(id, sub int) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				p := models.NewGroupPartial()
				p.SubGroups = &models.UpdateGroupDescriptions{Groups: groupDescriptions(sub), Mode: models.RelationshipUpdateModeAdd}
				_, err := svc.UpdatePartial(ctx, id, p, group.ImageInput{}, group.ImageInput{})
				return err
			}
		}
		addSub := func(id, sub int) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				return svc.AddSubGroups(ctx, id, groupDescriptions(sub), nil)
			}
		}
		create := func(containing, sub int) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				g := models.NewGroup()
				g.Name = "concurrent loop new"
				g.ContainingGroups = models.NewRelatedGroupDescriptions(groupDescriptions(containing))
				g.SubGroups = models.NewRelatedGroupDescriptions(groupDescriptions(sub))
				if err := svc.Create(ctx, &models.CreateGroupInput{Group: &g}); err != nil {
					return err
				}
				track(g.ID)
				return nil
			}
		}

		tests := []struct {
			name string
			// edits for groups a, b and c; a already contains b
			edits func(a, b, c int) []func(ctx context.Context) error
		}{
			{"containing groups", func(a, b, c int) []func(ctx context.Context) error {
				return []func(ctx context.Context) error{containedBy(c, b), containedBy(a, c)}
			}},
			{"sub-groups", func(a, b, c int) []func(ctx context.Context) error {
				return []func(ctx context.Context) error{contains(b, c), contains(c, a)}
			}},
			{"addGroupSubGroups", func(a, b, c int) []func(ctx context.Context) error {
				return []func(ctx context.Context) error{addSub(b, c), addSub(c, a)}
			}},
			{"each way round", func(a, b, c int) []func(ctx context.Context) error {
				return []func(ctx context.Context) error{containedBy(b, c), contains(b, c)}
			}},
			{"a new group", func(a, b, c int) []func(ctx context.Context) error {
				// a new group inside b that contains c, while b is put inside c
				return []func(ctx context.Context) error{create(b, c), containedBy(b, c)}
			}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				for round := 0; round < 5; round++ {
					g := newGroups(3, fmt.Sprintf("concurrent loop %s %d", tt.name, round))
					a, b, c := g[0], g[1], g[2]
					require.NoError(t, withTxn(func(ctx context.Context) error {
						return updateGroupLinks(ctx, a, false, models.RelationshipUpdateModeAdd, b)
					}))

					refused := 0
					for i, err := range runAtOnce(tt.edits(a, b, c)) {
						if err != nil {
							refused++
							assert.ErrorIs(t, err, group.ErrHierarchyLoop, "round %d edit %d", round, i)
						}
					}
					assert.Equal(t, 1, refused, "round %d", round)
				}
			})
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
