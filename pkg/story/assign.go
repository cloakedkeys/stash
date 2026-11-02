package story

import (
	"context"
	"errors"
	"fmt"

	"github.com/stashapp/stash/pkg/models"
)

// AssignRepository is what AssignFile needs to move a file between stories.
type AssignRepository interface {
	Find(ctx context.Context, id int) (*models.Story, error)
	FindByFileID(ctx context.Context, id models.FileID) ([]*models.Story, error)
	GetFiles(ctx context.Context, id int) ([]models.File, error)
	AssignFiles(ctx context.Context, id int, fileIDs []models.FileID) error
	UpdatePartial(ctx context.Context, id int, partial models.StoryPartial) (*models.Story, error)
	UpdateText(ctx context.Context, id int, text string) error
	Destroy(ctx context.Context, id int) error
}

// ErrFileUsedElsewhere is returned for a file that a scene, image, gallery or audio uses.
var ErrFileUsedElsewhere = errors.New("the file belongs to something that isn't a story")

// AssignFile moves a text file onto a story, such as one scraped from a website, from
// the story a scan made for it. The file's text is then shown, so any text stored with
// the story is removed. A story left with no file and no title, as a scan makes, is
// removed too. One with a title keeps its details without the file.
func AssignFile(ctx context.Context, r AssignRepository, others []FileUser, storyID int, fileID models.FileID) error {
	s, err := r.Find(ctx, storyID)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("story with id %d not found", storyID)
	}

	for _, o := range others {
		count, err := o.CountByFileID(ctx, fileID)
		if err != nil {
			return err
		}
		if count > 0 {
			return ErrFileUsedElsewhere
		}
	}

	owners, err := r.FindByFileID(ctx, fileID)
	if err != nil {
		return err
	}
	for _, o := range owners {
		if o.ID == storyID {
			return nil
		}
	}

	if err := r.AssignFiles(ctx, storyID, []models.FileID{fileID}); err != nil {
		return err
	}
	if err := r.UpdateText(ctx, storyID, ""); err != nil {
		return err
	}

	for _, o := range owners {
		files, err := r.GetFiles(ctx, o.ID)
		if err != nil {
			return err
		}

		if len(files) == 0 {
			if o.Title == "" {
				if err := r.Destroy(ctx, o.ID); err != nil {
					return err
				}
			}
			continue
		}

		// it keeps its other files, one of which becomes its main file
		if o.PrimaryFileID != nil && *o.PrimaryFileID == fileID {
			primary := files[0].Base().ID
			if _, err := r.UpdatePartial(ctx, o.ID, models.StoryPartial{PrimaryFileID: &primary}); err != nil {
				return err
			}
		}
	}

	return nil
}
