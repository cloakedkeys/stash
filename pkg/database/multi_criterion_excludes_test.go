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

// #2235: some filters ignored the criterion's excludes list, so objects
// linked to an excluded item still showed up. Like the Tags filter, an
// object linked to an excluded item must be left out.
func TestMultiCriterionExcludesList(t *testing.T) {
	ids := func(in ...int) []string {
		var ret []string
		for _, id := range in {
			ret = append(ret, strconv.Itoa(id))
		}
		return ret
	}
	newPerformer := func(t *testing.T, ctx context.Context, name string) int {
		p := models.Performer{Name: "excludes list " + name}
		require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
		return p.ID
	}
	newGroup := func(t *testing.T, ctx context.Context, name string) int {
		g := models.Group{Name: "excludes list " + name}
		require.NoError(t, db.Group().Create(ctx, &g))
		return g.ID
	}
	newScene := func(t *testing.T, ctx context.Context, s models.Scene) int {
		s.Title = "excludes list scene"
		require.NoError(t, db.Scene().Create(ctx, &s, nil))
		return s.ID
	}
	inGroups := func(groups ...int) models.RelatedGroups {
		var gs []models.GroupsScenes
		for _, g := range groups {
			gs = append(gs, models.GroupsScenes{GroupID: g})
		}
		return models.NewRelatedGroups(gs)
	}
	performerIDs := func(t *testing.T, ctx context.Context, f *models.PerformerFilterType) []int {
		performers, _, err := db.Performer().Query(ctx, f, perPageAll())
		require.NoError(t, err)
		var ret []int
		for _, p := range performers {
			ret = append(ret, p.ID)
		}
		return ret
	}
	all := -1

	runWithRollbackTxn(t, "performers by groups", func(t *testing.T, ctx context.Context) {
		a, x := newGroup(t, ctx, "group A"), newGroup(t, ctx, "group X")
		inA := newPerformer(t, ctx, "in A")
		inAX := newPerformer(t, ctx, "in A and X, one scene")
		inAThenX := newPerformer(t, ctx, "in A and X, two scenes")
		newScene(t, ctx, models.Scene{Groups: inGroups(a), PerformerIDs: models.NewRelatedIDs([]int{inA})})
		newScene(t, ctx, models.Scene{Groups: inGroups(a, x), PerformerIDs: models.NewRelatedIDs([]int{inAX})})
		newScene(t, ctx, models.Scene{Groups: inGroups(a), PerformerIDs: models.NewRelatedIDs([]int{inAThenX})})
		newScene(t, ctx, models.Scene{Groups: inGroups(x), PerformerIDs: models.NewRelatedIDs([]int{inAThenX})})

		for _, depth := range []*int{nil, &all} {
			got := performerIDs(t, ctx, &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{
				Value: ids(a), Excludes: ids(x), Modifier: models.CriterionModifierIncludes, Depth: depth,
			}})
			assert.Equal(t, []int{inA}, got, "includes A, excludes X, depth %v", depth)
		}
		// unchanged without an excludes list
		got := performerIDs(t, ctx, &models.PerformerFilterType{Groups: &models.HierarchicalMultiCriterionInput{
			Value: ids(a), Modifier: models.CriterionModifierIncludes,
		}})
		assert.ElementsMatch(t, []int{inA, inAX, inAThenX}, got)
	})

	runWithRollbackTxn(t, "performers by performers they appear with", func(t *testing.T, ctx context.Context) {
		q, r := newPerformer(t, ctx, "Q"), newPerformer(t, ctx, "R")
		withQ := newPerformer(t, ctx, "with Q")
		withQAndR := newPerformer(t, ctx, "with Q and R")
		newScene(t, ctx, models.Scene{PerformerIDs: models.NewRelatedIDs([]int{withQ, q})})
		newScene(t, ctx, models.Scene{PerformerIDs: models.NewRelatedIDs([]int{withQAndR, q})})
		newScene(t, ctx, models.Scene{PerformerIDs: models.NewRelatedIDs([]int{withQAndR, r})})

		got := performerIDs(t, ctx, &models.PerformerFilterType{Performers: &models.MultiCriterionInput{
			Value: ids(q), Excludes: ids(r), Modifier: models.CriterionModifierIncludes,
		}})
		assert.Equal(t, []int{withQ}, got)
		// unchanged without an excludes list
		got = performerIDs(t, ctx, &models.PerformerFilterType{Performers: &models.MultiCriterionInput{
			Value: ids(q), Modifier: models.CriterionModifierIncludes,
		}})
		assert.ElementsMatch(t, []int{withQ, withQAndR}, got)
		// only an excludes list: every performer except those who appear with R
		got = performerIDs(t, ctx, &models.PerformerFilterType{Performers: &models.MultiCriterionInput{
			Excludes: ids(r), Modifier: models.CriterionModifierIncludes,
		}})
		assert.Contains(t, got, withQ)
		assert.Contains(t, got, q)
		assert.NotContains(t, got, withQAndR)
	})

	runWithRollbackTxn(t, "groups by performers", func(t *testing.T, ctx context.Context) {
		p, z := newPerformer(t, ctx, "P"), newPerformer(t, ctx, "Z")
		withP := newGroup(t, ctx, "with P")
		withPZ := newGroup(t, ctx, "with P and Z, one scene")
		withPThenZ := newGroup(t, ctx, "with P and Z, two scenes")
		newScene(t, ctx, models.Scene{Groups: inGroups(withP), PerformerIDs: models.NewRelatedIDs([]int{p})})
		newScene(t, ctx, models.Scene{Groups: inGroups(withPZ), PerformerIDs: models.NewRelatedIDs([]int{p, z})})
		newScene(t, ctx, models.Scene{Groups: inGroups(withPThenZ), PerformerIDs: models.NewRelatedIDs([]int{p})})
		newScene(t, ctx, models.Scene{Groups: inGroups(withPThenZ), PerformerIDs: models.NewRelatedIDs([]int{z})})

		groupIDs := func(f *models.GroupFilterType) []int {
			groups, _, err := db.Group().Query(ctx, f, perPageAll())
			require.NoError(t, err)
			var ret []int
			for _, g := range groups {
				ret = append(ret, g.ID)
			}
			return ret
		}
		got := groupIDs(&models.GroupFilterType{Performers: &models.MultiCriterionInput{
			Value: ids(p), Excludes: ids(z), Modifier: models.CriterionModifierIncludes,
		}})
		assert.Equal(t, []int{withP}, got)
		// unchanged without an excludes list
		got = groupIDs(&models.GroupFilterType{Performers: &models.MultiCriterionInput{
			Value: ids(p), Modifier: models.CriterionModifierIncludes,
		}})
		assert.ElementsMatch(t, []int{withP, withPZ, withPThenZ}, got)
	})

	runWithRollbackTxn(t, "scenes by galleries", func(t *testing.T, ctx context.Context) {
		newGallery := func(name string) int {
			g := models.Gallery{Title: "excludes list " + name}
			require.NoError(t, db.Gallery().Create(ctx, &models.CreateGalleryInput{Gallery: &g}))
			return g.ID
		}
		a, x := newGallery("gallery A"), newGallery("gallery X")
		inA := newScene(t, ctx, models.Scene{GalleryIDs: models.NewRelatedIDs([]int{a})})
		newScene(t, ctx, models.Scene{GalleryIDs: models.NewRelatedIDs([]int{a, x})})

		res, err := db.Scene().Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
			SceneFilter: &models.SceneFilterType{Galleries: &models.MultiCriterionInput{
				Value: ids(a), Excludes: ids(x), Modifier: models.CriterionModifierIncludes,
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, []int{inA}, res.IDs)
	})

	runWithRollbackTxn(t, "studios by parent studio", func(t *testing.T, ctx context.Context) {
		newStudio := func(name string, parent *int) int {
			s := models.Studio{Name: "excludes list " + name, ParentID: parent}
			require.NoError(t, db.Studio().Create(ctx, &models.CreateStudioInput{Studio: &s}))
			return s.ID
		}
		pa, px := newStudio("parent A", nil), newStudio("parent X", nil)
		underA, underX := newStudio("under A", &pa), newStudio("under X", &px)

		// only an excludes list: every studio except those under X
		studios, _, err := db.Studio().Query(ctx, &models.StudioFilterType{Parents: &models.MultiCriterionInput{
			Excludes: ids(px), Modifier: models.CriterionModifierIncludes,
		}}, perPageAll())
		require.NoError(t, err)
		var got []int
		for _, s := range studios {
			got = append(got, s.ID)
		}
		assert.Contains(t, got, underA)
		assert.Contains(t, got, pa)
		assert.NotContains(t, got, underX)
	})
}
