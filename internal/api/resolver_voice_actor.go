package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/stashapp/stash/pkg/models"
)

func (r *queryResolver) FindVoiceActor(ctx context.Context, id string) (ret *models.VoiceActor, err error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	err = r.withReadTxn(ctx, func(ctx context.Context) error { ret, err = r.repository.VoiceActor.Find(ctx, idInt); return err })
	return
}
func (r *queryResolver) FindVoiceActors(ctx context.Context, filter *models.FindFilterType, favorite *bool) (ret *models.FindVoiceActorsResult, err error) {
	err = r.withReadTxn(ctx, func(ctx context.Context) error {
		actors, count, err := r.repository.VoiceActor.Query(ctx, filter, favorite)
		ret = &models.FindVoiceActorsResult{VoiceActors: actors, Count: count}
		return err
	})
	return
}
func (r *mutationResolver) VoiceActorCreate(ctx context.Context, input models.VoiceActorCreateInput) (ret *models.VoiceActor, err error) {
	ret = &models.VoiceActor{Name: input.Name, Aliases: input.Aliases, URLs: input.URLs, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if input.Details != nil {
		ret.Details = *input.Details
	}
	if input.Favorite != nil {
		ret.Favorite = *input.Favorite
	}
	err = r.withTxn(ctx, func(ctx context.Context) error { return r.repository.VoiceActor.Create(ctx, ret) })
	return
}
func (r *mutationResolver) VoiceActorUpdate(ctx context.Context, input models.VoiceActorUpdateInput) (ret *models.VoiceActor, err error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, err
	}
	err = r.withTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.VoiceActor.Find(ctx, id)
		if err != nil {
			return err
		}
		if ret == nil {
			return fmt.Errorf("voice actor %d not found", id)
		}
		if input.Name != nil {
			ret.Name = *input.Name
		}
		if input.Details != nil {
			ret.Details = *input.Details
		}
		if input.Aliases != nil {
			ret.Aliases = input.Aliases
		}
		if input.URLs != nil {
			ret.URLs = input.URLs
		}
		if input.Favorite != nil {
			ret.Favorite = *input.Favorite
		}
		ret.UpdatedAt = time.Now()
		return r.repository.VoiceActor.Update(ctx, ret)
	})
	return
}
func (r *mutationResolver) VoiceActorDestroy(ctx context.Context, id string) (bool, error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return false, err
	}
	err = r.withTxn(ctx, func(ctx context.Context) error { return r.repository.VoiceActor.Destroy(ctx, idInt) })
	return err == nil, err
}
func (r *voiceActorResolver) Scenes(ctx context.Context, obj *models.VoiceActor) (ret []*models.Scene, err error) {
	err = r.withReadTxn(ctx, func(ctx context.Context) error {
		ids, err := r.repository.VoiceActor.SceneIDs(ctx, obj.ID)
		if err != nil {
			return err
		}
		ret = []*models.Scene{}
		if len(ids) == 0 {
			return nil
		}
		ret, err = r.repository.Scene.FindMany(ctx, ids)
		return err
	})
	return
}
func (r *voiceActorResolver) SceneCount(ctx context.Context, obj *models.VoiceActor) (ret int, err error) {
	err = r.withReadTxn(ctx, func(ctx context.Context) error {
		ids, err := r.repository.VoiceActor.SceneIDs(ctx, obj.ID)
		ret = len(ids)
		return err
	})
	return
}
func (r *sceneResolver) VoiceActors(ctx context.Context, obj *models.Scene) (ret []*models.VoiceActor, err error) {
	err = r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.VoiceActor.FindBySceneID(ctx, obj.ID)
		return err
	})
	return
}
