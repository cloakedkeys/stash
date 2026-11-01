package postgres

import (
	"context"
	"github.com/stashapp/stash/pkg/models"
)

type storyFilterHandler struct {
	storyFilter *models.StoryFilterType
}

func (qb *storyFilterHandler) validate() error {
	storyFilter := qb.storyFilter
	if storyFilter == nil {
		return nil
	}

	if err := validateFilterCombination(storyFilter.OperatorFilter); err != nil {
		return err
	}

	if subFilter := storyFilter.SubFilter(); subFilter != nil {
		sqb := &storyFilterHandler{storyFilter: subFilter}
		if err := sqb.validate(); err != nil {
			return err
		}
	}

	return nil
}

func (qb *storyFilterHandler) handle(ctx context.Context, f *filterBuilder) {
	storyFilter := qb.storyFilter
	if storyFilter == nil {
		return
	}

	if err := qb.validate(); err != nil {
		f.setError(err)
		return
	}

	f.handleCriterion(ctx, qb.criterionHandler())

	sf := storyFilter.SubFilter()
	if sf != nil {
		sub := &storyFilterHandler{sf}
		handleSubFilter(ctx, sub, f, storyFilter.OperatorFilter)
	}
}

func (qb *storyFilterHandler) criterionHandler() criterionHandler {
	f := qb.storyFilter
	return compoundHandler{
		stringCriterionHandler(f.Title, "stories.title"),
		storyUserData.intCriterionHandler(f.Rating100, storyUserData.column("rating")),
		qb.tagsCriterionHandler(f.Tags),
	}
}
func (qb *storyFilterHandler) tagsCriterionHandler(tags *models.HierarchicalMultiCriterionInput) criterionHandlerFunc {
	h := joinedHierarchicalMultiCriterionHandlerBuilder{
		primaryTable: storyTable,
		foreignTable: tagTable,
		foreignFK:    "tag_id",

		relationsTable: "tags_relations",
		joinAs:         "story_tag",
		joinTable:      storiesTagsTable,
		primaryFK:      storyIDColumn,
	}

	return h.handler(tags)
}
