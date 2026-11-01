package api

import (
	"context"
	"slices"
	"strconv"

	"github.com/99designs/gqlgen/graphql"

	"github.com/stashapp/stash/pkg/models"
)

func (r *queryResolver) FindStory(ctx context.Context, id string) (ret *models.Story, err error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.Story.Find(ctx, idInt)
		return err
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (r *queryResolver) FindStories(ctx context.Context, storyFilter *models.StoryFilterType, filter *models.FindFilterType) (ret *FindStoriesResultType, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		fields := graphql.CollectAllFields(ctx)

		result, err := r.repository.Story.Query(ctx, models.StoryQueryOptions{
			QueryOptions: models.QueryOptions{
				FindFilter: filter,
				Count:      slices.Contains(fields, "count"),
			},
			StoryFilter: storyFilter,
		})
		if err != nil {
			return err
		}

		stories, err := result.Resolve(ctx)
		if err != nil {
			return err
		}

		ret = &FindStoriesResultType{
			Count:   result.Count,
			Stories: stories,
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}
