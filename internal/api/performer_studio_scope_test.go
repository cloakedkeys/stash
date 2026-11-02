package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPerformerStudioScopedCounts(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	db.Performer.On("Query", mock.Anything, mock.MatchedBy(func(f *models.PerformerFilterType) bool {
		return f.StudioScope != nil && f.StudioScope.ID == "7" && f.StudioScope.Depth != nil && *f.StudioScope.Depth == -1
	}), mock.Anything).Return([]*models.Performer{{ID: 1}}, 1, nil).Once()
	matches := func(studios *models.HierarchicalMultiCriterionInput, performers *models.MultiCriterionInput) bool {
		return studios != nil && len(studios.Value) == 1 && studios.Value[0] == "7" &&
			studios.Modifier == models.CriterionModifierIncludes && studios.Depth != nil && *studios.Depth == -1 &&
			performers != nil && len(performers.Value) == 1 && performers.Value[0] == "1" &&
			performers.Modifier == models.CriterionModifierIncludes
	}
	db.Scene.On("QueryCount", mock.Anything, mock.MatchedBy(func(f *models.SceneFilterType) bool { return matches(f.Studios, f.Performers) }), mock.Anything).Return(2, nil).Once()
	db.Audio.On("QueryCount", mock.Anything, mock.MatchedBy(func(f *models.AudioFilterType) bool { return matches(f.Studios, f.Performers) }), mock.Anything).Return(3, nil).Once()
	db.Image.On("QueryCount", mock.Anything, mock.MatchedBy(func(f *models.ImageFilterType) bool { return matches(f.Studios, f.Performers) }), mock.Anything).Return(4, nil).Once()
	db.Gallery.On("QueryCount", mock.Anything, mock.MatchedBy(func(f *models.GalleryFilterType) bool { return matches(f.Studios, f.Performers) }), mock.Anything).Return(5, nil).Once()
	db.Group.On("QueryCount", mock.Anything, mock.MatchedBy(func(f *models.GroupFilterType) bool { return matches(f.Studios, f.Performers) }), mock.Anything).Return(6, nil).Once()
	db.Scene.On("CountByPerformerID", mock.Anything, 1).Return(20, nil).Once()
	result := performerImageQuery(t, newTestGraphQLServer(r), `{
		findPerformers(performer_filter: {studio_scope: {id: "7", depth: -1}}) {
			performers {
				id
				global: scene_count
				scene_count(studio_scope: {id: "7", depth: -1})
				audio_count(studio_scope: {id: "7", depth: -1})
				image_count(studio_scope: {id: "7", depth: -1})
				gallery_count(studio_scope: {id: "7", depth: -1})
				group_count(studio_scope: {id: "7", depth: -1})
			}
		}
	}`, models.RoleEnumRead)
	require.NotContains(t, result, "errors")
	var response struct {
		Data struct {
			FindPerformers struct {
				Performers []map[string]interface{}
			}
		}
	}
	require.NoError(t, json.Unmarshal([]byte(result), &response))
	require.Equal(t, []map[string]interface{}{{"id": "1", "global": float64(20), "scene_count": float64(2), "audio_count": float64(3), "image_count": float64(4), "gallery_count": float64(5), "group_count": float64(6)}}, response.Data.FindPerformers.Performers)
	db.AssertExpectations(t)
}

func TestPerformerStudioScopedCountError(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	db.Scene.On("QueryCount", mock.Anything, mock.Anything, mock.Anything).Return(0, context.Canceled).Once()
	_, err := r.Performer().SceneCount(context.Background(), &models.Performer{ID: 1}, &models.StudioScopeInput{ID: "7"})
	require.ErrorIs(t, err, context.Canceled)
	db.AssertExpectations(t)
}
