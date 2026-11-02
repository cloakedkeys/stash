package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil/stringslice"
	"github.com/stashapp/stash/pkg/story"
	"github.com/stashapp/stash/pkg/utils"
)

// errStoryTextWithFile refuses text for a story with a text file, which always shows
// the file's text.
var errStoryTextWithFile = errors.New("a story with a text file shows the file's text, so it can't be given text as well")

// storyText is the text to store from an input: nil when it isn't given, and empty
// when it's blank, which removes it.
func storyText(text *string) *string {
	if text == nil {
		return nil
	}
	if strings.TrimSpace(*text) == "" {
		empty := ""
		return &empty
	}
	return text
}

func (r *mutationResolver) getStory(ctx context.Context, id int) (ret *models.Story, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.Story.Find(ctx, id)
		return err
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

// checkStoryFile refuses a file that isn't a plain file, or that already belongs to something.
func (r *mutationResolver) checkStoryFile(ctx context.Context, fileID models.FileID) error {
	files, err := r.repository.File.Find(ctx, fileID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("file with id %d not found", fileID)
	}

	// videos, audio and images have their own objects
	f := files[0]
	if _, ok := f.(*models.BaseFile); !ok || !manager.IsStoryFile(f.Base().Path) {
		return fmt.Errorf("%s is not a text file", f.Base().Path)
	}
	// a clean would remove a story whose file is in a library that doesn't include stories
	if !manager.UseAsStory(f.Base().Path) {
		return fmt.Errorf("%s is in a library that doesn't include stories", f.Base().Path)
	}

	isPrimary, err := r.repository.File.IsPrimary(ctx, fileID)
	if err != nil {
		return err
	}
	if isPrimary {
		return fmt.Errorf("%s already belongs to something else", f.Base().Path)
	}

	count, err := r.repository.Story.CountByFileID(ctx, fileID)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%s already belongs to another story", f.Base().Path)
	}

	return nil
}

func (r *mutationResolver) StoryCreate(ctx context.Context, input StoryCreateInput) (ret *models.Story, err error) {
	translator := changesetTranslator{
		inputMap: getUpdateInputMap(ctx),
	}

	fileIDs, err := translator.fileIDSliceFromStringSlice(input.FileIds)
	if err != nil {
		return nil, fmt.Errorf("converting file ids: %w", err)
	}

	newStory := models.NewStory()
	newStory.Title = translator.string(input.Title)
	newStory.Author = translator.string(input.Author)
	newStory.SourceWebsite = translator.string(input.SourceWebsite)
	newStory.Language = translator.string(input.Language)
	newStory.Details = translator.string(input.Details)
	newStory.Tagline = translator.string(input.Tagline)
	newStory.Rating = input.Rating100

	newStory.Date, err = translator.datePtr(input.Date)
	if err != nil {
		return nil, fmt.Errorf("converting date: %w", err)
	}

	newStory.URLs = models.NewRelatedStrings(stringslice.TrimSpace(input.Urls))

	newStory.TagIDs, err = translator.relatedIds(input.TagIds)
	if err != nil {
		return nil, fmt.Errorf("converting tag ids: %w", err)
	}

	if newStory.Title == "" && len(fileIDs) == 0 {
		return nil, errors.New("title must be set if story has no files")
	}

	text := storyText(input.Text)
	if text != nil && *text != "" && len(fileIDs) > 0 {
		return nil, errStoryTextWithFile
	}

	var id int
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		seen := make(map[models.FileID]bool)
		for _, fileID := range fileIDs {
			if seen[fileID] {
				return fmt.Errorf("file with id %d is listed twice", fileID)
			}
			seen[fileID] = true

			if err := r.checkStoryFile(ctx, fileID); err != nil {
				return err
			}
		}

		if err := r.repository.Story.Create(ctx, &newStory, fileIDs); err != nil {
			return err
		}
		id = newStory.ID

		if text != nil && *text != "" {
			return r.repository.Story.UpdateText(ctx, id, *text)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return r.getStory(ctx, id)
}

func storyPartialFromInput(input StoryUpdateInput, translator changesetTranslator) (*models.StoryPartial, error) {
	updated := models.NewStoryPartial()

	updated.Title = translator.optionalString(input.Title, "title")
	updated.Author = translator.optionalString(input.Author, "author")
	updated.SourceWebsite = translator.optionalString(input.SourceWebsite, "source_website")
	updated.Language = translator.optionalString(input.Language, "language")
	updated.Details = translator.optionalString(input.Details, "details")
	updated.Tagline = translator.optionalString(input.Tagline, "tagline")
	updated.Rating = translator.optionalInt(input.Rating100, "rating100")
	updated.URLs = translator.optionalURLs(input.Urls, nil)

	var err error

	updated.Date, err = translator.optionalDate(input.Date, "date")
	if err != nil {
		return nil, fmt.Errorf("converting date: %w", err)
	}

	updated.TagIDs, err = translator.updateIds(input.TagIds, "tag_ids")
	if err != nil {
		return nil, fmt.Errorf("converting tag ids: %w", err)
	}

	updated.PrimaryFileID, err = translator.fileIDPtrFromString(input.PrimaryFileID)
	if err != nil {
		return nil, fmt.Errorf("converting primary file id: %w", err)
	}

	return &updated, nil
}

func (r *mutationResolver) StoryUpdate(ctx context.Context, input StoryUpdateInput) (ret *models.Story, err error) {
	translator := changesetTranslator{
		inputMap: getUpdateInputMap(ctx),
	}

	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	updated, err := storyPartialFromInput(input, translator)
	if err != nil {
		return nil, err
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.Story

		original, err := qb.Find(ctx, id)
		if err != nil {
			return err
		}
		if original == nil {
			return fmt.Errorf("story with id %d not found", id)
		}

		text := storyText(input.Text)
		needFiles := updated.PrimaryFileID != nil || (updated.Title.Set && updated.Title.Value == "") || text != nil
		if needFiles {
			if err := original.LoadFiles(ctx, qb); err != nil {
				return err
			}
		}

		if updated.Title.Set && updated.Title.Value == "" && len(original.Files.List()) == 0 {
			return errors.New("title must be set if story has no files")
		}

		if updated.PrimaryFileID != nil {
			found := false
			for _, f := range original.Files.List() {
				if f.Base().ID == *updated.PrimaryFileID {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("file with id %d not associated with story", *updated.PrimaryFileID)
			}
		}

		if text != nil {
			if *text != "" && len(original.Files.List()) > 0 {
				return errStoryTextWithFile
			}
			if err := qb.UpdateText(ctx, id, *text); err != nil {
				return err
			}
		}

		_, err = qb.UpdatePartial(ctx, id, *updated)
		return err
	}); err != nil {
		return nil, err
	}

	return r.getStory(ctx, id)
}

func (r *mutationResolver) StoriesDestroy(ctx context.Context, input StoriesDestroyInput) (bool, error) {
	ids, err := stringslice.StringSliceToIntSlice(input.Ids)
	if err != nil {
		return false, fmt.Errorf("converting ids: %w", err)
	}

	// files are only deleted from disk once the database changes are saved
	deleteFile := utils.IsTrue(input.DeleteFile)
	fileDeleter := file.NewDeleter()
	if deleteFile {
		fileDeleter = file.NewDeleterWithTrash(manager.GetInstance().Config.GetDeleteTrashPath())
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.Story
		// a file is kept while anything else uses it
		others := []story.FileUser{r.repository.Scene, r.repository.Image, r.repository.Gallery, r.repository.Audio}

		for _, id := range ids {
			s, err := qb.Find(ctx, id)
			if err != nil {
				return err
			}
			if s == nil {
				return fmt.Errorf("story with id %d not found", id)
			}

			// without deleteFile, its files and their records stay
			if err := story.Destroy(ctx, qb, r.repository.File, others, s, fileDeleter, deleteFile); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		fileDeleter.Rollback()
		return false, err
	}

	fileDeleter.Commit()

	return true, nil
}

func (r *mutationResolver) StoryUserDataUpdate(ctx context.Context, input UserRatingInput) (*models.Story, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.StoryPartial{Rating: translator.optionalInt(input.Rating100, "rating100")}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		story, err := r.repository.Story.Find(ctx, id)
		if err != nil {
			return err
		}
		if story == nil {
			return fmt.Errorf("story with id %d not found", id)
		}

		_, err = r.repository.Story.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	return r.getStory(ctx, id)
}
