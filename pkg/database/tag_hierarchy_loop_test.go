//go:build integration
// +build integration

package database_test

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2208: the tag store accepts a loop of parents. Walking all sub-tags or all
// parent tags then went round the loop until something stopped it. Each walk
// must end, and find each tag once.
func TestTagHierarchyLoop(t *testing.T) {
	// Top -> A -> B -> A (the loop), B -> Leaf
	type loopData struct {
		tag    map[string]int
		scene  map[string]int
		marker int
	}
	setup := func(t *testing.T, ctx context.Context) loopData {
		d := loopData{tag: map[string]int{}, scene: map[string]int{}}
		tqb := db.Tag()
		for _, n := range []string{"Top", "A", "B", "Leaf"} {
			tg := models.Tag{Name: "hierarchy loop " + n}
			require.NoError(t, tqb.Create(ctx, &models.CreateTagInput{Tag: &tg}))
			d.tag[n] = tg.ID
		}
		require.NoError(t, tqb.UpdateParentTags(ctx, d.tag["A"], []int{d.tag["Top"], d.tag["B"]}))
		require.NoError(t, tqb.UpdateParentTags(ctx, d.tag["B"], []int{d.tag["A"]}))
		require.NoError(t, tqb.UpdateParentTags(ctx, d.tag["Leaf"], []int{d.tag["B"]}))

		for n, tg := range map[string]string{"leaf scene": "Leaf", "a scene": "A"} {
			s := models.Scene{Title: "hierarchy loop " + n, TagIDs: models.NewRelatedIDs([]int{d.tag[tg]})}
			require.NoError(t, db.Scene().Create(ctx, &s, nil))
			d.scene[n] = s.ID
		}
		m := models.SceneMarker{SceneID: d.scene["leaf scene"], PrimaryTagID: d.tag["Leaf"]}
		require.NoError(t, db.SceneMarker().Create(ctx, &m))
		d.marker = m.ID
		return d
	}

	all := -1
	// the tags of this test that the filter finds, by name
	tagNames := func(d loopData, tags []*models.Tag) []string {
		var ret []string
		for _, tg := range tags {
			for n, id := range d.tag {
				if tg.ID == id {
					ret = append(ret, n)
				}
			}
		}
		sort.Strings(ret)
		return ret
	}
	pathNames := func(d loopData, paths []*models.TagPath) []string {
		var tags []*models.Tag
		for _, p := range paths {
			tags = append(tags, &p.Tag)
		}
		return tagNames(d, tags)
	}

	tests := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, d loopData)
	}{
		{"scene count with all sub-tags", func(t *testing.T, ctx context.Context, d loopData) {
			// Top, A and B reach both scenes, Leaf only its own
			f := &models.TagFilterType{SceneCount: &models.HierarchicalCountInput{Modifier: models.CriterionModifierEquals, Value: 2, Depth: &all}}
			tags, _, err := db.Tag().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			assert.Equal(t, []string{"A", "B", "Top"}, tagNames(d, tags))
		}},
		{"marker count with all sub-tags", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.TagFilterType{MarkerCount: &models.HierarchicalCountInput{Modifier: models.CriterionModifierEquals, Value: 1, Depth: &all}}
			tags, _, err := db.Tag().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			assert.Equal(t, []string{"A", "B", "Leaf", "Top"}, tagNames(d, tags))
		}},
		{"tags with a parent, all levels", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.TagFilterType{Parents: &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(d.tag["Top"])}, Modifier: models.CriterionModifierIncludes, Depth: &all}}
			tags, _, err := db.Tag().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			assert.Equal(t, []string{"A", "B", "Leaf"}, tagNames(d, tags))
		}},
		{"tags with a child, all levels", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.TagFilterType{Children: &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(d.tag["Leaf"])}, Modifier: models.CriterionModifierIncludes, Depth: &all}}
			tags, _, err := db.Tag().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			assert.Equal(t, []string{"A", "B", "Top"}, tagNames(d, tags))
		}},
		{"scenes with a tag and all its sub-tags", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(d.tag["Top"])}, Modifier: models.CriterionModifierIncludes, Depth: &all}}
			res, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: f})
			require.NoError(t, err)
			assert.ElementsMatch(t, []int{d.scene["leaf scene"], d.scene["a scene"]}, res.IDs)
		}},
		{"markers with a tag and all its sub-tags", func(t *testing.T, ctx context.Context, d loopData) {
			f := &models.SceneMarkerFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(d.tag["A"])}, Modifier: models.CriterionModifierIncludes, Depth: &all}}
			markers, _, err := db.SceneMarker().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			var ids []int
			for _, m := range markers {
				ids = append(ids, m.ID)
			}
			assert.Equal(t, []int{d.marker}, ids)
		}},
		{"all descendants", func(t *testing.T, ctx context.Context, d loopData) {
			paths, err := db.Tag().FindAllDescendants(ctx, d.tag["Top"], nil)
			require.NoError(t, err)
			assert.Equal(t, []string{"A", "B", "Leaf", "Top"}, pathNames(d, paths))
		}},
		{"all ancestors", func(t *testing.T, ctx context.Context, d loopData) {
			paths, err := db.Tag().FindAllAncestors(ctx, d.tag["Leaf"], nil)
			require.NoError(t, err)
			assert.Equal(t, []string{"A", "B", "Leaf", "Top"}, pathNames(d, paths))
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
