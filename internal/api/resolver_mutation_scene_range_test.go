package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errRangeRefused = errors.New("range refused")

// rangeSceneService records the range it's asked to check, and refuses it.
type rangeSceneService struct {
	manager.SceneService
	called     bool
	start, end *float64
}

func (s *rangeSceneService) ValidateRangeUpdate(ctx context.Context, scene *models.Scene, start, end *float64) error {
	s.called = true
	s.start, s.end = start, end
	return errRangeRefused
}

func TestSceneUpdateChecksResultingRange(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	for _, c := range []struct {
		name       string
		input      map[string]interface{}
		start, end *float64
		checked    bool
	}{
		{"changing the end keeps the start", map[string]interface{}{"end_time": 30.0}, f(10), f(30), true},
		{"changing the start keeps the end", map[string]interface{}{"start_time": 5.0}, f(5), f(20), true},
		{"clearing the start", map[string]interface{}{"start_time": nil}, nil, f(20), true},
		{"clearing both", map[string]interface{}{"start_time": nil, "end_time": nil}, nil, nil, true},
		{"not changing the range", map[string]interface{}{"title": "t"}, nil, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := mocks.NewDatabase()
			svc := &rangeSceneService{}
			r := newResolver(db)
			r.sceneService = svc

			db.Scene.On("Find", mock.Anything, 1).Return(&models.Scene{ID: 1, StartTime: f(10), EndTime: f(20)}, nil).Once()
			if !c.checked {
				db.Scene.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(nil, errRangeRefused).Once()
			}

			input := models.SceneUpdateInput{ID: "1"}
			if v, ok := c.input["start_time"].(float64); ok {
				input.StartTime = &v
			}
			if v, ok := c.input["end_time"].(float64); ok {
				input.EndTime = &v
			}
			if v, ok := c.input["title"].(string); ok {
				input.Title = &v
			}

			_, err := r.Mutation().(*mutationResolver).sceneUpdate(context.Background(), input, changesetTranslator{inputMap: c.input})
			assert.ErrorIs(t, err, errRangeRefused)
			assert.Equal(t, c.checked, svc.called)
			assert.Equal(t, c.start, svc.start)
			assert.Equal(t, c.end, svc.end)
			db.AssertExpectations(t)
		})
	}
}
