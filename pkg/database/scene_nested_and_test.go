//go:build integration
// +build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

// The list screens send a repeated condition (two "is missing", two title
// conditions, ...) as a chain of AND sub-filters, one level per repeat. These
// check that the chain finds exactly the scenes every condition finds.

func allScenesFindFilter() *models.FindFilterType {
	perPage := -1
	return &models.FindFilterType{PerPage: &perPage}
}

func querySceneIDSet(ctx context.Context, t *testing.T, sqb models.SceneReader, f *models.SceneFilterType) map[int]bool {
	t.Helper()
	scenes := queryScene(ctx, t, sqb, f, allScenesFindFilter())
	ret := make(map[int]bool, len(scenes))
	for _, s := range scenes {
		ret[s.ID] = true
	}
	return ret
}

func intersectIDs(sets ...map[int]bool) []int {
	ret := []int{}
	for id := range sets[0] {
		inAll := true
		for _, s := range sets[1:] {
			if !s[id] {
				inAll = false
				break
			}
		}
		if inAll {
			ret = append(ret, id)
		}
	}
	return ret
}

func TestSceneQueryNestedAndSameCriterion(t *testing.T) {
	isMissing := func(v string) *string { return &v }

	runTxn(t, func(ctx context.Context) error {
		sqb := db.Scene()

		missingDate := querySceneIDSet(ctx, t, sqb, &models.SceneFilterType{IsMissing: isMissing("date")})
		missingTags := querySceneIDSet(ctx, t, sqb, &models.SceneFilterType{IsMissing: isMissing("tags")})
		missingRating := querySceneIDSet(ctx, t, sqb, &models.SceneFilterType{IsMissing: isMissing("rating")})

		// is missing date, and is missing tags, and is missing rating
		chained := models.SceneFilterType{
			IsMissing: isMissing("date"),
			OperatorFilter: models.OperatorFilter[models.SceneFilterType]{
				And: &models.SceneFilterType{
					IsMissing: isMissing("tags"),
					OperatorFilter: models.OperatorFilter[models.SceneFilterType]{
						And: &models.SceneFilterType{
							IsMissing: isMissing("rating"),
						},
					},
				},
			},
		}

		got := scenesToIDs(queryScene(ctx, t, sqb, &chained, allScenesFindFilter()))
		want := intersectIDs(missingDate, missingTags, missingRating)

		assert.NotEmpty(t, got)
		assert.ElementsMatch(t, want, got)
		// the chain narrows the results: each condition counts
		assert.Less(t, len(got), len(missingDate))

		return nil
	})
}

func TestSceneQueryNestedAndSameTextField(t *testing.T) {
	runTxn(t, func(ctx context.Context) error {
		sqb := db.Scene()

		includes := models.StringCriterionInput{Value: "scene_", Modifier: models.CriterionModifierIncludes}
		excludes := models.StringCriterionInput{Value: "_000", Modifier: models.CriterionModifierExcludes}

		withIncludes := querySceneIDSet(ctx, t, sqb, &models.SceneFilterType{Title: &includes})
		withExcludes := querySceneIDSet(ctx, t, sqb, &models.SceneFilterType{Title: &excludes})

		// title includes "scene_" and title excludes "_000" (scenes 0 to 9)
		chained := models.SceneFilterType{
			Title: &includes,
			OperatorFilter: models.OperatorFilter[models.SceneFilterType]{
				And: &models.SceneFilterType{Title: &excludes},
			},
		}

		got := scenesToIDs(queryScene(ctx, t, sqb, &chained, allScenesFindFilter()))
		want := intersectIDs(withIncludes, withExcludes)

		assert.NotEmpty(t, got)
		assert.ElementsMatch(t, want, got)
		assert.Less(t, len(got), len(withIncludes))

		return nil
	})
}
