//go:build integration

package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Runs against SQLite normally and PostgreSQL when PGSQL_TEST is set.
func TestPerformerPlayCountSameSecond(t *testing.T) {
	for _, sameScene := range []bool{true, false} {
		name := "different scenes"
		if sameScene {
			name = "same scene"
		}
		runWithRollbackTxn(t, name, func(t *testing.T, ctx context.Context) {
			createPerformer := func(suffix string) int {
				p := models.NewPerformer()
				p.Name = t.Name() + suffix
				require.NoError(t, db.Performer().Create(ctx, &models.CreatePerformerInput{Performer: &p}))
				return p.ID
			}
			repeated := createPerformer(" repeated")
			control := createPerformer(" control")
			unplayed := createPerformer(" unplayed")
			noScenes := createPerformer(" no scenes")
			createScene := func(performerID int, dates ...time.Time) int {
				s := models.NewScene()
				s.PerformerIDs = models.NewRelatedIDs([]int{performerID})
				require.NoError(t, db.Scene().Create(ctx, &s, nil))
				if len(dates) > 0 {
					_, err := db.Scene().AddViews(ctx, s.ID, dates)
					require.NoError(t, err)
				}
				count, err := db.Scene().CountViews(ctx, s.ID)
				require.NoError(t, err)
				require.Equal(t, len(dates), count)
				return s.ID
			}
			date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			var sceneID int
			if sameScene {
				sceneID = createScene(repeated, date, date, date)
			} else {
				sceneID = createScene(repeated, date, date)
				createScene(repeated, date)
			}
			createScene(repeated) // An unplayed scene must not add a play.
			createScene(control, date, date.Add(time.Second))
			createScene(unplayed)

			other := models.User{Username: t.Name(), Roles: models.Roles{models.RoleEnumRead}}
			require.NoError(t, db.User().Create(ctx, &other, ""))
			otherCtx := models.WithUserID(ctx, other.ID)
			_, err := db.Scene().AddViews(otherCtx, sceneID, []time.Time{date.Add(2 * time.Second)})
			require.NoError(t, err)

			filter := &models.PerformerFilterType{
				Name: &models.StringCriterionInput{Value: t.Name(), Modifier: models.CriterionModifierIncludes},
			}
			queryIDs := func(ctx context.Context, find *models.FindFilterType) []int {
				performers, _, err := db.Performer().Query(ctx, filter, find)
				require.NoError(t, err)
				ids := make([]int, len(performers))
				for i, p := range performers {
					ids[i] = p.ID
				}
				return ids
			}
			for count, want := range map[int][]int{
				0: {unplayed, noScenes},
				2: {control},
				3: {repeated},
			} {
				filter.PlayCount = &models.IntCriterionInput{Value: count, Modifier: models.CriterionModifierEquals}
				assert.ElementsMatch(t, want, queryIDs(ctx, nil), "play count %d", count)
				total, err := db.Performer().QueryCount(ctx, filter, nil)
				require.NoError(t, err)
				assert.Equal(t, len(want), total, "play count %d", count)
			}
			filter.PlayCount = &models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierEquals}
			assert.Equal(t, []int{repeated}, queryIDs(otherCtx, nil), "history belongs to the current user")

			filter.PlayCount = &models.IntCriterionInput{Value: 0, Modifier: models.CriterionModifierGreaterThan}
			sort := "play_count"
			asc, desc := models.SortDirectionEnumAsc, models.SortDirectionEnumDesc
			assert.Equal(t, []int{control, repeated}, queryIDs(ctx, &models.FindFilterType{Sort: &sort, Direction: &asc}))
			assert.Equal(t, []int{repeated, control}, queryIDs(ctx, &models.FindFilterType{Sort: &sort, Direction: &desc}))
		})
	}
}
