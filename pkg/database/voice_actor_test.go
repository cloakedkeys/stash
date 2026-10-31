//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestVoiceActorSharedMetadataAndPrivateFavorites(t *testing.T) {
	require.NoError(t, withRollbackTxn(func(ctx context.Context) error {
		store := db.VoiceActor()
		other := models.User{Username: "voice-actor-other", Roles: models.Roles{models.RoleEnumModify}}
		require.NoError(t, db.User().Create(ctx, &other, ""))
		otherCtx := models.WithUserID(ctx, other.ID)
		guestCtx := models.WithUserID(ctx, 0)
		a := &models.VoiceActor{Name: " Shared Artist ", Aliases: []string{"Alias", "ALIAS", " "}, URLs: []string{"https://example.com/Artist", "https://example.com/artist"}, Favorite: true}
		require.NoError(t, store.Create(ctx, a))
		found, err := store.FindByName(ctx, "shared artist")
		require.NoError(t, err)
		require.Equal(t, "Shared Artist", found.Name)
		require.Equal(t, []string{"Alias"}, found.Aliases)
		require.Equal(t, a.URLs, found.URLs)
		require.True(t, found.Favorite)
		for _, c := range []context.Context{otherCtx, guestCtx} {
			found, err = store.Find(c, a.ID)
			require.NoError(t, err)
			require.False(t, found.Favorite)
			_, count, err := store.Query(c, nil, ptr(true))
			require.NoError(t, err)
			require.Zero(t, count)
		}
		found, err = store.Find(otherCtx, a.ID)
		require.NoError(t, err)
		found.Name, found.Favorite = "Renamed Artist", true
		require.NoError(t, store.Update(otherCtx, found))
		a.Name, a.Favorite = found.Name, false
		require.NoError(t, store.Update(ctx, a))
		found, err = store.Find(otherCtx, a.ID)
		require.NoError(t, err)
		require.True(t, found.Favorite)
		found, err = store.Find(guestCtx, a.ID)
		require.NoError(t, err)
		require.Equal(t, "Renamed Artist", found.Name)
		require.False(t, found.Favorite)

		scenes := []*models.Scene{{Title: "voice target"}, {Title: "voice source"}}
		for _, scene := range scenes {
			scene.CreatedAt, scene.UpdatedAt = time.Now().UTC(), time.Now().UTC()
			require.NoError(t, db.Scene().Create(ctx, scene, nil))
		}
		require.NoError(t, store.SetScene(ctx, scenes[1].ID, []int{a.ID, a.ID}))
		require.NoError(t, store.MergeScenes(ctx, scenes[0].ID, []int{scenes[1].ID}))
		require.NoError(t, db.Scene().Destroy(ctx, scenes[1].ID))
		credits, err := store.FindBySceneID(otherCtx, scenes[0].ID)
		require.NoError(t, err)
		require.Len(t, credits, 1)
		require.True(t, credits[0].Favorite)
		require.NoError(t, db.Scene().Destroy(ctx, scenes[0].ID))
		ids, err := store.SceneIDs(ctx, a.ID)
		require.NoError(t, err)
		require.Empty(t, ids)
		found, err = store.Find(ctx, a.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		require.NoError(t, store.Destroy(ctx, a.ID))
		return nil
	}))
}
