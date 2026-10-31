//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestSceneCoverProvenance(t *testing.T) {
	err := withRollbackTxn(func(ctx context.Context) error {
		s := models.NewScene()
		require.NoError(t, db.Scene().Create(ctx, &s, nil))
		partial := models.NewScenePartial()
		partial.CoverGenerated = models.NewOptionalBool(true)
		_, err := db.Scene().UpdatePartial(ctx, s.ID, partial)
		require.NoError(t, err)
		got, err := db.Scene().Find(ctx, s.ID)
		require.NoError(t, err)
		require.True(t, got.CoverGenerated)
		// Uploaded, scraped and imported covers must clear generated provenance.
		require.NoError(t, db.Scene().UpdateCover(ctx, s.ID, []byte("custom cover")))
		got, err = db.Scene().Find(ctx, s.ID)
		require.NoError(t, err)
		require.False(t, got.CoverGenerated)
		cover, err := db.Scene().GetCover(ctx, s.ID)
		require.NoError(t, err)
		require.Equal(t, []byte("custom cover"), cover)
		return nil
	})
	require.NoError(t, err)
}
