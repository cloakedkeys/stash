//go:build integration
// +build integration

package database_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1206: ALL_OF lets one filter level hold several groups, such as
// (a OR b) AND (c OR d), which AND, OR and NOT alone cannot express.

// The test items have their index in their name, such as scene_0006_title or
// tag_6_name. Each pattern below picks the items with the listed indexes.
var (
	allOfA = indexRegex("01") // 0, 1
	allOfB = indexRegex("56") // 5, 6
	allOfC = indexRegex("12") // 1, 2
	allOfD = indexRegex("678")
	// (A or B) and (C or D)
	allOfAorBandCorD = indexRegex("16")
)

func indexRegex(indexes string) string {
	return fmt.Sprintf("_0*[%s]_", indexes)
}

type allOfCase[T any] struct {
	// returns a filter for items whose name matches the regex
	text  func(regex string) *T
	op    func(f *T) *models.OperatorFilter[T]
	query func(ctx context.Context, t *testing.T, f *T) idSet
}

func (c allOfCase[T]) orFilter(first, second string) *T {
	f := c.text(first)
	c.op(f).Or = c.text(second)
	return f
}

func (c allOfCase[T]) allOf(filters ...*T) *T {
	f := new(T)
	c.op(f).AllOf = filters
	return f
}

func (c allOfCase[T]) run(ctx context.Context, t *testing.T) {
	var empty T
	all := c.query(ctx, t, &empty)
	a := c.query(ctx, t, c.text(allOfA))
	b := c.query(ctx, t, c.text(allOfB))
	cc := c.query(ctx, t, c.text(allOfC))
	d := c.query(ctx, t, c.text(allOfD))
	for name, s := range map[string]idSet{"A": a, "B": b, "C": cc, "D": d} {
		require.NotEmpty(t, s, "pattern %s finds nothing", name)
	}

	check := func(name string, want idSet, f *T) {
		t.Helper()
		assert.Equal(t, want.sorted(), c.query(ctx, t, f).sorted(), name)
	}

	anyAB := setOr(a, b)
	anyCD := setOr(cc, d)
	both := setAnd(anyAB, anyCD)
	require.NotEmpty(t, both, "(A or B) and (C or D) finds nothing")

	check("ALL_OF [A, C]", setAnd(a, cc), c.allOf(c.text(allOfA), c.text(allOfC)))
	check("ALL_OF [A or B, C or D]", both, c.allOf(c.orFilter(allOfA, allOfB), c.orFilter(allOfC, allOfD)))
	check("same as the single pattern", c.query(ctx, t, c.text(allOfAorBandCorD)), c.allOf(c.orFilter(allOfA, allOfB), c.orFilter(allOfC, allOfD)))

	notCD := new(T)
	c.op(notCD).Not = c.orFilter(allOfC, allOfD)
	check("ALL_OF [A or B, NOT (C or D)]", setMinus(anyAB, anyCD), c.allOf(c.orFilter(allOfA, allOfB), notCD))

	// ALL_OF items count as conditions of their own level, so they are
	// ANDed with the other conditions there before OR is applied
	withOr := c.orFilter(allOfA, allOfB)
	c.op(withOr).AllOf = []*T{c.text(allOfC)}
	check("A, OR B, ALL_OF [C]", setOr(setAnd(a, cc), b), withOr)

	notGroups := new(T)
	c.op(notGroups).Not = c.allOf(c.orFilter(allOfA, allOfB), c.orFilter(allOfC, allOfD))
	check("NOT ALL_OF [A or B, C or D]", setMinus(all, both), notGroups)

	inOr := c.text(allOfA)
	c.op(inOr).Or = c.allOf(c.orFilter(allOfB, allOfC), c.text(allOfD))
	check("A OR ALL_OF [B or C, D]", setOr(a, setAnd(setOr(b, cc), d)), inOr)

	nested := c.allOf(c.allOf(c.orFilter(allOfA, allOfB)), c.allOf(c.orFilter(allOfC, allOfD)))
	check("nested ALL_OF", both, nested)

	// an empty item matches everything, like an empty filter
	check("ALL_OF [empty, A]", a, c.allOf(new(T), c.text(allOfA)))
}

func TestFilterAllOf(t *testing.T) {
	withTxn(func(ctx context.Context) error {
		t.Run("scenes", func(t *testing.T) {
			allOfCase[models.SceneFilterType]{
				text: func(r string) *models.SceneFilterType {
					return &models.SceneFilterType{Title: regexCriterion(r)}
				},
				op: func(f *models.SceneFilterType) *models.OperatorFilter[models.SceneFilterType] {
					return &f.OperatorFilter
				},
				query: querySceneSet,
			}.run(ctx, t)
		})

		t.Run("images", func(t *testing.T) {
			allOfCase[models.ImageFilterType]{
				text: func(r string) *models.ImageFilterType {
					return &models.ImageFilterType{Title: regexCriterion(r)}
				},
				op: func(f *models.ImageFilterType) *models.OperatorFilter[models.ImageFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.ImageFilterType) idSet {
					ret := idSet{}
					for _, i := range queryImages(ctx, t, db.Image(), f, allItems()) {
						ret[i.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("galleries", func(t *testing.T) {
			allOfCase[models.GalleryFilterType]{
				text: func(r string) *models.GalleryFilterType {
					return &models.GalleryFilterType{Title: regexCriterion(r)}
				},
				op: func(f *models.GalleryFilterType) *models.OperatorFilter[models.GalleryFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.GalleryFilterType) idSet {
					ret := idSet{}
					for _, g := range queryGallery(ctx, t, db.Gallery(), f, allItems()) {
						ret[g.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("performers", func(t *testing.T) {
			allOfCase[models.PerformerFilterType]{
				text: func(r string) *models.PerformerFilterType {
					return &models.PerformerFilterType{Name: regexCriterion(r)}
				},
				op: func(f *models.PerformerFilterType) *models.OperatorFilter[models.PerformerFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.PerformerFilterType) idSet {
					ret := idSet{}
					for _, p := range queryPerformers(ctx, t, f, allItems()) {
						ret[p.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("studios", func(t *testing.T) {
			allOfCase[models.StudioFilterType]{
				text: func(r string) *models.StudioFilterType {
					return &models.StudioFilterType{Name: regexCriterion(r)}
				},
				op: func(f *models.StudioFilterType) *models.OperatorFilter[models.StudioFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.StudioFilterType) idSet {
					ret := idSet{}
					for _, s := range queryStudios(ctx, t, f, allItems()) {
						ret[s.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("tags", func(t *testing.T) {
			allOfCase[models.TagFilterType]{
				text: func(r string) *models.TagFilterType {
					return &models.TagFilterType{Name: regexCriterion(r)}
				},
				op: func(f *models.TagFilterType) *models.OperatorFilter[models.TagFilterType] { return &f.OperatorFilter },
				query: func(ctx context.Context, t *testing.T, f *models.TagFilterType) idSet {
					ret := idSet{}
					for _, tag := range queryTags(ctx, t, db.Tag(), f, allItems()) {
						ret[tag.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("groups", func(t *testing.T) {
			allOfCase[models.GroupFilterType]{
				text: func(r string) *models.GroupFilterType {
					return &models.GroupFilterType{Name: regexCriterion(r)}
				},
				op: func(f *models.GroupFilterType) *models.OperatorFilter[models.GroupFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.GroupFilterType) idSet {
					ret := idSet{}
					for _, g := range queryGroups(ctx, t, f, allItems()) {
						ret[g.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("audios", func(t *testing.T) {
			allOfCase[models.AudioFilterType]{
				text: func(r string) *models.AudioFilterType {
					return &models.AudioFilterType{Title: regexCriterion(r)}
				},
				op: func(f *models.AudioFilterType) *models.OperatorFilter[models.AudioFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.AudioFilterType) idSet {
					ret := idSet{}
					for _, a := range queryAudio(ctx, t, db.Audio(), f, allItems()) {
						ret[a.ID] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("files", func(t *testing.T) {
			allOfCase[models.FileFilterType]{
				text: func(r string) *models.FileFilterType {
					return &models.FileFilterType{Basename: regexCriterion(r)}
				},
				op: func(f *models.FileFilterType) *models.OperatorFilter[models.FileFilterType] { return &f.OperatorFilter },
				query: func(ctx context.Context, t *testing.T, f *models.FileFilterType) idSet {
					t.Helper()
					r, err := db.File().Query(ctx, models.FileQueryOptions{
						QueryOptions: models.QueryOptions{FindFilter: allItems()},
						FileFilter:   f,
					})
					require.NoError(t, err)
					ret := idSet{}
					for _, id := range r.IDs {
						ret[int(id)] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		t.Run("folders", func(t *testing.T) {
			allOfCase[models.FolderFilterType]{
				text: func(r string) *models.FolderFilterType {
					return &models.FolderFilterType{Basename: regexCriterion(r)}
				},
				op: func(f *models.FolderFilterType) *models.OperatorFilter[models.FolderFilterType] {
					return &f.OperatorFilter
				},
				query: func(ctx context.Context, t *testing.T, f *models.FolderFilterType) idSet {
					t.Helper()
					r, err := db.Folder().Query(ctx, models.FolderQueryOptions{
						QueryOptions: models.QueryOptions{FindFilter: allItems()},
						FolderFilter: f,
					})
					require.NoError(t, err)
					ret := idSet{}
					for _, id := range r.IDs {
						ret[int(id)] = true
					}
					return ret
				},
			}.run(ctx, t)
		})

		return nil
	})

	// the test data has no stories, so make some
	runWithRollbackTxn(t, "stories", func(t *testing.T, ctx context.Context) {
		for i := 0; i < 10; i++ {
			s := models.NewStory()
			s.Title = fmt.Sprintf("story_%04d_title", i)
			require.NoError(t, db.Story().Create(ctx, &s, nil))
		}

		allOfCase[models.StoryFilterType]{
			text: func(r string) *models.StoryFilterType {
				return &models.StoryFilterType{Title: regexCriterion(r)}
			},
			op: func(f *models.StoryFilterType) *models.OperatorFilter[models.StoryFilterType] {
				return &f.OperatorFilter
			},
			query: func(ctx context.Context, t *testing.T, f *models.StoryFilterType) idSet {
				t.Helper()
				r, err := db.Story().Query(ctx, models.StoryQueryOptions{
					QueryOptions: models.QueryOptions{FindFilter: allItems()},
					StoryFilter:  f,
				})
				require.NoError(t, err)
				ret := idSet{}
				for _, id := range r.IDs {
					ret[id] = true
				}
				return ret
			},
		}.run(ctx, t)
	})
}

// Conditions the database checks after grouping (HAVING) must keep working
// inside ALL_OF items, also when those sit under OR or NOT (#2167).
func TestFilterAllOfGroupedConditions(t *testing.T) {
	withTxn(func(ctx context.Context) error {
		title := func(r string) *models.SceneFilterType {
			return &models.SceneFilterType{Title: regexCriterion(r)}
		}
		hasMarkers := func() *models.SceneFilterType {
			v := "true"
			return &models.SceneFilterType{HasMarkers: &v}
		}
		tagsAll := func() *models.SceneFilterType {
			return &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{
				Value:    []string{strconv.Itoa(tagIDs[tagIdx1WithScene]), strconv.Itoa(tagIDs[tagIdx2WithScene])},
				Modifier: models.CriterionModifierIncludesAll,
			}}
		}
		or := func(a, b *models.SceneFilterType) *models.SceneFilterType {
			a.Or = b
			return a
		}
		allOf := func(filters ...*models.SceneFilterType) *models.SceneFilterType {
			return &models.SceneFilterType{OperatorFilter: models.OperatorFilter[models.SceneFilterType]{AllOf: filters}}
		}

		all := querySceneSet(ctx, t, &models.SceneFilterType{})
		w := querySceneSet(ctx, t, title(firstIndexesRegex))
		m := querySceneSet(ctx, t, hasMarkers())
		tg := querySceneSet(ctx, t, tagsAll())
		require.NotEmpty(t, m)
		require.NotEmpty(t, tg)

		check := func(name string, want idSet, f *models.SceneFilterType) {
			t.Helper()
			assert.Equal(t, want.sorted(), querySceneSet(ctx, t, f).sorted(), name)
		}

		check("ALL_OF [title or markers, tags]", setAnd(setOr(w, m), tg),
			allOf(or(title(firstIndexesRegex), hasMarkers()), tagsAll()))
		check("ALL_OF [markers or title, title or tags]", setAnd(setOr(m, w), setOr(w, tg)),
			allOf(or(hasMarkers(), title(firstIndexesRegex)), or(title(firstIndexesRegex), tagsAll())))
		check("title OR ALL_OF [markers, tags]", setOr(w, setAnd(m, tg)),
			or(title(firstIndexesRegex), allOf(hasMarkers(), tagsAll())))

		not := &models.SceneFilterType{}
		not.Not = allOf(hasMarkers())
		check("NOT ALL_OF [markers]", setMinus(all, m), not)

		return nil
	})
}

// Related filters such as performers_filter and files_filter take ALL_OF too.
// files_filter is joined straight into the scene query unless it has ALL_OF.
func TestFilterAllOfRelated(t *testing.T) {
	withTxn(func(ctx context.Context) error {
		perf := func(r string) *models.PerformerFilterType {
			return &models.PerformerFilterType{Name: regexCriterion(r)}
		}
		perfOr := func(a, b string) *models.PerformerFilterType {
			f := perf(a)
			f.Or = perf(b)
			return f
		}
		groups := &models.PerformerFilterType{OperatorFilter: models.OperatorFilter[models.PerformerFilterType]{
			AllOf: []*models.PerformerFilterType{perfOr(allOfA, allOfB), perfOr(allOfC, allOfD)},
		}}

		want := querySceneSet(ctx, t, &models.SceneFilterType{PerformersFilter: perf(allOfAorBandCorD)})
		got := querySceneSet(ctx, t, &models.SceneFilterType{PerformersFilter: groups})
		require.NotEmpty(t, want)
		assert.Equal(t, want.sorted(), got.sorted(), "performers_filter")

		file := func(r string) *models.FileFilterType {
			return &models.FileFilterType{Basename: regexCriterion(r)}
		}
		fileOr := func(a, b string) *models.FileFilterType {
			f := file(a)
			f.Or = file(b)
			return f
		}
		fileGroups := &models.FileFilterType{OperatorFilter: models.OperatorFilter[models.FileFilterType]{
			AllOf: []*models.FileFilterType{fileOr(allOfA, allOfB), fileOr(allOfC, allOfD)},
		}}

		want = querySceneSet(ctx, t, &models.SceneFilterType{FilesFilter: file(allOfAorBandCorD)})
		got = querySceneSet(ctx, t, &models.SceneFilterType{FilesFilter: fileGroups})
		require.NotEmpty(t, want)
		assert.Equal(t, want.sorted(), got.sorted(), "files_filter")

		return nil
	})
}

// An invalid ALL_OF item is reported, not skipped.
func TestFilterAllOfInvalidItem(t *testing.T) {
	withTxn(func(ctx context.Context) error {
		bad := &models.SceneFilterType{}
		bad.And = &models.SceneFilterType{}
		bad.Or = &models.SceneFilterType{}
		f := &models.SceneFilterType{OperatorFilter: models.OperatorFilter[models.SceneFilterType]{
			AllOf: []*models.SceneFilterType{bad},
		}}
		_, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: allItems()},
			SceneFilter:  f,
		})
		assert.Error(t, err)
		return nil
	})
}
