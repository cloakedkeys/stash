package api

import (
	"context"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/internal/api/urlbuilders"
	"github.com/stashapp/stash/pkg/models"
)

func (r *storyResolver) Date(ctx context.Context, obj *models.Story) (*string, error) {
	if obj.Date != nil {
		result := obj.Date.String()
		return &result, nil
	}
	return nil, nil
}

func (r *storyResolver) Rating100(ctx context.Context, obj *models.Story) (*int, error) {
	return obj.Rating, nil
}

func (r *storyResolver) Urls(ctx context.Context, obj *models.Story) ([]string, error) {
	if !obj.URLs.Loaded() {
		if err := r.withReadTxn(ctx, func(ctx context.Context) error {
			return obj.LoadURLs(ctx, r.repository.Story)
		}); err != nil {
			return nil, err
		}
	}

	return obj.URLs.List(), nil
}

func (r *storyResolver) Files(ctx context.Context, obj *models.Story) ([]*BasicFile, error) {
	if !obj.Files.Loaded() {
		if err := r.withReadTxn(ctx, func(ctx context.Context) error {
			return obj.LoadFiles(ctx, r.repository.Story)
		}); err != nil {
			return nil, err
		}
	}

	files := obj.Files.List()
	ret := make([]*BasicFile, len(files))
	for i, f := range files {
		ret[i] = &BasicFile{BaseFile: f.Base()}
	}

	return ret, nil
}

func (r *storyResolver) Paths(ctx context.Context, obj *models.Story) (*StoryPathsType, error) {
	baseURL, _ := ctx.Value(BaseURLCtxKey).(string)
	text := urlbuilders.NewStoryURLBuilder(baseURL, obj).GetTextURL()

	return &StoryPathsType{
		Text: &text,
	}, nil
}

func (r *storyResolver) Tags(ctx context.Context, obj *models.Story) (ret []*models.Tag, err error) {
	if !obj.TagIDs.Loaded() {
		if err := r.withReadTxn(ctx, func(ctx context.Context) error {
			return obj.LoadTagIDs(ctx, r.repository.Story)
		}); err != nil {
			return nil, err
		}
	}

	var errs []error
	ret, errs = loaders.From(ctx).TagByID.LoadAll(obj.TagIDs.List())
	return withoutNil(ret), firstError(errs)
}
