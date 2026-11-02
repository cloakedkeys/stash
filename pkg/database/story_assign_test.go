//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/story"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// titleSavedMeanwhile gives a story's title just after AssignFile reads the stories that
// have the file, as another session could, so AssignFile holds an untitled copy.
type titleSavedMeanwhile struct {
	models.StoryReaderWriter
	t *testing.T
}

func (r *titleSavedMeanwhile) FindByFileID(ctx context.Context, id models.FileID) ([]*models.Story, error) {
	ret, err := r.StoryReaderWriter.FindByFileID(ctx, id)
	for _, s := range ret {
		_, err := r.UpdatePartial(ctx, s.ID, models.StoryPartial{Title: models.NewOptionalString("Saved meanwhile")})
		require.NoError(r.t, err)
	}
	return ret, err
}

// Runs on SQLite, or PostgreSQL when PGSQL_TEST is set.
func TestStoryAssignFile(t *testing.T) {
	others := func() []story.FileUser {
		return []story.FileUser{db.Scene(), db.Image(), db.Gallery(), db.Audio()}
	}
	create := func(ctx context.Context, t *testing.T, title string, files ...models.FileID) *models.Story {
		t.Helper()
		s := models.NewStory()
		s.Title = title
		require.NoError(t, db.Story().Create(ctx, &s, files))
		return &s
	}
	fileIDs := func(ctx context.Context, t *testing.T, id int) []models.FileID {
		t.Helper()
		ids, err := db.Story().GetManyFileIDs(ctx, []int{id})
		require.NoError(t, err)
		return ids[0]
	}

	runWithRollbackTxn(t, "joins a scanned file to a scraped story", func(t *testing.T, ctx context.Context) {
		scraped := create(ctx, t, "Same name")
		require.NoError(t, db.Story().UpdateText(ctx, scraped.ID, "Scraped text."))
		f := storyFile(ctx, t, "Same name.txt")
		scanned := create(ctx, t, "", f.ID)
		before, err := db.Story().Count(ctx)
		require.NoError(t, err)

		require.NoError(t, story.AssignFile(ctx, db.Story(), others(), scraped.ID, f.ID))

		got, err := db.Story().Find(ctx, scraped.ID)
		require.NoError(t, err)
		assert.Equal(t, "Same name", got.Title, "the scraped details stay")
		assert.Equal(t, &f.ID, got.PrimaryFileID)
		text, err := db.Story().GetText(ctx, scraped.ID)
		require.NoError(t, err)
		assert.Nil(t, text, "the file's text is shown, so the stored text goes")
		gone, err := db.Story().Find(ctx, scanned.ID)
		require.NoError(t, err)
		assert.Nil(t, gone, "the empty story the scan made is removed")
		after, err := db.Story().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, before-1, after)
	})

	runWithRollbackTxn(t, "a story with a title keeps its details", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "kept.txt")
		owner := create(ctx, t, "Kept", f.ID)
		dest := create(ctx, t, "Destination")

		require.NoError(t, story.AssignFile(ctx, db.Story(), others(), dest.ID, f.ID))

		kept, err := db.Story().Find(ctx, owner.ID)
		require.NoError(t, err)
		require.NotNil(t, kept)
		assert.Equal(t, "Kept", kept.Title)
		assert.Nil(t, kept.PrimaryFileID)
		assert.Empty(t, fileIDs(ctx, t, owner.ID))
		assert.Equal(t, []models.FileID{f.ID}, fileIDs(ctx, t, dest.ID))
	})

	runWithRollbackTxn(t, "the old story's other file becomes its main file", func(t *testing.T, ctx context.Context) {
		moved := storyFile(ctx, t, "moved.txt")
		other := storyFile(ctx, t, "other.txt")
		owner := create(ctx, t, "", moved.ID, other.ID)
		dest := create(ctx, t, "Destination")

		require.NoError(t, story.AssignFile(ctx, db.Story(), others(), dest.ID, moved.ID))

		got, err := db.Story().Find(ctx, owner.ID)
		require.NoError(t, err)
		require.NotNil(t, got, "a story that still has a file stays")
		assert.Equal(t, &other.ID, got.PrimaryFileID)
		assert.Equal(t, []models.FileID{other.ID}, fileIDs(ctx, t, owner.ID))
	})

	runWithRollbackTxn(t, "a story with a file gains another", func(t *testing.T, ctx context.Context) {
		first := storyFile(ctx, t, "first.txt")
		second := storyFile(ctx, t, "second.txt")
		dest := create(ctx, t, "Destination", first.ID)
		create(ctx, t, "", second.ID)

		require.NoError(t, story.AssignFile(ctx, db.Story(), others(), dest.ID, second.ID))

		got, err := db.Story().Find(ctx, dest.ID)
		require.NoError(t, err)
		assert.Equal(t, &first.ID, got.PrimaryFileID, "its main file doesn't change")
		assert.ElementsMatch(t, []models.FileID{first.ID, second.ID}, fileIDs(ctx, t, dest.ID))
	})

	runWithRollbackTxn(t, "a file with no story", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "loose.txt")
		dest := create(ctx, t, "Destination")

		require.NoError(t, story.AssignFile(ctx, db.Story(), others(), dest.ID, f.ID))
		assert.Equal(t, []models.FileID{f.ID}, fileIDs(ctx, t, dest.ID))
	})

	runWithRollbackTxn(t, "a file the story already has", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "same.txt")
		dest := create(ctx, t, "Destination", f.ID)

		require.NoError(t, story.AssignFile(ctx, db.Story(), others(), dest.ID, f.ID))
		got, err := db.Story().Find(ctx, dest.ID)
		require.NoError(t, err)
		assert.Equal(t, &f.ID, got.PrimaryFileID)
	})

	runWithRollbackTxn(t, "refuses a file something else uses", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "shared.txt")
		a := models.NewAudio()
		require.NoError(t, db.Audio().Create(ctx, &a, []models.FileID{f.ID}))
		dest := create(ctx, t, "Destination")
		require.NoError(t, db.Story().UpdateText(ctx, dest.ID, "Scraped text."))

		err := story.AssignFile(ctx, db.Story(), others(), dest.ID, f.ID)
		assert.ErrorIs(t, err, story.ErrFileUsedElsewhere)
		assert.Empty(t, fileIDs(ctx, t, dest.ID))
		text, err := db.Story().GetText(ctx, dest.ID)
		require.NoError(t, err)
		require.NotNil(t, text, "nothing changes")
	})

	runWithRollbackTxn(t, "a title saved after the old story was read keeps it", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "raced.txt")
		owner := create(ctx, t, "", f.ID)
		dest := create(ctx, t, "Destination")
		repo := &titleSavedMeanwhile{StoryReaderWriter: db.Story(), t: t}

		require.NoError(t, story.AssignFile(ctx, repo, others(), dest.ID, f.ID))

		got, err := db.Story().Find(ctx, owner.ID)
		require.NoError(t, err)
		require.NotNil(t, got, "the story someone just titled isn't removed")
		assert.Equal(t, "Saved meanwhile", got.Title)
		assert.Equal(t, []models.FileID{f.ID}, fileIDs(ctx, t, dest.ID))
	})

	runWithRollbackTxn(t, "DestroyIfEmpty removes only a story with no title and no files", func(t *testing.T, ctx context.Context) {
		empty := create(ctx, t, "")
		titled := create(ctx, t, "Titled")
		withFile := create(ctx, t, "", storyFile(ctx, t, "has-file.txt").ID)

		for _, c := range []struct {
			id      int
			removed bool
		}{{empty.ID, true}, {titled.ID, false}, {withFile.ID, false}} {
			removed, err := db.Story().DestroyIfEmpty(ctx, c.id)
			require.NoError(t, err)
			assert.Equal(t, c.removed, removed)
			got, err := db.Story().Find(ctx, c.id)
			require.NoError(t, err)
			assert.Equal(t, c.removed, got == nil)
		}
	})

	runWithRollbackTxn(t, "refuses a story that doesn't exist", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "nowhere.txt")
		owner := create(ctx, t, "", f.ID)

		assert.Error(t, story.AssignFile(ctx, db.Story(), others(), invalidID, f.ID))
		assert.Equal(t, []models.FileID{f.ID}, fileIDs(ctx, t, owner.ID))
	})
}
