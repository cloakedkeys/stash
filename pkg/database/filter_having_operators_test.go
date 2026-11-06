//go:build integration
// +build integration

package database_test

import (
	"context"
	"sort"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// #2167: conditions that the database checks after grouping (SQL HAVING:
// "includes all", counts, has markers, has chapters, average resolution) were
// always ANDed with the rest of the filter, even under OR or NOT. Each kind is
// checked here under AND, OR and NOT against the same set operation on the
// results of the two conditions on their own.

type idSet map[int]bool

func (s idSet) sorted() []int {
	ret := []int{}
	for id := range s {
		ret = append(ret, id)
	}
	sort.Ints(ret)
	return ret
}

func setAnd(a, b idSet) idSet {
	ret := idSet{}
	for id := range a {
		if b[id] {
			ret[id] = true
		}
	}
	return ret
}

func setOr(a, b idSet) idSet {
	ret := idSet{}
	for id := range a {
		ret[id] = true
	}
	for id := range b {
		ret[id] = true
	}
	return ret
}

func setMinus(a, b idSet) idSet {
	ret := idSet{}
	for id := range a {
		if !b[id] {
			ret[id] = true
		}
	}
	return ret
}

type operatorCase[T any] struct {
	// returns a new filter for the condition the database checks by row
	where func() *T
	// returns a new filter for the condition it checks after grouping
	having func() *T
	query  func(ctx context.Context, t *testing.T, f *T) idSet
	op     func(f *T) *models.OperatorFilter[T]
}

func (c operatorCase[T]) run(t *testing.T) {
	runTxn(t, func(ctx context.Context) error {
		var empty T
		all := c.query(ctx, t, &empty)
		w := c.query(ctx, t, c.where())
		h := c.query(ctx, t, c.having())

		// the conditions must split the items for the checks to mean anything
		assert.NotEmpty(t, h.sorted(), "having condition finds nothing")
		assert.NotEqual(t, all.sorted(), h.sorted(), "having condition finds everything")
		assert.NotEmpty(t, w.sorted(), "where condition finds nothing")
		assert.NotEqual(t, w.sorted(), all.sorted(), "where condition finds everything")

		combine := func(first func() *T, set func(o *models.OperatorFilter[T], sub *T), second func() *T) idSet {
			f := first()
			set(c.op(f), second())
			return c.query(ctx, t, f)
		}
		and := func(o *models.OperatorFilter[T], sub *T) { o.And = sub }
		or := func(o *models.OperatorFilter[T], sub *T) { o.Or = sub }
		not := func(o *models.OperatorFilter[T], sub *T) { o.Not = sub }

		check := func(name string, want, got idSet) {
			assert.Equal(t, want.sorted(), got.sorted(), name)
		}

		check("where AND having", setAnd(w, h), combine(c.where, and, c.having))
		check("having AND where", setAnd(h, w), combine(c.having, and, c.where))
		check("where OR having", setOr(w, h), combine(c.where, or, c.having))
		check("having OR where", setOr(h, w), combine(c.having, or, c.where))
		check("where NOT having", setMinus(w, h), combine(c.where, not, c.having))
		check("having NOT where", setMinus(h, w), combine(c.having, not, c.where))

		notOnly := new(T)
		c.op(notOnly).Not = c.having()
		check("NOT having", setMinus(all, h), c.query(ctx, t, notOnly))

		// a HAVING condition deeper down: where OR (where AND having)
		deep := c.where()
		inner := c.where()
		c.op(inner).And = c.having()
		c.op(deep).Or = inner
		check("where OR (where AND having)", setOr(w, setAnd(w, h)), c.query(ctx, t, deep))

		return nil
	})
}

var allItems = func() *models.FindFilterType {
	perPage := -1
	return &models.FindFilterType{PerPage: &perPage}
}

func regexCriterion(v string) *models.StringCriterionInput {
	return &models.StringCriterionInput{Value: v, Modifier: models.CriterionModifierMatchesRegex}
}

// items with index 0 to 4 or 10 to 14
const firstIndexesRegex = "_00[01][0-4]_"

func querySceneSet(ctx context.Context, t *testing.T, f *models.SceneFilterType) idSet {
	t.Helper()
	ret := idSet{}
	for _, s := range queryScene(ctx, t, db.Scene(), f, allItems()) {
		ret[s.ID] = true
	}
	return ret
}

func TestHavingConditionsUnderOperators(t *testing.T) {
	sceneOp := func(f *models.SceneFilterType) *models.OperatorFilter[models.SceneFilterType] {
		return &f.OperatorFilter
	}
	sceneWhere := func() *models.SceneFilterType {
		return &models.SceneFilterType{Title: regexCriterion(firstIndexesRegex)}
	}

	t.Run("scenes has markers", func(t *testing.T) {
		operatorCase[models.SceneFilterType]{
			where: sceneWhere,
			having: func() *models.SceneFilterType {
				v := "true"
				return &models.SceneFilterType{HasMarkers: &v}
			},
			query: querySceneSet,
			op:    sceneOp,
		}.run(t)
	})

	t.Run("scenes tags includes all", func(t *testing.T) {
		operatorCase[models.SceneFilterType]{
			where: sceneWhere,
			having: func() *models.SceneFilterType {
				return &models.SceneFilterType{Tags: &models.HierarchicalMultiCriterionInput{
					Value:    []string{strconv.Itoa(tagIDs[tagIdx1WithScene]), strconv.Itoa(tagIDs[tagIdx2WithScene])},
					Modifier: models.CriterionModifierIncludesAll,
				}}
			},
			query: querySceneSet,
			op:    sceneOp,
		}.run(t)
	})

	t.Run("scenes performers includes all", func(t *testing.T) {
		operatorCase[models.SceneFilterType]{
			where: sceneWhere,
			having: func() *models.SceneFilterType {
				return &models.SceneFilterType{Performers: &models.MultiCriterionInput{
					Value:    []string{strconv.Itoa(performerIDs[performerIdx1WithScene]), strconv.Itoa(performerIDs[performerIdx2WithScene])},
					Modifier: models.CriterionModifierIncludesAll,
				}}
			},
			query: querySceneSet,
			op:    sceneOp,
		}.run(t)
	})

	studioQuery := func(ctx context.Context, t *testing.T, f *models.StudioFilterType) idSet {
		t.Helper()
		studios, _, err := db.Studio().Query(ctx, f, allItems())
		if err != nil {
			t.Fatalf("querying studios: %v", err)
		}
		ret := idSet{}
		for _, s := range studios {
			ret[s.ID] = true
		}
		return ret
	}
	studioOp := func(f *models.StudioFilterType) *models.OperatorFilter[models.StudioFilterType] {
		return &f.OperatorFilter
	}
	studioWhere := func() *models.StudioFilterType {
		return &models.StudioFilterType{Name: regexCriterion(firstIndexesRegex)}
	}

	t.Run("studios scene count", func(t *testing.T) {
		operatorCase[models.StudioFilterType]{
			where: studioWhere,
			having: func() *models.StudioFilterType {
				return &models.StudioFilterType{SceneCount: &models.IntCriterionInput{Value: 0, Modifier: models.CriterionModifierGreaterThan}}
			},
			query: studioQuery,
			op:    studioOp,
		}.run(t)
	})

	t.Run("studios child count", func(t *testing.T) {
		operatorCase[models.StudioFilterType]{
			where: studioWhere,
			having: func() *models.StudioFilterType {
				return &models.StudioFilterType{ChildCount: &models.IntCriterionInput{Value: 0, Modifier: models.CriterionModifierGreaterThan}}
			},
			query: studioQuery,
			op:    studioOp,
		}.run(t)
	})

	t.Run("tags scene count", func(t *testing.T) {
		operatorCase[models.TagFilterType]{
			where: func() *models.TagFilterType {
				// tag names look like tag_0_Name
				return &models.TagFilterType{Name: regexCriterion("^tag_[0-4]_")}
			},
			having: func() *models.TagFilterType {
				return &models.TagFilterType{SceneCount: &models.HierarchicalCountInput{Value: 0, Modifier: models.CriterionModifierGreaterThan}}
			},
			query: func(ctx context.Context, t *testing.T, f *models.TagFilterType) idSet {
				t.Helper()
				tags, _, err := db.Tag().Query(ctx, f, allItems())
				if err != nil {
					t.Fatalf("querying tags: %v", err)
				}
				ret := idSet{}
				for _, s := range tags {
					ret[s.ID] = true
				}
				return ret
			},
			op: func(f *models.TagFilterType) *models.OperatorFilter[models.TagFilterType] { return &f.OperatorFilter },
		}.run(t)
	})

	galleryQuery := func(ctx context.Context, t *testing.T, f *models.GalleryFilterType) idSet {
		t.Helper()
		galleries, _, err := db.Gallery().Query(ctx, f, allItems())
		if err != nil {
			t.Fatalf("querying galleries: %v", err)
		}
		ret := idSet{}
		for _, s := range galleries {
			ret[s.ID] = true
		}
		return ret
	}
	galleryOp := func(f *models.GalleryFilterType) *models.OperatorFilter[models.GalleryFilterType] {
		return &f.OperatorFilter
	}
	galleryWhere := func() *models.GalleryFilterType {
		return &models.GalleryFilterType{Title: regexCriterion(firstIndexesRegex)}
	}

	t.Run("galleries has chapters", func(t *testing.T) {
		operatorCase[models.GalleryFilterType]{
			where: galleryWhere,
			having: func() *models.GalleryFilterType {
				v := "true"
				return &models.GalleryFilterType{HasChapters: &v}
			},
			query: galleryQuery,
			op:    galleryOp,
		}.run(t)
	})

	t.Run("galleries average resolution", func(t *testing.T) {
		operatorCase[models.GalleryFilterType]{
			where: galleryWhere,
			having: func() *models.GalleryFilterType {
				return &models.GalleryFilterType{AverageResolution: &models.ResolutionCriterionInput{
					Value:    models.ResolutionEnumVeryLow,
					Modifier: models.CriterionModifierGreaterThan,
				}}
			},
			query: galleryQuery,
			op:    galleryOp,
		}.run(t)
	})

	t.Run("folders parent folder includes all", func(t *testing.T) {
		operatorCase[models.FolderFilterType]{
			where: func() *models.FolderFilterType {
				return &models.FolderFilterType{Path: regexCriterion("folder_00[01][0-4]_Path$")}
			},
			having: func() *models.FolderFilterType {
				return &models.FolderFilterType{ParentFolder: &models.HierarchicalMultiCriterionInput{
					Value:    []string{folderIDs[folderIdxWithSubFolder].String()},
					Modifier: models.CriterionModifierIncludesAll,
				}}
			},
			query: func(ctx context.Context, t *testing.T, f *models.FolderFilterType) idSet {
				t.Helper()
				r, err := db.Folder().Query(ctx, models.FolderQueryOptions{
					QueryOptions: models.QueryOptions{FindFilter: allItems()},
					FolderFilter: f,
				})
				if err != nil {
					t.Fatalf("querying folders: %v", err)
				}
				ret := idSet{}
				for _, id := range r.IDs {
					ret[int(id)] = true
				}
				return ret
			},
			op: func(f *models.FolderFilterType) *models.OperatorFilter[models.FolderFilterType] {
				return &f.OperatorFilter
			},
		}.run(t)
	})
}
