package identify

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

func createMissingStudio(ctx context.Context, endpoint string, w models.StudioReaderWriter, s *models.ScrapedStudio) (*int, error) {
	var err error

	if s.Parent != nil {
		if s.Parent.StoredID == nil {
			// The parent needs to be created
			newParentStudio := s.Parent.ToStudio(endpoint, nil)
			parentImage, err := s.Parent.GetImage(ctx, nil)
			if err != nil {
				logger.Errorf("Failed to make parent studio from scraped studio %s: %s", s.Parent.Name, err.Error())
				return nil, err
			}

			// Create the studio
			err = w.Create(ctx, newParentStudio)
			if err != nil {
				return nil, err
			}

			// Update image table
			if len(parentImage) > 0 {
				if err := w.UpdateImage(ctx, newParentStudio.ID, parentImage); err != nil {
					return nil, err
				}
			}

			storedId := strconv.Itoa(newParentStudio.ID)
			s.Parent.StoredID = &storedId
		}
	}

	newStudio := s.ToStudio(endpoint, nil)
	studioImage, err := s.GetImage(ctx, nil)
	if err != nil {
		return nil, err
	}

	err = w.Create(ctx, newStudio)
	if err != nil {
		return nil, err
	}

	// Update image table
	if len(studioImage) > 0 {
		if err := w.UpdateImage(ctx, newStudio.ID, studioImage); err != nil {
			return nil, err
		}
	}

	return &newStudio.ID, nil
}
