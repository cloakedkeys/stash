//go:build integration

package database_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storyFile(ctx context.Context, t *testing.T, name string) *models.BaseFile {
	t.Helper()
	f := &models.BaseFile{
		Basename:       name,
		ParentFolderID: folderIDs[folderIdxWithAudioFiles],
		Size:           12,
		DirEntry:       models.DirEntry{ModTime: time.Now()},
	}
	require.NoError(t, db.File().Create(ctx, f))
	return f
}

func TestStoryStore(t *testing.T) {
	runWithRollbackTxn(t, "metadata, relationships and primary files", func(t *testing.T, ctx context.Context) {
		store := db.Story()
		first := storyFile(ctx, t, "story-first.txt")
		second := storyFile(ctx, t, "story-second.txt")
		rating := 83
		date := models.Date{Time: time.Date(2020, 2, 3, 0, 0, 0, 0, time.UTC)}
		s := models.NewStory()
		s.Title = "A story"
		s.Author = "An author"
		s.SourceWebsite = "A website"
		s.Date = &date
		s.Language = "en"
		s.Details = "Details"
		s.Tagline = "Tagline"
		s.Rating = &rating
		s.URLs = models.NewRelatedStrings([]string{"https://one.example/", "https://two.example/"})
		s.TagIDs = models.NewRelatedIDs([]int{tagIDs[tagIdx1WithNothing]})
		want := s
		require.NoError(t, store.Create(ctx, &s, []models.FileID{first.ID, second.ID}))
		require.NotZero(t, s.ID)
		require.Equal(t, &first.ID, s.PrimaryFileID)
		require.NoError(t, s.LoadRelationships(ctx, store))
		assert.Equal(t, want.Title, s.Title)
		assert.Equal(t, want.Author, s.Author)
		assert.Equal(t, want.SourceWebsite, s.SourceWebsite)
		assert.Equal(t, want.Language, s.Language)
		assert.Equal(t, want.Details, s.Details)
		assert.Equal(t, want.Tagline, s.Tagline)
		assert.Equal(t, want.Date, s.Date)
		assert.WithinDuration(t, want.CreatedAt, s.CreatedAt, time.Second)
		assert.WithinDuration(t, want.UpdatedAt, s.UpdatedAt, time.Second)
		assert.Equal(t, want.Rating, s.Rating)
		assert.Equal(t, want.URLs.List(), s.URLs.List())
		assert.Equal(t, want.TagIDs.List(), s.TagIDs.List())
		files, err := store.GetFiles(ctx, s.ID)
		require.NoError(t, err)
		require.Len(t, files, 2)
		ids, err := store.GetManyFileIDs(ctx, []int{s.ID})
		require.NoError(t, err)
		assert.Equal(t, [][]models.FileID{{first.ID, second.ID}}, ids)
		found, err := store.FindByFileID(ctx, second.ID)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, s.ID, found[0].ID)
		count, err := store.CountByFileID(ctx, first.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		byFiles, err := store.GetManyIDsByFileIDs(ctx, []models.FileID{second.ID, first.ID})
		require.NoError(t, err)
		assert.Equal(t, [][]int{{s.ID}, {s.ID}}, byFiles)

		changed, err := store.UpdatePartial(ctx, s.ID, models.StoryPartial{
			Title:         models.NewOptionalString("Changed"),
			PrimaryFileID: &second.ID,
			Rating:        models.NewOptionalInt(47),
			URLs:          &models.UpdateStrings{Values: []string{"https://three.example/"}, Mode: models.RelationshipUpdateModeAdd},
			TagIDs:        &models.UpdateIDs{IDs: []int{tagIDs[tagIdx1WithAudio]}, Mode: models.RelationshipUpdateModeAdd},
		})
		require.NoError(t, err)
		assert.Equal(t, "Changed", changed.Title)
		assert.Equal(t, &second.ID, changed.PrimaryFileID)
		require.NoError(t, changed.LoadRelationships(ctx, store))
		assert.Len(t, changed.URLs.List(), 3)
		assert.Len(t, changed.TagIDs.List(), 2)
		changed, err = store.UpdatePartial(ctx, s.ID, models.StoryPartial{
			URLs:   &models.UpdateStrings{Values: []string{"https://three.example/"}, Mode: models.RelationshipUpdateModeRemove},
			TagIDs: &models.UpdateIDs{IDs: []int{tagIDs[tagIdx1WithAudio]}, Mode: models.RelationshipUpdateModeRemove},
		})
		require.NoError(t, err)
		require.NoError(t, changed.LoadRelationships(ctx, store))
		assert.Len(t, changed.URLs.List(), 2)
		assert.Len(t, changed.TagIDs.List(), 1)
		changed.Author = "Changed author"
		changed.URLs = models.NewRelatedStrings([]string{})
		changed.TagIDs = models.NewRelatedIDs([]int{})
		require.NoError(t, store.Update(ctx, changed))
		got, err := store.Find(ctx, s.ID)
		require.NoError(t, err)
		require.NoError(t, got.LoadRelationships(ctx, store))
		assert.Equal(t, "Changed author", got.Author)
		assert.Empty(t, got.URLs.List())
		assert.Empty(t, got.TagIDs.List())
		assert.Equal(t, ptr(47), got.Rating)
		primary, err := store.FindByPrimaryFileID(ctx, second.ID)
		require.NoError(t, err)
		require.Len(t, primary, 1)
		assert.Equal(t, s.ID, primary[0].ID)
		cleared, err := store.UpdatePartial(ctx, s.ID, models.StoryPartial{
			Date:   models.OptionalDate{Set: true, Null: true},
			Rating: models.OptionalInt{Set: true, Null: true},
			Author: models.OptionalString{Set: true, Null: true},
		})
		require.NoError(t, err)
		assert.Nil(t, cleared.Date)
		assert.Nil(t, cleared.Rating)
		assert.Empty(t, cleared.Author)
		missing, err := store.Find(ctx, invalidID)
		require.NoError(t, err)
		assert.Nil(t, missing)
		_, err = store.FindMany(ctx, []int{invalidID})
		assert.Error(t, err)
	})
}

func TestStoryDestroyKeepsFiles(t *testing.T) {
	runWithRollbackTxn(t, "keeps source files and shared file records", func(t *testing.T, ctx context.Context) {
		path := filepath.Join(t.TempDir(), "story.txt")
		content := []byte("The source story.")
		require.NoError(t, os.WriteFile(path, content, 0600))
		folder := &models.Folder{Path: filepath.Dir(path)}
		require.NoError(t, db.Folder().Create(ctx, folder))
		f := &models.BaseFile{Basename: filepath.Base(path), ParentFolderID: folder.ID, Size: int64(len(content)), DirEntry: models.DirEntry{ModTime: time.Now()}}
		require.NoError(t, db.File().Create(ctx, f))
		unshared := storyFile(ctx, t, "unshared-story.txt")
		shared := models.NewAudio()
		require.NoError(t, db.Audio().Create(ctx, &shared, []models.FileID{f.ID}))
		s := models.NewStory()
		s.Rating = ptr(90)
		s.URLs = models.NewRelatedStrings([]string{"https://story.example/"})
		s.TagIDs = models.NewRelatedIDs([]int{tagIDs[tagIdx1WithNothing]})
		before, err := db.Story().Count(ctx)
		require.NoError(t, err)
		require.NoError(t, db.Story().Create(ctx, &s, []models.FileID{f.ID, unshared.ID}))
		require.NoError(t, db.Story().Destroy(ctx, s.ID))
		after, err := db.Story().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, after)
		got, err := db.Story().Find(ctx, s.ID)
		require.NoError(t, err)
		assert.Nil(t, got)
		files, err := db.File().Find(ctx, f.ID, unshared.ID)
		require.NoError(t, err)
		assert.Len(t, files, 2, "both shared and unshared file records are retained")
		linked, err := db.Audio().GetManyFileIDs(ctx, []int{shared.ID})
		require.NoError(t, err)
		assert.Equal(t, [][]models.FileID{{f.ID}}, linked)
		urls, err := db.Story().GetURLs(ctx, s.ID)
		require.NoError(t, err)
		assert.Empty(t, urls)
		tags, err := db.Story().GetTagIDs(ctx, s.ID)
		require.NoError(t, err)
		assert.Empty(t, tags)
		storyFiles, err := db.Story().GetManyFileIDs(ctx, []int{s.ID})
		require.NoError(t, err)
		assert.Empty(t, storyFiles[0])
		onDisk, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, onDisk)
	})
}

func TestStoryQuery(t *testing.T) {
	runWithRollbackTxn(t, "filtering, pagination and per-user ratings", func(t *testing.T, ctx context.Context) {
		store := db.Story()
		tag := &models.Tag{Name: "story-query-tag"}
		require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: tag}))
		var ids []int
		for _, title := range []string{"Story query A", "Story query B", "Other title"} {
			s := models.NewStory()
			s.Title = title
			s.Author = "Searchable author"
			s.Rating = ptr(80)
			s.TagIDs = models.NewRelatedIDs([]int{tag.ID})
			require.NoError(t, store.Create(ctx, &s, nil))
			ids = append(ids, s.ID)
		}
		filter := &models.StoryFilterType{
			Title:     &models.StringCriterionInput{Value: "Story query", Modifier: models.CriterionModifierIncludes},
			Tags:      &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(tag.ID)}, Modifier: models.CriterionModifierIncludes},
			Rating100: &models.IntCriterionInput{Value: 75, Modifier: models.CriterionModifierGreaterThan},
		}
		result, err := store.Query(ctx, models.StoryQueryOptions{QueryOptions: models.QueryOptions{Count: true, FindFilter: &models.FindFilterType{Sort: ptr("title"), PerPage: ptr(1), Page: ptr(2)}}, StoryFilter: filter})
		require.NoError(t, err)
		assert.Equal(t, 2, result.Count)
		resolved, err := result.Resolve(ctx)
		require.NoError(t, err)
		require.Len(t, resolved, 1)
		assert.Equal(t, ids[1], resolved[0].ID)
		count, err := store.QueryCount(ctx, filter, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		count, err = store.QueryCount(ctx, nil, &models.FindFilterType{Q: ptr("Searchable")})
		require.NoError(t, err)
		assert.Equal(t, 3, count)
		user := &models.User{Username: "story-rater"}
		require.NoError(t, db.User().Create(ctx, user, ""))
		otherCtx := models.WithUserID(ctx, user.ID)
		found, err := store.Find(otherCtx, ids[0])
		require.NoError(t, err)
		assert.Nil(t, found.Rating)
		_, err = store.UpdatePartial(otherCtx, ids[0], models.StoryPartial{Rating: models.NewOptionalInt(20)})
		require.NoError(t, err)
		own, err := store.Find(ctx, ids[0])
		require.NoError(t, err)
		assert.Equal(t, ptr(80), own.Rating)
		_, err = store.UpdatePartial(ctx, ids[0], models.StoryPartial{Rating: models.NewOptionalInt(70)})
		require.NoError(t, err)
		sorted, err := store.Query(ctx, models.StoryQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: &models.FindFilterType{Sort: ptr("rating")}},
			StoryFilter:  &models.StoryFilterType{Tags: filter.Tags},
		})
		require.NoError(t, err)
		assert.Equal(t, ids[0], sorted.IDs[0])
		count, err = store.QueryCount(otherCtx, filter, nil)
		require.NoError(t, err)
		assert.Zero(t, count)
		_, err = store.Query(ctx, models.StoryQueryOptions{QueryOptions: models.QueryOptions{FindFilter: &models.FindFilterType{Sort: ptr("invalid")}}})
		assert.Error(t, err)
	})
}

func TestStoryTagsAndLibraryLimits(t *testing.T) {
	runWithRollbackTxn(t, "tag merges, deletion and limited reads", func(t *testing.T, ctx context.Context) {
		source := &models.Tag{Name: "story-source-tag"}
		target := &models.Tag{Name: "story-target-tag"}
		for _, tag := range []*models.Tag{source, target} {
			require.NoError(t, db.Tag().Create(ctx, &models.CreateTagInput{Tag: tag}))
		}
		s := models.NewStory()
		s.TagIDs = models.NewRelatedIDs([]int{source.ID, target.ID})
		f := storyFile(ctx, t, "limited-story.txt")
		require.NoError(t, db.Story().Create(ctx, &s, []models.FileID{f.ID}))
		primary, err := db.File().IsPrimary(ctx, f.ID)
		require.NoError(t, err)
		assert.True(t, primary)
		require.NoError(t, db.Tag().Merge(ctx, []int{source.ID}, target.ID))
		tags, err := db.Story().GetTagIDs(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, []int{target.ID}, tags)
		for _, test := range []struct {
			name    string
			limit   *models.LibraryLimit
			visible bool
		}{
			{"tag", &models.LibraryLimit{TagIDs: []int{target.ID}}, true},
			{"folder", &models.LibraryLimit{FolderIDs: []int{int(f.ParentFolderID)}}, true},
			{"hidden", &models.LibraryLimit{TagIDs: []int{invalidID}}, false},
			{"studio cannot grant story access", &models.LibraryLimit{StudioIDs: []int{studioIDs[studioIdxWithAudio]}}, false},
		} {
			t.Run(test.name, func(t *testing.T) {
				limited := models.WithLibraryLimit(ctx, test.limit)
				got, err := db.Story().Find(limited, s.ID)
				require.NoError(t, err)
				assert.Equal(t, test.visible, got != nil)
				many, err := db.Story().FindMany(limited, []int{s.ID})
				require.NoError(t, err)
				assert.Equal(t, test.visible, len(many) > 0)
				byFile, err := db.Story().FindByFileID(limited, f.ID)
				require.NoError(t, err)
				assert.Equal(t, test.visible, len(byFile) > 0)
				count, err := db.Story().QueryCount(limited, nil, nil)
				require.NoError(t, err)
				if test.visible {
					assert.Equal(t, 1, count)
				} else {
					assert.Zero(t, count)
				}
			})
		}
		require.NoError(t, db.Tag().Destroy(ctx, target.ID))
		tags, err = db.Story().GetTagIDs(ctx, s.ID)
		require.NoError(t, err)
		assert.Empty(t, tags)
	})
}

func TestStorySharedFileLookup(t *testing.T) {
	runWithRollbackTxn(t, "find every story sharing a file", func(t *testing.T, ctx context.Context) {
		f := storyFile(ctx, t, "shared-stories.txt")
		var ids []int
		for i := 0; i < 2; i++ {
			s := models.NewStory()
			require.NoError(t, db.Story().Create(ctx, &s, []models.FileID{f.ID}))
			ids = append(ids, s.ID)
		}
		found, err := db.Story().FindByFileID(ctx, f.ID)
		require.NoError(t, err)
		var got []int
		for _, s := range found {
			got = append(got, s.ID)
		}
		assert.ElementsMatch(t, ids, got)
	})
}
