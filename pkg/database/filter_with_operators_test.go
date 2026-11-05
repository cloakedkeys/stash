//go:build integration
// +build integration

package database_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// #2170: some conditions build their SQL with a WITH clause. Only the WITH
// clauses of the top level of a filter went into the query, so these
// conditions failed inside AND, OR and NOT. Each one is checked here inside
// AND, OR and NOT (also nested, and twice with different values) against the
// same set operation on the results of the conditions on their own.

type withOperatorCase[T any] struct {
	// returns a new filter for a plain condition
	plain func() *T
	// return new filters for the condition that uses a WITH clause, with two
	// different values
	with  func() *T
	with2 func() *T
	query func(ctx context.Context, f *T) ([]int, error)
	op    func(f *T) *models.OperatorFilter[T]
	// optional; adds data the fixtures don't have. Rolled back afterwards.
	setup func(ctx context.Context) error
	// if set, the checks where the WITH condition is directly under OR are
	// skipped, for this reason
	skipOr string
}

func (c withOperatorCase[T]) run(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		if c.setup != nil {
			if err := c.setup(ctx); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}

		query := func(name string, f *T) (idSet, bool) {
			ids, err := c.query(ctx, f)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				return nil, false
			}
			ret := idSet{}
			for _, id := range ids {
				ret[id] = true
			}
			return ret, true
		}

		var empty T
		all, ok1 := query("all", &empty)
		p, ok2 := query("plain", c.plain())
		w, ok3 := query("with", c.with())
		w2, ok4 := query("with2", c.with2())
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return nil
		}

		// the conditions must split the items for the checks to mean anything
		assert.NotEmpty(t, w.sorted(), "with condition finds nothing")
		assert.NotEqual(t, all.sorted(), w.sorted(), "with condition finds everything")
		assert.NotEmpty(t, w2.sorted(), "second with condition finds nothing")
		assert.NotEqual(t, w.sorted(), w2.sorted(), "with conditions find the same")
		assert.NotEmpty(t, p.sorted(), "plain condition finds nothing")
		assert.NotEqual(t, all.sorted(), p.sorted(), "plain condition finds everything")

		check := func(name string, want idSet, f *T) {
			if got, ok := query(name, f); ok {
				assert.Equal(t, want.sorted(), got.sorted(), name)
			}
		}
		checkOr := func(name string, want idSet, f *T) {
			if c.skipOr != "" {
				t.Logf("skipping %s: %s", name, c.skipOr)
				return
			}
			check(name, want, f)
		}
		and := func(a, b *T) *T { c.op(a).And = b; return a }
		or := func(a, b *T) *T { c.op(a).Or = b; return a }
		not := func(a, b *T) *T { c.op(a).Not = b; return a }

		check("plain AND with", setAnd(p, w), and(c.plain(), c.with()))
		check("with AND plain", setAnd(w, p), and(c.with(), c.plain()))
		checkOr("plain OR with", setOr(p, w), or(c.plain(), c.with()))
		checkOr("with OR plain", setOr(w, p), or(c.with(), c.plain()))
		check("plain NOT with", setMinus(p, w), not(c.plain(), c.with()))
		check("with NOT plain", setMinus(w, p), not(c.with(), c.plain()))
		check("NOT with", setMinus(all, w), not(new(T), c.with()))

		// deeper down
		check("plain OR (plain AND with)", setOr(p, setAnd(p, w)), or(c.plain(), and(c.plain(), c.with())))
		check("plain AND (NOT with)", setMinus(p, w), and(c.plain(), not(new(T), c.with())))
		checkOr("NOT (plain OR with)", setMinus(all, setOr(p, w)), not(new(T), or(c.plain(), c.with())))

		// the same condition at two levels, with different values
		check("with AND with2", setAnd(w, w2), and(c.with(), c.with2()))
		checkOr("with OR with2", setOr(w, w2), or(c.with(), c.with2()))
		check("with NOT with2", setMinus(w, w2), not(c.with(), c.with2()))

		return nil
	})
}

func idStrings(ids []int, idxs ...int) []string {
	var ret []string
	for _, idx := range idxs {
		ret = append(ret, strconv.Itoa(ids[idx]))
	}
	return ret
}

// names and titles with an even index
const evenIndexRegex = `_[0-9]{3}[02468]_`

func TestWithConditionsUnderOperators(t *testing.T) {
	findAll := allItems

	sceneQuery := func(ctx context.Context, f *models.SceneFilterType) ([]int, error) {
		r, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: findAll()},
			SceneFilter:  f,
		})
		if err != nil {
			return nil, err
		}
		return r.IDs, nil
	}
	sceneOp := func(f *models.SceneFilterType) *models.OperatorFilter[models.SceneFilterType] {
		return &f.OperatorFilter
	}
	scenePlain := func() *models.SceneFilterType {
		return &models.SceneFilterType{Title: regexCriterion(evenIndexRegex)}
	}

	t.Run("scenes performer tags", func(t *testing.T) {
		withOperatorCase[models.SceneFilterType]{
			plain: scenePlain,
			with: func() *models.SceneFilterType {
				return &models.SceneFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			with2: func() *models.SceneFilterType {
				return &models.SceneFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdx2WithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			query: sceneQuery,
			op:    sceneOp,
		}.run(t)
	})

	t.Run("images performer tags", func(t *testing.T) {
		withOperatorCase[models.ImageFilterType]{
			plain: func() *models.ImageFilterType {
				return &models.ImageFilterType{Title: regexCriterion(evenIndexRegex)}
			},
			with: func() *models.ImageFilterType {
				return &models.ImageFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			with2: func() *models.ImageFilterType {
				return &models.ImageFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdx2WithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			query: func(ctx context.Context, f *models.ImageFilterType) ([]int, error) {
				r, err := db.Image().Query(ctx, models.ImageQueryOptions{
					QueryOptions: models.QueryOptions{FindFilter: findAll()},
					ImageFilter:  f,
				})
				if err != nil {
					return nil, err
				}
				return r.IDs, nil
			},
			op: func(f *models.ImageFilterType) *models.OperatorFilter[models.ImageFilterType] {
				return &f.OperatorFilter
			},
		}.run(t)
	})

	t.Run("galleries performer tags", func(t *testing.T) {
		withOperatorCase[models.GalleryFilterType]{
			plain: func() *models.GalleryFilterType {
				return &models.GalleryFilterType{Title: regexCriterion(evenIndexRegex)}
			},
			with: func() *models.GalleryFilterType {
				return &models.GalleryFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			with2: func() *models.GalleryFilterType {
				return &models.GalleryFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdx2WithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			query: func(ctx context.Context, f *models.GalleryFilterType) ([]int, error) {
				galleries, _, err := db.Gallery().Query(ctx, f, findAll())
				var ids []int
				for _, g := range galleries {
					ids = append(ids, g.ID)
				}
				return ids, err
			},
			op: func(f *models.GalleryFilterType) *models.OperatorFilter[models.GalleryFilterType] {
				return &f.OperatorFilter
			},
		}.run(t)
	})

	t.Run("audios performer tags", func(t *testing.T) {
		withOperatorCase[models.AudioFilterType]{
			plain: func() *models.AudioFilterType {
				return &models.AudioFilterType{Title: regexCriterion(evenIndexRegex)}
			},
			with: func() *models.AudioFilterType {
				return &models.AudioFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			with2: func() *models.AudioFilterType {
				return &models.AudioFilterType{PerformerTags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdx2WithPerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			query: func(ctx context.Context, f *models.AudioFilterType) ([]int, error) {
				r, err := db.Audio().Query(ctx, models.AudioQueryOptions{
					QueryOptions: models.QueryOptions{FindFilter: findAll()},
					AudioFilter:  f,
				})
				if err != nil {
					return nil, err
				}
				return r.IDs, nil
			},
			op: func(f *models.AudioFilterType) *models.OperatorFilter[models.AudioFilterType] {
				return &f.OperatorFilter
			},
		}.run(t)
	})

	tagQuery := func(ctx context.Context, f *models.TagFilterType) ([]int, error) {
		tags, _, err := db.Tag().Query(ctx, f, findAll())
		var ids []int
		for _, tag := range tags {
			ids = append(ids, tag.ID)
		}
		return ids, err
	}
	tagOp := func(f *models.TagFilterType) *models.OperatorFilter[models.TagFilterType] { return &f.OperatorFilter }
	tagPlain := func() *models.TagFilterType {
		// tag names are not zero-padded
		return &models.TagFilterType{Name: regexCriterion(`^tag_[0-9]*[02468]_`)}
	}

	t.Run("tags markers filter", func(t *testing.T) {
		withOperatorCase[models.TagFilterType]{
			plain: tagPlain,
			with: func() *models.TagFilterType {
				return &models.TagFilterType{MarkersFilter: &models.SceneMarkerFilterType{Tags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithMarkers),
					Modifier: models.CriterionModifierIncludes,
				}}}
			},
			with2: func() *models.TagFilterType {
				return &models.TagFilterType{MarkersFilter: &models.SceneMarkerFilterType{Tags: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithGrandParent),
					Modifier: models.CriterionModifierIncludes,
				}}}
			},
			query: tagQuery,
			op:    tagOp,
		}.run(t)
	})

	t.Run("tags parents", func(t *testing.T) {
		withOperatorCase[models.TagFilterType]{
			plain: tagPlain,
			with: func() *models.TagFilterType {
				return &models.TagFilterType{Parents: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithGrandChild),
					Modifier: models.CriterionModifierIncludes,
					Depth:    intPtr(-1),
				}}
			},
			with2: func() *models.TagFilterType {
				return &models.TagFilterType{Parents: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(tagIDs, tagIdxWithChildTag),
					Modifier: models.CriterionModifierIncludes,
					Depth:    intPtr(-1),
				}}
			},
			query: tagQuery,
			op:    tagOp,
		}.run(t)
	})

	t.Run("tags scene count with depth", func(t *testing.T) {
		withOperatorCase[models.TagFilterType]{
			plain: tagPlain,
			with: func() *models.TagFilterType {
				return &models.TagFilterType{SceneCount: &models.HierarchicalCountInput{
					Value:    0,
					Modifier: models.CriterionModifierGreaterThan,
					Depth:    intPtr(-1),
				}}
			},
			with2: func() *models.TagFilterType {
				return &models.TagFilterType{SceneCount: &models.HierarchicalCountInput{
					Value:    1,
					Modifier: models.CriterionModifierGreaterThan,
					Depth:    intPtr(0),
				}}
			},
			query: tagQuery,
			op:    tagOp,
		}.run(t)
	})

	// no fixture scene has both groups and performers
	addSceneGroups := func(ctx context.Context) error {
		for sceneIdx, groupIdx := range map[int]int{
			sceneIdxWithTwoPerformers: groupIdxWithScene,
			sceneIdxWithPerformerTag:  groupIdxWithParentAndScene,
		} {
			if _, err := db.Scene().UpdatePartial(ctx, sceneIDs[sceneIdx], models.ScenePartial{
				GroupIDs: &models.UpdateGroupIDs{
					Groups: []models.GroupsScenes{{GroupID: groupIDs[groupIdx]}},
					Mode:   models.RelationshipUpdateModeAdd,
				},
			}); err != nil {
				return err
			}
		}
		return nil
	}

	groupQuery := func(ctx context.Context, f *models.GroupFilterType) ([]int, error) {
		groups, _, err := db.Group().Query(ctx, f, findAll())
		var ids []int
		for _, g := range groups {
			ids = append(ids, g.ID)
		}
		return ids, err
	}
	groupOp := func(f *models.GroupFilterType) *models.OperatorFilter[models.GroupFilterType] {
		return &f.OperatorFilter
	}
	groupPlain := func() *models.GroupFilterType {
		return &models.GroupFilterType{Name: regexCriterion(evenIndexRegex)}
	}

	t.Run("groups performers", func(t *testing.T) {
		withOperatorCase[models.GroupFilterType]{
			plain: groupPlain,
			with: func() *models.GroupFilterType {
				return &models.GroupFilterType{Performers: &models.MultiCriterionInput{
					Value:    idStrings(performerIDs, performerIdx1WithScene),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			with2: func() *models.GroupFilterType {
				return &models.GroupFilterType{Performers: &models.MultiCriterionInput{
					Value:    idStrings(performerIDs, performerIdxWithTag),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			query: groupQuery,
			op:    groupOp,
			setup: addSceneGroups,
		}.run(t)
	})

	t.Run("groups containing groups", func(t *testing.T) {
		withOperatorCase[models.GroupFilterType]{
			plain: groupPlain,
			with: func() *models.GroupFilterType {
				return &models.GroupFilterType{ContainingGroups: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(groupIDs, groupIdxWithGrandChild),
					Modifier: models.CriterionModifierIncludes,
					Depth:    intPtr(-1),
				}}
			},
			with2: func() *models.GroupFilterType {
				return &models.GroupFilterType{ContainingGroups: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(groupIDs, groupIdxWithChild),
					Modifier: models.CriterionModifierIncludes,
					Depth:    intPtr(-1),
				}}
			},
			query: groupQuery,
			op:    groupOp,
		}.run(t)
	})

	performerQuery := func(ctx context.Context, f *models.PerformerFilterType) ([]int, error) {
		performers, _, err := db.Performer().Query(ctx, f, findAll())
		var ids []int
		for _, p := range performers {
			ids = append(ids, p.ID)
		}
		return ids, err
	}
	performerOp := func(f *models.PerformerFilterType) *models.OperatorFilter[models.PerformerFilterType] {
		return &f.OperatorFilter
	}
	performerPlain := func() *models.PerformerFilterType {
		return &models.PerformerFilterType{Name: regexCriterion(evenIndexRegex)}
	}

	t.Run("performers studios", func(t *testing.T) {
		withOperatorCase[models.PerformerFilterType]{
			plain: performerPlain,
			with: func() *models.PerformerFilterType {
				return &models.PerformerFilterType{Studios: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(studioIDs, studioIdxWithScenePerformer, studioIdxWithImagePerformer),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			// includes and excludes
			with2: func() *models.PerformerFilterType {
				return &models.PerformerFilterType{Studios: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(studioIDs, studioIdxWithGalleryPerformer, studioIdx1WithTwoScenePerformer),
					Modifier: models.CriterionModifierIncludes,
					Excludes: idStrings(studioIDs, studioIdx2WithTwoScenePerformer),
				}}
			},
			query: performerQuery,
			op:    performerOp,
		}.run(t)
	})

	t.Run("performers groups", func(t *testing.T) {
		withOperatorCase[models.PerformerFilterType]{
			plain: performerPlain,
			with: func() *models.PerformerFilterType {
				return &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(groupIDs, groupIdxWithScene),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			// with sub-groups, which uses a recursive WITH clause
			with2: func() *models.PerformerFilterType {
				return &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{
					Value:    idStrings(groupIDs, groupIdxWithChildWithScene),
					Modifier: models.CriterionModifierIncludes,
					Depth:    intPtr(-1),
				}}
			},
			query: performerQuery,
			op:    performerOp,
			setup: addSceneGroups,
		}.run(t)
	})

	t.Run("performers appears with", func(t *testing.T) {
		withOperatorCase[models.PerformerFilterType]{
			plain: performerPlain,
			with: func() *models.PerformerFilterType {
				return &models.PerformerFilterType{Performers: &models.MultiCriterionInput{
					Value:    idStrings(performerIDs, performerIdx1WithScene),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			with2: func() *models.PerformerFilterType {
				return &models.PerformerFilterType{Performers: &models.MultiCriterionInput{
					Value:    idStrings(performerIDs, performerIdxWithTag),
					Modifier: models.CriterionModifierIncludes,
				}}
			},
			query:  performerQuery,
			op:     performerOp,
			skipOr: "appears with only uses an inner join, which OR makes a left join, so it matches everything (a separate, older bug)",
		}.run(t)
	})
}
