//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This shared suite runs on SQLite and on PostgreSQL via PGSQL_TEST.
func TestPerformerOrganized(t *testing.T) {
	runWithRollbackTxn(t, "set and filter", func(t *testing.T, ctx context.Context) {
		qb := db.Performer()
		kept := models.NewPerformer()
		kept.Name = t.Name() + " kept"
		kept.Organized = true
		other := models.NewPerformer()
		other.Name = t.Name() + " other"
		require.NoError(t, qb.Create(ctx, &models.CreatePerformerInput{Performer: &kept}))
		require.NoError(t, qb.Create(ctx, &models.CreatePerformerInput{Performer: &other}))
		stored, err := qb.Find(ctx, kept.ID)
		require.NoError(t, err)
		require.True(t, stored.Organized)
		perPage := -1
		for _, value := range []bool{true, false} {
			found, _, err := qb.Query(ctx, &models.PerformerFilterType{Organized: &value}, &models.FindFilterType{PerPage: &perPage})
			require.NoError(t, err)
			var ids []int
			for _, item := range found {
				ids = append(ids, item.ID)
			}
			if value {
				assert.Contains(t, ids, kept.ID)
				assert.NotContains(t, ids, other.ID)
			} else {
				assert.Contains(t, ids, other.ID)
				assert.NotContains(t, ids, kept.ID)
			}
		}
		// Bulk mutations apply the same partial to each selected object.
		for _, value := range []bool{true, false} {
			partial := models.PerformerPartial{Organized: models.NewOptionalBool(value)}
			for _, id := range []int{kept.ID, other.ID} {
				_, err := qb.UpdatePartial(ctx, id, partial)
				require.NoError(t, err)
				stored, err := qb.Find(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, value, stored.Organized)
			}
		}
		kept.Organized = true
		require.NoError(t, qb.Update(ctx, &models.UpdatePerformerInput{Performer: &kept}))
		_, err = qb.UpdatePartial(ctx, kept.ID, models.PerformerPartial{Name: models.NewOptionalString("Renamed " + t.Name())})
		require.NoError(t, err)
		stored, err = qb.Find(models.WithUserID(ctx, 0), kept.ID)
		require.NoError(t, err)
		assert.True(t, stored.Organized, "organized is shared and survives unrelated edits")
	})
	for _, organized := range []bool{true, false} {
		runWithRollbackTxn(t, map[bool]string{true: "kept organized", false: "kept unorganized"}[organized], func(t *testing.T, ctx context.Context) {
			qb := db.Performer()
			kept := models.NewPerformer()
			kept.Name = t.Name() + " kept"
			kept.Organized = organized
			source := models.NewPerformer()
			source.Name = t.Name() + " source"
			source.Organized = !organized
			require.NoError(t, qb.Create(ctx, &models.CreatePerformerInput{Performer: &kept}))
			require.NoError(t, qb.Create(ctx, &models.CreatePerformerInput{Performer: &source}))
			require.NoError(t, qb.Merge(ctx, []int{source.ID}, kept.ID))
			stored, err := qb.Find(ctx, kept.ID)
			require.NoError(t, err)
			assert.Equal(t, organized, stored.Organized)
		})
	}
}

func TestTagOrganized(t *testing.T) {
	runWithRollbackTxn(t, "set and filter", func(t *testing.T, ctx context.Context) {
		qb := db.Tag()
		kept := models.NewTag()
		kept.Name = t.Name() + " kept"
		kept.Organized = true
		other := models.NewTag()
		other.Name = t.Name() + " other"
		require.NoError(t, qb.Create(ctx, &models.CreateTagInput{Tag: &kept}))
		require.NoError(t, qb.Create(ctx, &models.CreateTagInput{Tag: &other}))
		stored, err := qb.Find(ctx, kept.ID)
		require.NoError(t, err)
		require.True(t, stored.Organized)
		perPage := -1
		for _, value := range []bool{true, false} {
			found, _, err := qb.Query(ctx, &models.TagFilterType{Organized: &value}, &models.FindFilterType{PerPage: &perPage})
			require.NoError(t, err)
			var ids []int
			for _, item := range found {
				ids = append(ids, item.ID)
			}
			if value {
				assert.Contains(t, ids, kept.ID)
				assert.NotContains(t, ids, other.ID)
			} else {
				assert.Contains(t, ids, other.ID)
				assert.NotContains(t, ids, kept.ID)
			}
		}
		// Bulk mutations apply the same partial to each selected object.
		for _, value := range []bool{true, false} {
			partial := models.TagPartial{Organized: models.NewOptionalBool(value)}
			for _, id := range []int{kept.ID, other.ID} {
				_, err := qb.UpdatePartial(ctx, id, partial)
				require.NoError(t, err)
				stored, err := qb.Find(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, value, stored.Organized)
			}
		}
		kept.Organized = true
		require.NoError(t, qb.Update(ctx, &models.UpdateTagInput{Tag: &kept}))
		_, err = qb.UpdatePartial(ctx, kept.ID, models.TagPartial{Name: models.NewOptionalString("Renamed " + t.Name())})
		require.NoError(t, err)
		stored, err = qb.Find(models.WithUserID(ctx, 0), kept.ID)
		require.NoError(t, err)
		assert.True(t, stored.Organized, "organized is shared and survives unrelated edits")
	})
	for _, organized := range []bool{true, false} {
		runWithRollbackTxn(t, map[bool]string{true: "kept organized", false: "kept unorganized"}[organized], func(t *testing.T, ctx context.Context) {
			qb := db.Tag()
			kept := models.NewTag()
			kept.Name = t.Name() + " kept"
			kept.Organized = organized
			source := models.NewTag()
			source.Name = t.Name() + " source"
			source.Organized = !organized
			require.NoError(t, qb.Create(ctx, &models.CreateTagInput{Tag: &kept}))
			require.NoError(t, qb.Create(ctx, &models.CreateTagInput{Tag: &source}))
			require.NoError(t, qb.Merge(ctx, []int{source.ID}, kept.ID))
			stored, err := qb.Find(ctx, kept.ID)
			require.NoError(t, err)
			assert.Equal(t, organized, stored.Organized)
		})
	}
}
