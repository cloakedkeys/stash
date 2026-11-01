package models

import "context"

type StoryFilterType struct {
	OperatorFilter[StoryFilterType]
	Title     *StringCriterionInput
	Tags      *HierarchicalMultiCriterionInput
	Rating100 *IntCriterionInput
}
type StoryQueryOptions struct {
	QueryOptions
	StoryFilter *StoryFilterType
}
type StoryQueryResult struct {
	QueryResult[int]
	getter     StoryGetter
	stories    []*Story
	resolveErr error
}

func NewStoryQueryResult(getter StoryGetter) *StoryQueryResult {
	return &StoryQueryResult{getter: getter}
}
func (r *StoryQueryResult) Resolve(ctx context.Context) ([]*Story, error) {
	if r.stories == nil && r.resolveErr == nil {
		r.stories, r.resolveErr = r.getter.FindMany(ctx, r.IDs)
	}
	return r.stories, r.resolveErr
}
