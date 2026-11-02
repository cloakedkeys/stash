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

func TestPerformerStudioScopedOCounter(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	db.Performer.On("Find", mock.Anything, 1).Return(&models.Performer{ID: 1}, nil).Once()
	db.Performer.On("OCount", mock.Anything, 1, mock.MatchedBy(func(scope *models.StudioScopeInput) bool {
		return scope != nil && scope.ID == "7" && scope.Depth != nil && *scope.Depth == -1
	})).Return(3, nil).Once()
	db.Scene.On("OCountByPerformerID", mock.Anything, 1).Return(10, nil).Once()
	db.Audio.On("OCountByPerformerID", mock.Anything, 1).Return(20, nil).Once()
	db.Image.On("OCountByPerformerID", mock.Anything, 1).Return(30, nil).Once()
	db.Gallery.On("OCountByPerformerID", mock.Anything, 1).Return(40, nil).Once()
	result := performerImageQuery(t, newTestGraphQLServer(r), `{
		findPerformer(id: "1") {
			global: o_counter
			scoped: o_counter(studio_scope: {id: "7", depth: -1})
		}
	}`, models.RoleEnumRead)
	require.NotContains(t, result, "errors")
	var response struct {
		Data struct {
			FindPerformer map[string]int
		}
	}
	require.NoError(t, json.Unmarshal([]byte(result), &response))
	require.Equal(t, map[string]int{"global": 100, "scoped": 3}, response.Data.FindPerformer)
	db.AssertExpectations(t)
}

func TestPerformerStudioScopedOCounterError(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	db.Performer.On("OCount", mock.Anything, 1, mock.Anything).Return(0, context.Canceled).Once()
	_, err := r.Performer().OCounter(context.Background(), &models.Performer{ID: 1}, &models.StudioScopeInput{ID: "7"})
	require.ErrorIs(t, err, context.Canceled)
	db.AssertExpectations(t)
}
