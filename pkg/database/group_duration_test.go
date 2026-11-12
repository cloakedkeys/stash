//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
)

// Runs against SQLite normally and PostgreSQL when PGSQL_TEST is set.
func TestGroupQueryNullDuration(t *testing.T) {
	runWithRollbackTxn(t, "null duration", func(t *testing.T, ctx context.Context) {
		qb := db.Group()
		missing := models.NewGroup()
		missing.Name = "TestGroupQueryNullDuration"
		require.NoError(t, qb.Create(ctx, &missing))

		zero := models.NewGroup()
		zero.Name = missing.Name
		zeroDuration := 0
		zero.Duration = &zeroDuration
		require.NoError(t, qb.Create(ctx, &zero))

		known := models.NewGroup()
		known.Name = missing.Name
		duration := 3600
		known.Duration = &duration
		require.NoError(t, qb.Create(ctx, &known))

		for _, modifier := range []models.CriterionModifier{models.CriterionModifierIsNull, models.CriterionModifierNotNull} {
			t.Run(string(modifier), func(t *testing.T) {
				groups, count, err := qb.Query(ctx, &models.GroupFilterType{
					Name: &models.StringCriterionInput{
						Value: missing.Name, Modifier: models.CriterionModifierEquals,
					},
					Duration: &models.IntCriterionInput{Modifier: modifier},
				}, nil)
				require.NoError(t, err)
				wantIDs := []int{missing.ID}
				if modifier == models.CriterionModifierNotNull {
					wantIDs = []int{zero.ID, known.ID}
				}
				assert.Equal(t, len(wantIDs), count)
				assert.ElementsMatch(t, wantIDs, groupsToIDs(groups))
			})
		}
	})
}
