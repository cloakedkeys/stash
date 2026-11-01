//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
)

// The shared store suite runs against SQLite by default and PostgreSQL with PGSQL_TEST.
func TestGroupStoreOrganized(t *testing.T) {
	runWithRollbackTxn(t, "organized", func(t *testing.T, ctx context.Context) {
		qb := db.Group()
		plain := models.NewGroup()
		plain.Name = "TestGroupStoreOrganized"
		require.NoError(t, qb.Create(ctx, &plain))
		assert.False(t, plain.Organized)

		organized := models.NewGroup()
		organized.Name = plain.Name
		organized.Organized = true
		require.NoError(t, qb.Create(ctx, &organized))
		assert.True(t, organized.Organized)

		found, err := qb.Find(ctx, organized.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.True(t, found.Organized)

		// Both full and partial updates must be able to clear as well as set it.
		for _, value := range []bool{false, true} {
			organized.Organized = value
			require.NoError(t, qb.Update(ctx, &organized))
			found, err = qb.Find(ctx, organized.ID)
			require.NoError(t, err)
			require.NotNil(t, found)
			assert.Equal(t, value, found.Organized)
		}
		for _, value := range []bool{false, true} {
			found, err = qb.UpdatePartial(ctx, organized.ID, models.GroupPartial{
				Organized: models.NewOptionalBool(value),
			})
			require.NoError(t, err)
			require.NotNil(t, found)
			assert.Equal(t, value, found.Organized)
		}
		found, err = qb.UpdatePartial(ctx, organized.ID, models.NewGroupPartial())
		require.NoError(t, err)
		assert.True(t, found.Organized, "an omitted flag leaves the value alone")

		filter := &models.GroupFilterType{
			Name: &models.StringCriterionInput{
				Value:    plain.Name,
				Modifier: models.CriterionModifierEquals,
			},
		}
		for _, value := range []bool{false, true} {
			filter.Organized = &value
			groups, count, err := qb.Query(ctx, filter, nil)
			require.NoError(t, err)
			require.Equal(t, 1, count)
			require.Len(t, groups, 1)
			wantID := plain.ID
			if value {
				wantID = organized.ID
			}
			assert.Equal(t, wantID, groups[0].ID)
			assert.Equal(t, value, groups[0].Organized)
		}

		filter.Organized = nil
		sortBy := "organized"
		for _, direction := range []models.SortDirectionEnum{models.SortDirectionEnumAsc, models.SortDirectionEnumDesc} {
			groups, count, err := qb.Query(ctx, filter, &models.FindFilterType{
				Sort:      &sortBy,
				Direction: &direction,
			})
			require.NoError(t, err)
			require.Equal(t, 2, count)
			require.Len(t, groups, 2)
			wantIDs := []int{plain.ID, organized.ID}
			if direction == models.SortDirectionEnumDesc {
				wantIDs = []int{organized.ID, plain.ID}
			}
			assert.Equal(t, wantIDs, groupsToIDs(groups))
		}
	})
}
