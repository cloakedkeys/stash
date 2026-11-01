// Package story handles stories found by scanning text files.
package story

import (
	"context"
	"errors"
	"fmt"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

type ScanCreator interface {
	FindByFileID(ctx context.Context, fileID models.FileID) ([]*models.Story, error)
	Create(ctx context.Context, newStory *models.Story, fileIDs []models.FileID) error
}

// ScanHandler makes a story for each text file a scan finds that isn't one yet.
type ScanHandler struct {
	CreatorUpdater ScanCreator
}

func (h *ScanHandler) Handle(ctx context.Context, f models.File, oldFile models.File) error {
	if h.CreatorUpdater == nil {
		return errors.New("internal error: CreatorUpdater is required")
	}

	// a moved or changed file keeps its story, which is linked to the file's record
	existing, err := h.CreatorUpdater.FindByFileID(ctx, f.Base().ID)
	if err != nil {
		return fmt.Errorf("finding existing story: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}

	logger.Infof("%s doesn't exist. Creating new story...", f.Base().Path)

	newStory := models.NewStory()
	if err := h.CreatorUpdater.Create(ctx, &newStory, []models.FileID{f.Base().ID}); err != nil {
		return fmt.Errorf("creating new story: %w", err)
	}

	return nil
}
