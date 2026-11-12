//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestSceneStore_GetManyGroupSceneCount(t *testing.T) {
	counter, ok := db.Scene().(interface {
		GetManyGroupSceneCount(context.Context, []int) ([]int, error)
	})
	require.True(t, ok, "scene counts must be available as a batch")
	runRollbackTxn(t, func(ctx context.Context) error {
		groups := []*models.Group{{Name: "Parts"}, {Name: "Other parts"}, {Name: "Empty"}, {Name: "Child"}}
		for _, g := range groups {
			require.NoError(t, db.Group().Create(ctx, g))
		}
		_, err := db.Group().UpdatePartial(ctx, groups[3].ID, models.GroupPartial{
			ContainingGroups: &models.UpdateGroupDescriptions{Mode: models.RelationshipUpdateModeSet, Groups: []models.GroupIDDescription{{GroupID: groups[0].ID}}},
		})
		require.NoError(t, err)
		for i := range 5 {
			links := []models.GroupsScenes{{GroupID: groups[0].ID}}
			if i < 2 {
				links = append(links, models.GroupsScenes{GroupID: groups[1].ID})
			}
			s := &models.Scene{Title: "Part", Groups: models.NewRelatedGroups(links)}
			require.NoError(t, db.Scene().Create(ctx, s, nil))
		}
		child := &models.Scene{Title: "Child", Groups: models.NewRelatedGroups([]models.GroupsScenes{{GroupID: groups[3].ID}})}
		require.NoError(t, db.Scene().Create(ctx, child, nil))
		ids := []int{groups[1].ID, groups[0].ID, groups[2].ID, 0, groups[0].ID}
		counts, err := counter.GetManyGroupSceneCount(ctx, ids)
		require.NoError(t, err)
		require.Equal(t, []int{2, 5, 0, 0, 5}, counts)
		counts, err = counter.GetManyGroupSceneCount(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, counts)
		counts, err = counter.GetManyGroupSceneCount(models.WithLibraryLimit(ctx, &models.LibraryLimit{}), ids)
		require.NoError(t, err)
		require.Equal(t, []int{0, 0, 0, 0, 0}, counts)
		return nil
	})
}
