package story

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/json"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	created = time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC)
	updated = time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC)
)

func TestToBasicJSON(t *testing.T) {
	rating := 80
	date := models.Date{Time: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)}
	primary := models.FileID(2)
	s := &models.Story{
		Title: "A story", Author: "Someone", SourceWebsite: "example.com", Language: "en",
		Details: "Details", Tagline: "Tagline", Rating: &rating, Date: &date,
		URLs:          models.NewRelatedStrings([]string{"https://one.example/"}),
		PrimaryFileID: &primary,
		// the primary file isn't first in the list
		Files: models.NewRelatedFiles([]models.File{
			&models.BaseFile{ID: 1, Path: "/stories/second.txt"},
			&models.BaseFile{ID: 2, Path: "/stories/first.txt"},
		}),
		CreatedAt: created, UpdatedAt: updated,
	}

	assert.Equal(t, &jsonschema.Story{
		Title: "A story", Author: "Someone", SourceWebsite: "example.com", Language: "en",
		Details: "Details", Tagline: "Tagline", Rating: 80, Date: "2020-01-02",
		URLs:      []string{"https://one.example/"},
		Files:     []string{"/stories/first.txt", "/stories/second.txt"},
		CreatedAt: json.JSONTime{Time: created}, UpdatedAt: json.JSONTime{Time: updated},
	}, ToBasicJSON(s))

	// a story with only a title
	bare := &models.Story{Title: "Only a title", URLs: models.NewRelatedStrings([]string{}), Files: models.NewRelatedFiles([]models.File{})}
	j := ToBasicJSON(bare)
	assert.Empty(t, j.Files)
	assert.Zero(t, j.Rating)
	assert.Empty(t, j.Date)
}

func newImporter(db *mocks.Database, in jsonschema.Story, missing models.ImportMissingRefEnum) *Importer {
	return &Importer{ReaderWriter: db.Story, TagWriter: db.Tag, FileFinder: db.File, Input: in, MissingRefBehaviour: missing}
}

func TestImporterPreImport(t *testing.T) {
	ctx := context.Background()
	first := &models.BaseFile{ID: 2, Path: "/stories/first.txt"}
	second := &models.BaseFile{ID: 1, Path: "/stories/second.txt"}

	t.Run("fields, files in order and tags", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.File.On("FindByPath", ctx, first.Path, true).Return(first, nil)
		db.File.On("FindByPath", ctx, second.Path, true).Return(second, nil)
		db.Tag.On("FindByNames", ctx, []string{"kept"}, false).Return([]*models.Tag{{ID: 7, Name: "kept"}}, nil)

		i := newImporter(db, jsonschema.Story{
			Title: "A story", Author: "Someone", Rating: 80, Date: "2020-01-02",
			URLs: []string{"https://one.example/"}, Files: []string{first.Path, second.Path}, Tags: []string{"kept"},
			CreatedAt: json.JSONTime{Time: created}, UpdatedAt: json.JSONTime{Time: updated},
		}, models.ImportMissingRefEnumFail)
		require.NoError(t, i.PreImport(ctx))

		assert.Equal(t, "A story", i.story.Title)
		assert.Equal(t, "Someone", i.story.Author)
		assert.Equal(t, 80, *i.story.Rating)
		assert.Equal(t, "2020-01-02", i.story.Date.String())
		assert.Equal(t, []string{"https://one.example/"}, i.story.URLs.List())
		assert.Equal(t, []int{7}, i.story.TagIDs.List())
		assert.True(t, created.Equal(i.story.CreatedAt), "created at %v", i.story.CreatedAt)
		assert.True(t, updated.Equal(i.story.UpdatedAt), "updated at %v", i.story.UpdatedAt)
		require.Len(t, i.story.Files.List(), 2)
		assert.Equal(t, first.ID, i.story.Files.List()[0].Base().ID)

		db.Story.On("Create", ctx, mock.AnythingOfType("*models.Story"), []models.FileID{first.ID, second.ID}).Run(func(args mock.Arguments) {
			args.Get(1).(*models.Story).ID = 5
		}).Return(nil)
		id, err := i.Create(ctx)
		require.NoError(t, err)
		assert.Equal(t, 5, *id)
	})

	t.Run("a missing file fails", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.File.On("FindByPath", ctx, "/gone.txt", true).Return(nil, nil)
		err := newImporter(db, jsonschema.Story{Files: []string{"/gone.txt"}}, models.ImportMissingRefEnumCreate).PreImport(ctx)
		assert.ErrorContains(t, err, "story file '/gone.txt' not found")
	})

	t.Run("missing tags", func(t *testing.T) {
		in := jsonschema.Story{Title: "t", Tags: []string{"kept", "new"}}
		found := []*models.Tag{{ID: 7, Name: "kept"}}

		db := mocks.NewDatabase()
		db.Tag.On("FindByNames", ctx, in.Tags, false).Return(found, nil)
		assert.ErrorContains(t, newImporter(db, in, models.ImportMissingRefEnumFail).PreImport(ctx), "story tags [new] not found")

		db = mocks.NewDatabase()
		db.Tag.On("FindByNames", ctx, in.Tags, false).Return(found, nil)
		i := newImporter(db, in, models.ImportMissingRefEnumIgnore)
		require.NoError(t, i.PreImport(ctx))
		assert.Equal(t, []int{7}, i.story.TagIDs.List())

		db = mocks.NewDatabase()
		db.Tag.On("FindByNames", ctx, in.Tags, false).Return(found, nil)
		db.Tag.On("Create", ctx, mock.MatchedBy(func(c *models.CreateTagInput) bool { return c.Name == "new" })).Run(func(args mock.Arguments) {
			args.Get(1).(*models.CreateTagInput).ID = 8
		}).Return(nil)
		i = newImporter(db, in, models.ImportMissingRefEnumCreate)
		require.NoError(t, i.PreImport(ctx))
		assert.Equal(t, []int{7, 8}, i.story.TagIDs.List())

		db = mocks.NewDatabase()
		db.Tag.On("FindByNames", ctx, in.Tags, false).Return(found, nil)
		db.Tag.On("Create", ctx, mock.Anything).Return(errors.New("no"))
		assert.ErrorContains(t, newImporter(db, in, models.ImportMissingRefEnumCreate).PreImport(ctx), "error creating story tag")
	})
}

func TestImporterFindExistingID(t *testing.T) {
	ctx := context.Background()
	f := &models.BaseFile{ID: 3, Path: "/stories/tale.txt"}

	t.Run("by file", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.File.On("FindByPath", ctx, f.Path, true).Return(f, nil)
		db.Story.On("FindByFileID", ctx, f.ID).Return([]*models.Story{{ID: 9}}, nil)
		i := newImporter(db, jsonschema.Story{Title: "t", Files: []string{f.Path}}, models.ImportMissingRefEnumFail)
		require.NoError(t, i.PreImport(ctx))
		id, err := i.FindExistingID(ctx)
		require.NoError(t, err)
		assert.Equal(t, 9, *id)
		db.Story.AssertNotCalled(t, "Query", mock.Anything, mock.Anything)
	})

	t.Run("a file with no story is new, even if a story has the title", func(t *testing.T) {
		db := mocks.NewDatabase()
		db.File.On("FindByPath", ctx, f.Path, true).Return(f, nil)
		db.Story.On("FindByFileID", ctx, f.ID).Return(nil, nil)
		i := newImporter(db, jsonschema.Story{Title: "t", Files: []string{f.Path}}, models.ImportMissingRefEnumFail)
		require.NoError(t, i.PreImport(ctx))
		id, err := i.FindExistingID(ctx)
		require.NoError(t, err)
		assert.Nil(t, id)
		db.Story.AssertNotCalled(t, "Query", mock.Anything, mock.Anything)
	})

	t.Run("a story with no files, by title", func(t *testing.T) {
		withFile := models.FileID(1)
		for _, c := range []struct {
			name    string
			stories []*models.Story
			want    *int
		}{
			{"same title, no files", []*models.Story{{ID: 4, Title: "Only a title"}}, intPtr(4)},
			{"same title but it has a file", []*models.Story{{ID: 4, Title: "Only a title", PrimaryFileID: &withFile}}, nil},
			{"title differs in case", []*models.Story{{ID: 4, Title: "only a title"}}, nil},
			{"none", nil, nil},
		} {
			t.Run(c.name, func(t *testing.T) {
				db := mocks.NewDatabase()
				ids := make([]int, len(c.stories))
				for n, s := range c.stories {
					ids[n] = s.ID
				}
				result := models.NewStoryQueryResult(db.Story)
				result.IDs = ids
				db.Story.On("Query", ctx, mock.MatchedBy(func(o models.StoryQueryOptions) bool {
					return o.StoryFilter.Title.Value == "Only a title" && o.StoryFilter.Title.Modifier == models.CriterionModifierEquals && *o.FindFilter.PerPage == -1
				})).Return(result, nil)
				db.Story.On("FindMany", ctx, ids).Return(c.stories, nil)

				i := newImporter(db, jsonschema.Story{Title: "Only a title"}, models.ImportMissingRefEnumFail)
				require.NoError(t, i.PreImport(ctx))
				id, err := i.FindExistingID(ctx)
				require.NoError(t, err)
				assert.Equal(t, c.want, id)
			})
		}
	})
}

func TestImporterUpdate(t *testing.T) {
	ctx := context.Background()
	db := mocks.NewDatabase()
	db.Story.On("Update", ctx, mock.MatchedBy(func(s *models.Story) bool { return s.ID == 9 && s.Title == "t" })).Return(nil)
	i := newImporter(db, jsonschema.Story{Title: "t"}, models.ImportMissingRefEnumFail)
	require.NoError(t, i.PreImport(ctx))
	require.NoError(t, i.Update(ctx, 9))
	db.Story.AssertExpectations(t)
}

func intPtr(v int) *int { return &v }
