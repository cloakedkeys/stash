package performer

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/utils"
)

// ScrapedOtherImages loads a scraped performer's images after the first one,
// which is the main image. An image that can't be loaded is skipped with a
// warning, so one broken link doesn't stop the rest. Nothing is loaded when
// the image field is excluded.
func ScrapedOtherImages(ctx context.Context, p *models.ScrapedPerformer, excluded map[string]bool) [][]byte {
	if excluded["image"] || len(p.Images) < 2 {
		return nil
	}

	var ret [][]byte
	for _, img := range p.Images[1:] {
		data, err := utils.ProcessImageInput(ctx, img)
		if err != nil {
			name := ""
			if p.Name != nil {
				name = *p.Name
			}
			logger.Warnf("Skipping an image for performer %s: %v", name, err)
			continue
		}
		if len(data) > 0 {
			ret = append(ret, data)
		}
	}
	return ret
}

type AlternateImageAdder interface {
	AddAlternateImage(ctx context.Context, performerID int, image []byte) (bool, error)
}

// AddAlternateImages adds images to a performer's other images, in order.
// Images the performer already has are skipped.
func AddAlternateImages(ctx context.Context, w AlternateImageAdder, performerID int, images [][]byte) error {
	for _, img := range images {
		if _, err := w.AddAlternateImage(ctx, performerID, img); err != nil {
			return fmt.Errorf("adding image to performer %d: %w", performerID, err)
		}
	}
	return nil
}
