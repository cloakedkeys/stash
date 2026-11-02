//go:build integration

package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Long titles, sort names and URLs save on both databases, as they always did
// on SQLite. Runs against PostgreSQL when PGSQL_TEST is set.
func TestLongTextSaves(t *testing.T) {
	title := strings.Repeat("a long title ", 60) // 780 characters
	url := "https://example.com/" + strings.Repeat("p", 1000)

	runWithRollbackTxn(t, "story", func(t *testing.T, ctx context.Context) {
		story := models.NewStory()
		story.Title = title
		story.URLs = models.NewRelatedStrings([]string{url})
		require.NoError(t, db.Story().Create(ctx, &story, nil))

		got, err := db.Story().Find(ctx, story.ID)
		require.NoError(t, err)
		assert.Equal(t, title, got.Title)
		urls, err := db.Story().GetURLs(ctx, story.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{url}, urls)
	})

	runWithRollbackTxn(t, "audio", func(t *testing.T, ctx context.Context) {
		now := time.Now()
		audio := models.Audio{
			Title:     title,
			URLs:      models.NewRelatedStrings([]string{url}),
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, db.Audio().Create(ctx, &audio, nil))

		got, err := db.Audio().Find(ctx, audio.ID)
		require.NoError(t, err)
		assert.Equal(t, title, got.Title)
		urls, err := db.Audio().GetURLs(ctx, audio.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{url}, urls)
	})

	runWithRollbackTxn(t, "tag sort name", func(t *testing.T, ctx context.Context) {
		tag := models.NewTag()
		tag.Name = "long sort name"
		tag.SortName = title
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: &tag}))

		got, err := db.Tag().Find(ctx, tag.ID)
		require.NoError(t, err)
		assert.Equal(t, title, got.SortName)
	})
}
