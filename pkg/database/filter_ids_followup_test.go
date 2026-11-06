//go:build integration
// +build integration

package database_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The marker Tags filter with the "excludes" modifier and no excludes list
// must leave out markers with that tag, as the scene Tags filter does.
func TestSceneMarkerTagsExcludesModifier(t *testing.T) {
	for _, depth := range []int{0, -1} {
		d := depth
		runWithRollbackTxn(t, "depth "+strconv.Itoa(d), func(t *testing.T, ctx context.Context) {
			var tags []int
			for _, n := range []string{"T1", "T2"} {
				tg := models.Tag{Name: "marker excludes " + n}
				require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tg}))
				tags = append(tags, tg.ID)
			}
			s := models.Scene{Title: "marker excludes scene"}
			require.NoError(t, db.Scene().Create(ctx, &s, nil))
			var markers []int
			for _, tg := range tags {
				m := &models.SceneMarker{Title: "marker excludes", SceneID: s.ID, PrimaryTagID: tg}
				require.NoError(t, db.SceneMarker().Create(ctx, m))
				markers = append(markers, m.ID)
			}

			ms, _, err := db.SceneMarker().Query(ctx, &models.SceneMarkerFilterType{Tags: &models.HierarchicalMultiCriterionInput{
				Value: []string{strconv.Itoa(tags[0])}, Modifier: models.CriterionModifierExcludes, Depth: &d,
			}}, perPageAll())
			require.NoError(t, err)
			var ids []int
			for _, m := range ms {
				ids = append(ids, m.ID)
			}
			assert.NotContains(t, ids, markers[0], "the marker with the excluded tag is left out")
			assert.Contains(t, ids, markers[1])
		})
	}
}

// An id too large for 64 bits is no row's id. It matches nothing on both
// databases, as on PostgreSQL since #2245, instead of being refused. An id
// that isn't a number is still refused.
func TestFilterIDsBeyond64Bits(t *testing.T) {
	const huge = "99999999999999999999"
	all := -1
	zero := 0

	runWithRollbackTxn(t, "filters", func(t *testing.T, ctx context.Context) {
		tg := models.Tag{Name: "beyond 64 bits tag"}
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tg}))
		var pids []int
		for _, n := range []string{"A", "B"} {
			p := models.Performer{Name: "beyond 64 bits " + n}
			require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
			pids = append(pids, p.ID)
		}
		s := models.Scene{Title: "beyond 64 bits scene", TagIDs: models.NewRelatedIDs([]int{tg.ID}), PerformerIDs: models.NewRelatedIDs(pids)}
		require.NoError(t, db.Scene().Create(ctx, &s, nil))

		scenes := func(f *models.SceneFilterType) ([]int, error) {
			res, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: f})
			if err != nil {
				return nil, err
			}
			return res.IDs, nil
		}
		performers := func(f *models.PerformerFilterType) ([]int, error) {
			ps, _, err := db.Performer().Query(ctx, f, perPageAll())
			var ids []int
			for _, p := range ps {
				ids = append(ids, p.ID)
			}
			return ids, err
		}

		none := map[string]func() ([]int, error){
			"scenes by tags": func() ([]int, error) {
				return scenes(&models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{huge}, Modifier: models.CriterionModifierIncludes, Depth: &zero}})
			},
			"scenes by tags, sub-tags": func() ([]int, error) {
				return scenes(&models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{huge}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			},
			"performers they appear with": func() ([]int, error) {
				return performers(&models.PerformerFilterType{Performers: &models.MultiCriterionInput{Value: []string{huge}, Modifier: models.CriterionModifierIncludes}})
			},
			"performers by groups": func() ([]int, error) {
				return performers(&models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{Value: []string{huge}, Modifier: models.CriterionModifierIncludes, Depth: &zero}})
			},
			"performers by groups, sub-groups": func() ([]int, error) {
				return performers(&models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{Value: []string{huge}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
			},
		}
		for name, query := range none {
			ids, err := query()
			if assert.NoError(t, err, name) {
				assert.Empty(t, ids, name)
			}
		}

		// in an excludes list it leaves nothing out
		ids, err := performers(&models.PerformerFilterType{Performers: &models.MultiCriterionInput{Value: []string{strconv.Itoa(pids[0])}, Excludes: []string{huge}, Modifier: models.CriterionModifierIncludes}})
		if assert.NoError(t, err) {
			assert.Contains(t, ids, pids[1])
		}
		ids, err = scenes(&models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(tg.ID)}, Excludes: []string{huge}, Modifier: models.CriterionModifierIncludes, Depth: &all}})
		if assert.NoError(t, err) {
			assert.Contains(t, ids, s.ID)
		}

		// a value that isn't a number is still refused
		_, err = performers(&models.PerformerFilterType{Performers: &models.MultiCriterionInput{Value: []string{"1x"}, Modifier: models.CriterionModifierIncludes}})
		assert.ErrorContains(t, err, "invalid id")
	})
}

// The id number filters compare a value outside the 32-bit range as SQLite
// does, instead of failing on PostgreSQL: no id is greater than it, every id
// is less than it.
func TestIDNumberFiltersOutOfRange(t *testing.T) {
	const big = 3000000000
	two := big
	one := 1

	type idQuery func(ctx context.Context, c *models.IntCriterionInput) ([]int, error)
	objects := []struct {
		name   string
		create func(ctx context.Context) int
		query  idQuery
	}{
		{"scenes", func(ctx context.Context) int {
			s := models.Scene{Title: "id range scene"}
			require.NoError(t, db.Scene().Create(ctx, &s, nil))
			return s.ID
		}, func(ctx context.Context, c *models.IntCriterionInput) ([]int, error) {
			res, err := db.Scene().Query(ctx, models.SceneQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, SceneFilter: &models.SceneFilterType{ID: c}})
			if err != nil {
				return nil, err
			}
			return res.IDs, nil
		}},
		{"images", func(ctx context.Context) int {
			o := &models.Image{Title: "id range image"}
			require.NoError(t, db.Image().Create(ctx, &models.CreateImageInput{Image: o}))
			return o.ID
		}, func(ctx context.Context, c *models.IntCriterionInput) ([]int, error) {
			res, err := db.Image().Query(ctx, models.ImageQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, ImageFilter: &models.ImageFilterType{ID: c}})
			if err != nil {
				return nil, err
			}
			return res.IDs, nil
		}},
		{"galleries", func(ctx context.Context) int {
			g := models.Gallery{Title: "id range gallery"}
			require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &g}))
			return g.ID
		}, func(ctx context.Context, c *models.IntCriterionInput) ([]int, error) {
			gs, _, err := db.Gallery().Query(ctx, &models.GalleryFilterType{ID: c}, perPageAll())
			var ids []int
			for _, g := range gs {
				ids = append(ids, g.ID)
			}
			return ids, err
		}},
		{"audios", func(ctx context.Context) int {
			a := models.Audio{Title: "id range audio"}
			require.NoError(t, db.Audio().Create(ctx, &a, nil))
			return a.ID
		}, func(ctx context.Context, c *models.IntCriterionInput) ([]int, error) {
			res, err := db.Audio().Query(ctx, models.AudioQueryOptions{QueryOptions: models.QueryOptions{FindFilter: perPageAll()}, AudioFilter: &models.AudioFilterType{ID: c}})
			if err != nil {
				return nil, err
			}
			return res.IDs, nil
		}},
	}

	cases := []struct {
		criterion models.IntCriterionInput
		found     bool
	}{
		{models.IntCriterionInput{Value: big, Modifier: models.CriterionModifierGreaterThan}, false},
		{models.IntCriterionInput{Value: big, Modifier: models.CriterionModifierEquals}, false},
		{models.IntCriterionInput{Value: big, Modifier: models.CriterionModifierLessThan}, true},
		{models.IntCriterionInput{Value: big, Modifier: models.CriterionModifierNotEquals}, true},
		{models.IntCriterionInput{Value: one, Value2: &two, Modifier: models.CriterionModifierBetween}, true},
		{models.IntCriterionInput{Value: one, Value2: &two, Modifier: models.CriterionModifierNotBetween}, false},
		{models.IntCriterionInput{Value: -big, Modifier: models.CriterionModifierGreaterThan}, true},
	}

	for _, o := range objects {
		for _, c := range cases {
			name := o.name + " " + string(c.criterion.Modifier)
			crit := c.criterion
			runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
				id := o.create(ctx)
				ids, err := o.query(ctx, &crit)
				require.NoError(t, err)
				if c.found {
					assert.Contains(t, ids, id)
				} else {
					assert.Empty(t, ids)
				}
			})
		}
	}
}
