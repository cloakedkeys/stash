package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

// These mutations set only what belongs to the current user, so unlike the update
// mutations they don't need permission to edit the library, and they don't change
// when the object itself was last updated.

func (r *mutationResolver) SceneUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Scene, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.ScenePartial{Rating: translator.optionalInt(input.Rating100, "rating100"), Favorite: translator.optionalBool(input.Favorite, "favorite")}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Scene.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.SceneUpdatePost, input, translator.getFields())
	return r.getScene(ctx, id)
}

func (r *mutationResolver) AudioUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Audio, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.AudioPartial{Rating: translator.optionalInt(input.Rating100, "rating100"), Favorite: translator.optionalBool(input.Favorite, "favorite")}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Audio.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.AudioUpdatePost, input, translator.getFields())
	return r.getAudio(ctx, id)
}

func (r *mutationResolver) ImageUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Image, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.ImagePartial{Rating: translator.optionalInt(input.Rating100, "rating100"), Favorite: translator.optionalBool(input.Favorite, "favorite")}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Image.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.ImageUpdatePost, input, translator.getFields())
	return r.getImage(ctx, id)
}

func (r *mutationResolver) GalleryUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Gallery, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.GalleryPartial{Rating: translator.optionalInt(input.Rating100, "rating100"), Favorite: translator.optionalBool(input.Favorite, "favorite")}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Gallery.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.GalleryUpdatePost, input, translator.getFields())
	return r.getGallery(ctx, id)
}

func (r *mutationResolver) GroupUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Group, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.GroupPartial{Rating: translator.optionalInt(input.Rating100, "rating100"), Favorite: translator.optionalBool(input.Favorite, "favorite")}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Group.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.GroupUpdatePost, input, translator.getFields())
	return r.getGroup(ctx, id)
}

func (r *mutationResolver) PerformerUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Performer, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.PerformerPartial{
		Rating:   translator.optionalInt(input.Rating100, "rating100"),
		Favorite: translator.optionalBool(input.Favorite, "favorite"),
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Performer.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.PerformerUpdatePost, input, translator.getFields())
	return r.getPerformer(ctx, id)
}

func (r *mutationResolver) StudioUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Studio, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.StudioPartial{
		ID:       id,
		Rating:   translator.optionalInt(input.Rating100, "rating100"),
		Favorite: translator.optionalBool(input.Favorite, "favorite"),
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Studio.UpdatePartial(ctx, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.StudioUpdatePost, input, translator.getFields())
	return r.getStudio(ctx, id)
}

func (r *mutationResolver) TagUserDataUpdate(ctx context.Context, input UserRatingFavoriteInput) (*models.Tag, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	translator := changesetTranslator{inputMap: getUpdateInputMap(ctx)}
	partial := models.TagPartial{
		Rating:   translator.optionalInt(input.Rating100, "rating100"),
		Favorite: translator.optionalBool(input.Favorite, "favorite"),
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		_, err := r.repository.Tag.UpdatePartial(ctx, id, partial)
		return err
	}); err != nil {
		return nil, err
	}

	r.hookExecutor.ExecutePostHooks(ctx, id, hook.TagUpdatePost, input, translator.getFields())
	return r.getTag(ctx, id)
}

func (r *mutationResolver) VoiceActorUserDataUpdate(ctx context.Context, input UserFavoriteInput) (ret *models.VoiceActor, err error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	err = r.withTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.VoiceActor.Find(ctx, id)
		if err != nil {
			return err
		}
		if ret == nil {
			return fmt.Errorf("voice actor %d not found", id)
		}

		ret.Favorite = input.Favorite
		return r.repository.VoiceActor.Update(ctx, ret)
	})

	return ret, err
}
