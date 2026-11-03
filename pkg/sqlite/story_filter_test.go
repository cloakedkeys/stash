//go:build integration

package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

// Import story looks up a story by its web address before adding it again
// (#1964). Any of a story's addresses matches.
func TestStoryURLFilter(t *testing.T) {
	_ = config.InitializeEmpty()
	ctx := context.Background()
	db := NewDatabase()
	require.NoError(t, db.Open(ctx, filepath.Join(t.TempDir(), "stories.sqlite")))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	r := db.Repository()

	stories := map[string][]string{
		"a": {"https://www.literotica.com/s/a-story", "https://example.com/a"},
		"b": {"https://www.literotica.com/s/b-story"},
		"c": nil,
	}
	ids := map[int]string{}
	require.NoError(t, txn.WithTxn(ctx, db, func(ctx context.Context) error {
		for title, urls := range stories {
			s := &models.Story{Title: title, URLs: models.NewRelatedStrings(urls)}
			if err := r.Story.Create(ctx, s, nil); err != nil {
				return err
			}
			ids[s.ID] = title
		}
		return nil
	}))

	for _, tc := range []struct {
		name string
		url  models.StringCriterionInput
		want []string
	}{
		{"first address", models.StringCriterionInput{Value: "https://www.literotica.com/s/a-story", Modifier: models.CriterionModifierEquals}, []string{"a"}},
		{"second address", models.StringCriterionInput{Value: "https://example.com/a", Modifier: models.CriterionModifierEquals}, []string{"a"}},
		{"part of an address", models.StringCriterionInput{Value: "literotica.com/s/", Modifier: models.CriterionModifierIncludes}, []string{"a", "b"}},
		{"no address", models.StringCriterionInput{Modifier: models.CriterionModifierIsNull}, []string{"c"}},
		{"unknown address", models.StringCriterionInput{Value: "https://www.literotica.com/s/other", Modifier: models.CriterionModifierEquals}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			require.NoError(t, txn.WithReadTxn(ctx, db, func(ctx context.Context) error {
				url := tc.url
				res, err := r.Story.Query(ctx, models.StoryQueryOptions{StoryFilter: &models.StoryFilterType{URL: &url}})
				if err != nil {
					return err
				}
				for _, id := range res.IDs {
					got = append(got, ids[id])
				}
				return nil
			}))
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}
