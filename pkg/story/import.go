package story

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stashapp/stash/pkg/sliceutil"
)

type ImporterReaderWriter interface {
	Create(ctx context.Context, newStory *models.Story, fileIDs []models.FileID) error
	Update(ctx context.Context, updatedStory *models.Story) error
	FindByFileID(ctx context.Context, fileID models.FileID) ([]*models.Story, error)
	Query(ctx context.Context, options models.StoryQueryOptions) (*models.StoryQueryResult, error)
}

type Importer struct {
	ReaderWriter        ImporterReaderWriter
	TagWriter           models.TagFinderCreator
	FileFinder          models.FileFinder
	Input               jsonschema.Story
	MissingRefBehaviour models.ImportMissingRefEnum

	// Claimed holds the stories this import has already matched by title or
	// made, shared by the importers of one import. Two stories with no file
	// and the same title then each get their own story.
	Claimed map[int]bool

	story models.Story
}

func (i *Importer) claim(id int) {
	if i.Claimed != nil {
		i.Claimed[id] = true
	}
}

func (i *Importer) PreImport(ctx context.Context) error {
	i.story = i.storyJSONToStory(i.Input)
	if err := i.populateFiles(ctx); err != nil {
		return err
	}
	return i.populateTags(ctx)
}

func (i *Importer) storyJSONToStory(in jsonschema.Story) models.Story {
	ret := models.Story{
		Title:         in.Title,
		Author:        in.Author,
		SourceWebsite: in.SourceWebsite,
		Language:      in.Language,
		Details:       in.Details,
		Tagline:       in.Tagline,
		URLs:          models.NewRelatedStrings(in.URLs),
		TagIDs:        models.NewRelatedIDs([]int{}),
		CreatedAt:     in.CreatedAt.GetTime(),
		UpdatedAt:     in.UpdatedAt.GetTime(),
	}
	if in.URLs == nil {
		ret.URLs = models.NewRelatedStrings([]string{})
	}
	if in.Date != "" {
		if d, err := models.ParseDate(in.Date); err == nil {
			ret.Date = &d
		}
	}
	if in.Rating != 0 {
		rating := in.Rating
		ret.Rating = &rating
	}
	return ret
}

func (i *Importer) populateFiles(ctx context.Context) error {
	files := make([]models.File, 0, len(i.Input.Files))
	for _, path := range i.Input.Files {
		f, err := i.FileFinder.FindByPath(ctx, path, true)
		if err != nil {
			return fmt.Errorf("error finding file: %w", err)
		}
		if f == nil {
			return fmt.Errorf("story file '%s' not found", path)
		}
		files = append(files, f)
	}
	i.story.Files = models.NewRelatedFiles(files)
	return nil
}

func (i *Importer) populateTags(ctx context.Context) error {
	names := i.Input.Tags
	if len(names) == 0 {
		return nil
	}

	tags, err := i.TagWriter.FindByNames(ctx, names, false)
	if err != nil {
		return err
	}

	var found []string
	for _, t := range tags {
		found = append(found, t.Name)
	}
	missing := sliceutil.Filter(names, func(name string) bool {
		return !slices.Contains(found, name)
	})

	if len(missing) > 0 {
		switch i.MissingRefBehaviour {
		case models.ImportMissingRefEnumFail:
			return fmt.Errorf("story tags [%s] not found", strings.Join(missing, ", "))
		case models.ImportMissingRefEnumCreate:
			for _, name := range missing {
				t := models.NewTag()
				t.Name = name
				if err := i.TagWriter.Create(ctx, &models.CreateTagInput{Tag: &t}); err != nil {
					return fmt.Errorf("error creating story tag: %w", err)
				}
				tags = append(tags, &t)
			}
		}
		// ignored otherwise
	}

	for _, t := range tags {
		i.story.TagIDs.Add(t.ID)
	}
	return nil
}

func (i *Importer) PostImport(ctx context.Context, id int) error {
	return nil
}

func (i *Importer) Name() string {
	if i.Input.Title != "" {
		return i.Input.Title
	}
	if len(i.Input.Files) > 0 {
		return i.Input.Files[0]
	}
	return ""
}

// FindExistingID finds the story with any of the same files, or for a story with no
// files, one with no files and the same title.
func (i *Importer) FindExistingID(ctx context.Context) (*int, error) {
	files := i.story.Files.List()
	for _, f := range files {
		existing, err := i.ReaderWriter.FindByFileID(ctx, f.Base().ID)
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			id := existing[0].ID
			return &id, nil
		}
	}
	if len(files) > 0 || i.story.Title == "" {
		return nil, nil
	}

	all := -1
	result, err := i.ReaderWriter.Query(ctx, models.StoryQueryOptions{
		QueryOptions: models.QueryOptions{FindFilter: &models.FindFilterType{PerPage: &all}},
		StoryFilter: &models.StoryFilterType{
			Title: &models.StringCriterionInput{Value: i.story.Title, Modifier: models.CriterionModifierEquals},
		},
	})
	if err != nil {
		return nil, err
	}
	stories, err := result.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range stories {
		if s.PrimaryFileID == nil && s.Title == i.story.Title && !i.Claimed[s.ID] {
			id := s.ID
			i.claim(id)
			return &id, nil
		}
	}
	return nil, nil
}

func (i *Importer) Create(ctx context.Context) (*int, error) {
	var fileIDs []models.FileID
	for _, f := range i.story.Files.List() {
		fileIDs = append(fileIDs, f.Base().ID)
	}
	if err := i.ReaderWriter.Create(ctx, &i.story, fileIDs); err != nil {
		return nil, fmt.Errorf("error creating story: %w", err)
	}
	id := i.story.ID
	i.claim(id)
	return &id, nil
}

func (i *Importer) Update(ctx context.Context, id int) error {
	s := i.story
	s.ID = id
	if err := i.ReaderWriter.Update(ctx, &s); err != nil {
		return fmt.Errorf("error updating existing story: %w", err)
	}
	return nil
}
