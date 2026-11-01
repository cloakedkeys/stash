package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin/hook"
	"github.com/stashapp/stash/pkg/utils"
	"strconv"

	"github.com/stashapp/stash/internal/api/loaders"
)

func (r *mutationResolver) updatePerformerImages(ctx context.Context, id string, input interface{}, fields []string, apply func(context.Context, int) error) (*models.Performer, error) {
	performerID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		p, err := r.repository.Performer.Find(ctx, performerID)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("performer %s not found", id)
		}
		if err := apply(ctx, performerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("image checksum does not belong to performer %s", id)
			}
			return err
		}
		_, err = r.repository.Performer.UpdatePartial(ctx, performerID, models.NewPerformerPartial())
		return err
	}); err != nil {
		return nil, err
	}
	r.hookExecutor.ExecutePostHooks(ctx, performerID, hook.PerformerUpdatePost, input, fields)
	loaders.InvalidatePerformerImages(ctx, performerID)
	return r.getPerformer(ctx, performerID)
}

func (r *mutationResolver) PerformerImageAdd(ctx context.Context, id string, image string) (*models.Performer, error) {
	data, err := utils.ProcessImageInput(ctx, image)
	if err != nil {
		return nil, fmt.Errorf("processing image: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image must not be empty")
	}
	return r.updatePerformerImages(ctx, id, models.PerformerUpdateInput{ID: id, Image: &image}, []string{"alternate_images"}, func(ctx context.Context, id int) error {
		_, err := r.repository.Performer.AddAlternateImage(ctx, id, data)
		return err
	})
}

func (r *mutationResolver) PerformerImageRemove(ctx context.Context, id string, checksum string) (*models.Performer, error) {
	return r.updatePerformerImages(ctx, id, map[string]interface{}{"id": id, "checksum": checksum}, []string{"alternate_images"}, func(ctx context.Context, id int) error {
		return r.repository.Performer.RemoveAlternateImage(ctx, id, checksum)
	})
}

func (r *mutationResolver) PerformerImageSetPrimary(ctx context.Context, id string, checksum string) (*models.Performer, error) {
	return r.updatePerformerImages(ctx, id, map[string]interface{}{"id": id, "checksum": checksum}, []string{"image", "alternate_images"}, func(ctx context.Context, id int) error {
		return r.repository.Performer.SetPrimaryImage(ctx, id, checksum)
	})
}

func (r *mutationResolver) PerformerImagesReorder(ctx context.Context, id string, checksums []string) (*models.Performer, error) {
	return r.updatePerformerImages(ctx, id, map[string]interface{}{"id": id, "checksums": checksums}, []string{"alternate_images"}, func(ctx context.Context, id int) error {
		if err := r.repository.Performer.ReorderAlternateImages(ctx, id, checksums); err != nil {
			return fmt.Errorf("reordering images for performer %d: %w", id, err)
		}
		return nil
	})
}
