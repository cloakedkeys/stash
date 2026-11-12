//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

// #1272: shared suite runs on SQLite and on PostgreSQL with PGSQL_TEST.
func TestSceneStore_FindDuplicatesByStashID(t *testing.T) {
	runWithRollbackTxn(t, "stash ID", func(t *testing.T, ctx context.Context) {
		makeScene := func(endpoint, id string, organized bool) int {
			s := models.NewScene()
			s.Title = "stash-id-duplicates-1272"
			s.Organized = organized
			if id != "" {
				s.StashIDs = models.NewRelatedStashIDs([]models.StashID{{Endpoint: endpoint, StashID: id}})
			}
			require.NoError(t, db.Scene().Create(ctx, &s, nil))
			return s.ID
		}
		first := makeScene("https://one.example/graphql", "shared", false)
		second := makeScene("https://one.example/graphql", "shared", false)
		third := makeScene("https://one.example/graphql", "shared", true)
		makeScene("https://two.example/graphql", "shared", false)
		makeScene("https://one.example/graphql", "different", false)
		makeScene("", "", false)

		organized, unorganized := true, false
		for _, tc := range []struct {
			name      string
			stashID   bool
			organized *bool
			want      []int
		}{
			{name: "same endpoint and ID without phashes", stashID: true, want: []int{first, second, third}},
			{name: "filter removes one member", stashID: true, organized: &unorganized, want: []int{first, second}},
			{name: "filter leaves no group", stashID: true, organized: &organized},
			{name: "phash mode does not match stash IDs"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				filter := &models.SceneFilterType{Title: &models.StringCriterionInput{Value: "stash-id-duplicates-1272", Modifier: models.CriterionModifierEquals}, Organized: tc.organized}
				// Stash ID mode ignores even restrictive phash distance and duration settings.
				groups, err := db.Scene().FindDuplicates(ctx, 0, 0, filter, tc.stashID)
				require.NoError(t, err)
				if len(tc.want) == 0 {
					require.Empty(t, groups)
					return
				}
				require.Len(t, groups, 1)
				var ids []int
				for _, scene := range groups[0] {
					ids = append(ids, scene.ID)
				}
				require.ElementsMatch(t, tc.want, ids)
			})
		}
	})
}
