//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2180: the scene, image and marker counts of tags with sub-tags, pinned on a
// small tag tree. C has two parents (B and D), so A reaches C twice, and some
// items have more than one tag under the same tag: each item still counts
// once.
func TestTagQueryHierarchicalCounts(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		tqb := db.Tag()
		names := []string{"A", "B", "C", "D", "E", "F", "G"}
		tag := map[string]int{}
		for _, n := range names {
			tg := models.Tag{Name: "hierarchical count " + n}
			require.NoError(t, tqb.Create(ctx, &models.CreateTagInput{Tag: &tg}))
			tag[n] = tg.ID
		}
		// A -> B -> C, A -> D -> C, F -> G; E stands alone
		for child, parents := range map[string][]string{"B": {"A"}, "D": {"A"}, "C": {"B", "D"}, "G": {"F"}} {
			var ids []int
			for _, p := range parents {
				ids = append(ids, tag[p])
			}
			require.NoError(t, tqb.UpdateParentTags(ctx, tag[child], ids))
		}
		tagIDs := func(ns ...string) []int {
			var ret []int
			for _, n := range ns {
				ret = append(ret, tag[n])
			}
			return ret
		}

		var scenes []int
		for _, ts := range [][]string{{"B", "C"}, {"C"}, {"D"}, {"A"}, {"G"}} {
			s := models.Scene{TagIDs: models.NewRelatedIDs(tagIDs(ts...))}
			require.NoError(t, db.Scene().Create(ctx, &s, nil))
			scenes = append(scenes, s.ID)
		}
		for _, ts := range [][]string{{"A", "C"}, {"G"}} {
			i := models.Image{TagIDs: models.NewRelatedIDs(tagIDs(ts...))}
			require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: &i}))
		}
		// primary tag, other tags
		for i, m := range []struct {
			primary string
			tags    []string
		}{{"C", []string{"B"}}, {"A", nil}, {"E", []string{"D"}}, {"C", nil}} {
			marker := models.SceneMarker{SceneID: scenes[i], PrimaryTagID: tag[m.primary]}
			require.NoError(t, db.SceneMarker().Create(ctx, &marker))
			if len(m.tags) > 0 {
				require.NoError(t, db.SceneMarker().UpdateTags(ctx, marker.ID, tagIDs(m.tags...)))
			}
		}

		type counts map[string]int
		type countFilter func(*models.HierarchicalCountInput) *models.TagFilterType
		var (
			sceneCount countFilter = func(c *models.HierarchicalCountInput) *models.TagFilterType {
				return &models.TagFilterType{SceneCount: c}
			}
			imageCount countFilter = func(c *models.HierarchicalCountInput) *models.TagFilterType {
				return &models.TagFilterType{ImageCount: c}
			}
			markerCount countFilter = func(c *models.HierarchicalCountInput) *models.TagFilterType {
				return &models.TagFilterType{MarkerCount: c}
			}
		)
		tests := []struct {
			name   string
			filter countFilter
			depth  int
			want   counts
		}{
			{"scenes all", sceneCount, -1, counts{"A": 4, "B": 2, "C": 2, "D": 3, "E": 0, "F": 1, "G": 1}},
			{"scenes depth 1", sceneCount, 1, counts{"A": 3, "B": 2, "C": 2, "D": 3, "E": 0, "F": 1, "G": 1}},
			{"scenes depth 0", sceneCount, 0, counts{"A": 1, "B": 1, "C": 2, "D": 1, "E": 0, "F": 0, "G": 1}},
			{"images all", imageCount, -1, counts{"A": 1, "B": 1, "C": 1, "D": 1, "E": 0, "F": 1, "G": 1}},
			{"markers all", markerCount, -1, counts{"A": 4, "B": 2, "C": 2, "D": 3, "E": 1, "F": 0, "G": 0}},
			{"markers depth 1", markerCount, 1, counts{"A": 3, "B": 2, "C": 2, "D": 3, "E": 1, "F": 0, "G": 0}},
		}

		conditions := []struct {
			modifier models.CriterionModifier
			value    int
			value2   int
			match    func(n int) bool
		}{
			{models.CriterionModifierEquals, 2, 0, func(n int) bool { return n == 2 }},
			{models.CriterionModifierNotEquals, 2, 0, func(n int) bool { return n != 2 }},
			{models.CriterionModifierGreaterThan, 1, 0, func(n int) bool { return n > 1 }},
			{models.CriterionModifierLessThan, 2, 0, func(n int) bool { return n < 2 }},
			{models.CriterionModifierEquals, 0, 0, func(n int) bool { return n == 0 }},
			{models.CriterionModifierBetween, 1, 2, func(n int) bool { return n >= 1 && n <= 2 }},
			{models.CriterionModifierNotBetween, 1, 2, func(n int) bool { return n < 1 || n > 2 }},
		}

		for _, tt := range tests {
			for _, c := range conditions {
				name := fmt.Sprintf("%s %s %d", tt.name, c.modifier, c.value)
				t.Run(name, func(t *testing.T) {
					depth := tt.depth
					input := &models.HierarchicalCountInput{Value: c.value, Modifier: c.modifier, Depth: &depth}
					if c.modifier == models.CriterionModifierBetween || c.modifier == models.CriterionModifierNotBetween {
						input.Value2 = &c.value2
					}
					tags, _, err := tqb.Query(ctx, tt.filter(input), allItems())
					require.NoError(t, err)

					// only look at the tags of this test
					found := map[int]bool{}
					for _, tg := range tags {
						found[tg.ID] = true
					}
					var got, want []string
					for _, n := range names {
						if found[tag[n]] {
							got = append(got, n)
						}
						if c.match(tt.want[n]) {
							want = append(want, n)
						}
					}
					assert.Equal(t, want, got)
				})
			}
		}
		return nil
	})
}
