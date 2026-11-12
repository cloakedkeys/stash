package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var errRangeRefused = errors.New("range refused")

// rangeSceneService records the range it's asked to check, and refuses it.
type rangeSceneService struct {
	manager.SceneService
	called     bool
	start, end *float64
}

func (s *rangeSceneService) ValidateRangeUpdate(ctx context.Context, scene *models.Scene, start, end *float64) (*float64, *float64, error) {
	s.called = true
	s.start, s.end = start, end
	return nil, nil, errRangeRefused
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

type keyNaming struct{}

func (keyNaming) GetVideoFileNamingAlgorithm() models.HashAlgorithm {
	return models.HashAlgorithmOshash
}

func TestSceneUpdateInvalidatesChangedRangeKey(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name          string
		before, after *float64
	}{
		{"range edit", ptr(10), ptr(20)}, {"part to whole", ptr(10), nil}, {"whole to part", nil, ptr(10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := paths.NewPaths(t.TempDir(), "")
			f := &models.VideoFile{BaseFile: &models.BaseFile{ID: 100}, Duration: 60}
			old := &models.Scene{ID: 1, OSHash: "0123456789abcdef", PrimaryFileID: &f.ID, PrimaryFileDuration: 60, StartTime: tc.before, Files: models.NewRelatedVideoFiles([]*models.VideoFile{f})}
			updated := *old
			updated.StartTime = tc.after
			path := p.Scene.SpriteImage(scene.GeneratedKey(old, models.HashAlgorithmOshash)).Path
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
			require.NoError(t, os.WriteFile(path, []byte("generated"), 0600))
			db := mocks.NewDatabase()
			db.Scene.On("Find", mock.Anything, 1).Return(old, nil)
			db.Scene.On("FindByFileID", mock.Anything, f.ID).Return([]*models.Scene{old}, nil).Maybe()
			db.Scene.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(&updated, nil)
			db.Scene.On("FindByFingerprints", mock.Anything, mock.Anything).Return([]*models.Scene{&updated}, nil)
			r := newResolver(db)
			r.sceneService = &scene.Service{Repository: db.Scene, File: db.File, Paths: &p, Config: keyNaming{}}
			err := txn.WithTxn(context.Background(), db, func(ctx context.Context) error {
				_, err := r.Mutation().(*mutationResolver).sceneUpdate(ctx, models.SceneUpdateInput{ID: "1", StartTime: tc.after}, changesetTranslator{inputMap: map[string]interface{}{"start_time": tc.after}})
				require.FileExists(t, path, "cleanup waits for commit")
				return err
			})
			require.NoError(t, err)
			require.NoFileExists(t, path)
		})
	}
}
