//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2217: a loop of parent studios, or groups that contain each other, made
// the studio and group walks go round the loop until something stopped them.
// Each walk must end, and find each studio or group once.
func TestStudioAndGroupHierarchyLoop(t *testing.T) {
	// studios: A and B are each other's parent, C's parent is B
	// groups: GA and GB contain each other, GB contains GC
	type loopData struct {
		studio    map[string]int
		group     map[string]int
		scene     map[string]int
		performer int
	}
	exec := func(t *testing.T, ctx context.Context, q string) {
		_, _, err := db.ExecSQL(ctx, q, nil)
		require.NoError(t, err)
	}
	setup := func(t *testing.T, ctx context.Context) loopData {
		d := loopData{studio: map[string]int{}, group: map[string]int{}, scene: map[string]int{}}
		for _, n := range []string{"A", "B", "C"} {
			s := models.Studio{Name: "hierarchy loop studio " + n}
			require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: &s}))
			d.studio[n] = s.ID
		}
		// the API refuses these loops, so they are written directly, as an
		// import or an old database could have them
		exec(t, ctx, fmt.Sprintf("UPDATE studios SET parent_id = %d WHERE id = %d", d.studio["B"], d.studio["A"]))
		exec(t, ctx, fmt.Sprintf("UPDATE studios SET parent_id = %d WHERE id = %d", d.studio["A"], d.studio["B"]))
		exec(t, ctx, fmt.Sprintf("UPDATE studios SET parent_id = %d WHERE id = %d", d.studio["B"], d.studio["C"]))

		for _, n := range []string{"GA", "GB", "GC"} {
			g := models.Group{Name: "hierarchy loop group " + n}
			require.NoError(t, db.Group().Create(ctx, &g))
			d.group[n] = g.ID
		}
		for i, r := range [][2]string{{"GA", "GB"}, {"GB", "GA"}, {"GB", "GC"}} {
			exec(t, ctx, fmt.Sprintf("INSERT INTO groups_relations (containing_id, sub_id, order_index) VALUES (%d, %d, %d)", d.group[r[0]], d.group[r[1]], i))
		}

		p := models.Performer{Name: "hierarchy loop performer"}
		require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
		d.performer = p.ID

		// a scene in studio C and group GC with the performer, one in studio A
		c := models.Scene{
			Title:        "hierarchy loop scene c",
			StudioID:     &[]int{d.studio["C"]}[0],
			Groups:       models.NewRelatedGroups([]models.GroupsScenes{{GroupID: d.group["GC"]}}),
			PerformerIDs: models.NewRelatedIDs([]int{p.ID}),
		}
		require.NoError(t, db.Scene().Create(ctx, &c, nil))
		d.scene["c"] = c.ID
		a := models.Scene{Title: "hierarchy loop scene a", StudioID: &[]int{d.studio["A"]}[0]}
		require.NoError(t, db.Scene().Create(ctx, &a, nil))
		d.scene["a"] = a.ID
		_, err := db.Scene().AddO(ctx, c.ID, []time.Time{time.Now()})
		require.NoError(t, err)
		au := models.Audio{Title: "hierarchy loop audio", StudioID: &[]int{d.studio["C"]}[0]}
		require.NoError(t, db.Audio().Create(ctx, &au, nil))
		_, err = db.Audio().AddO(ctx, au.ID, []time.Time{time.Now()})
		require.NoError(t, err)
		return d
	}

	all := -1
	includes := func(id int) *models.HierarchicalMultiCriterionInput {
		return &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(id)}, Modifier: models.CriterionModifierIncludes, Depth: &all}
	}
	tests := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, d loopData)
	}{
		{"scene o-count of a studio and all sub-studios", func(t *testing.T, ctx context.Context, d loopData) {
			n, err := db.Scene().OCountByStudioID(ctx, d.studio["A"], all)
			require.NoError(t, err)
			assert.Equal(t, 1, n)
		}},
		{"image o-count of a studio and all sub-studios", func(t *testing.T, ctx context.Context, d loopData) {
			n, err := db.Image().OCountByStudioID(ctx, d.studio["A"], all)
			require.NoError(t, err)
			assert.Equal(t, 0, n)
		}},
		{"gallery o-count of a studio and all sub-studios", func(t *testing.T, ctx context.Context, d loopData) {
			n, err := db.Gallery().OCountByStudioID(ctx, d.studio["A"], all)
			require.NoError(t, err)
			assert.Equal(t, 0, n)
		}},
		{"audio o-count of a studio and all sub-studios", func(t *testing.T, ctx context.Context, d loopData) {
			n, err := db.Audio().OCountByStudioID(ctx, d.studio["A"], all)
			require.NoError(t, err)
			assert.Equal(t, 1, n)
		}},
		{"studios sorted by counts with sub-studios", func(t *testing.T, ctx context.Context, d loopData) {
			for _, sort := range []string{"scenes_count_all", "performer_count_all", "o_counter_all"} {
				studios, _, err := db.Studio().Query(ctx, nil, &models.FindFilterType{Sort: &sort, PerPage: &all})
				require.NoError(t, err, sort)
				var ids []int
				for _, s := range studios {
					ids = append(ids, s.ID)
				}
				assert.Subset(t, ids, []int{d.studio["A"], d.studio["B"], d.studio["C"]}, sort)
			}
		}},
		{"scenes in a studio and all sub-studios", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.SceneFilterType{Studios: includes(d.studio["A"])}
			res, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: f})
			require.NoError(t, err)
			assert.ElementsMatch(t, []int{d.scene["a"], d.scene["c"]}, res.IDs)
		}},
		{"scenes in a group and all sub-groups", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.SceneFilterType{Groups: includes(d.group["GA"])}
			res, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: f})
			require.NoError(t, err)
			assert.ElementsMatch(t, []int{d.scene["c"]}, res.IDs)
		}},
		{"groups in a group, all levels", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.GroupFilterType{ContainingGroups: includes(d.group["GA"])}
			groups, _, err := db.Group().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			var ids []int
			for _, g := range groups {
				ids = append(ids, g.ID)
			}
			// GA is inside GB, which is inside GA
			assert.ElementsMatch(t, []int{d.group["GA"], d.group["GB"], d.group["GC"]}, ids)
		}},
		{"performers in a group and all sub-groups", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.PerformerFilterType{Groups: includes(d.group["GA"])}
			performers, _, err := db.Performer().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			var ids []int
			for _, p := range performers {
				ids = append(ids, p.ID)
			}
			assert.Equal(t, []int{d.performer}, ids)
		}},
	}

	for _, tt := range tests {
		// each in its own short transaction: a walk that doesn't end is
		// stopped after a few seconds
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			d := setup(t, ctx)
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			tt.run(t, ctx, d)
		})
	}
}
