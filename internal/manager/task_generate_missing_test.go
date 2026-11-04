package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stashapp/stash/internal/log"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// warnRecorder keeps the warnings logged while a test runs.
type warnRecorder struct {
	logger.LoggerImpl
	mu    sync.Mutex
	warns []string
}

func (l *warnRecorder) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprintf(format, args...))
}

func recordWarnings(t *testing.T) *warnRecorder {
	t.Helper()
	base := log.NewLogger()
	base.Init("", false, "Error", 0)
	l := &warnRecorder{LoggerImpl: base}

	orig := logger.Logger
	logger.Logger = l
	t.Cleanup(func() { logger.Logger = orig })
	return l
}

func sceneIDsOf(scenes []*models.Scene) []int {
	ret := []int{}
	for _, s := range scenes {
		ret = append(ret, s.ID)
	}
	return ret
}

func markerIDsOf(markers []*models.SceneMarker) []int {
	ret := []int{}
	for _, m := range markers {
		ret = append(ret, m.ID)
	}
	return ret
}

func imageIDsOf(images []*models.Image) []int {
	ret := []int{}
	for _, i := range images {
		ret = append(ret, i.ID)
	}
	return ret
}

// Generate given an id that no longer exists does every item that does, and
// names the missing ones in a warning (#2092). Before, the scene and image
// lists were dropped whole, and a missing marker stopped the job.
func TestGenerateSkipsMissingIDs(t *testing.T) {
	warns := recordWarnings(t)
	db := mocks.NewDatabase()

	db.Scene.On("FindMany", mock.Anything, []int{1, 2, 3}).Return(nil, errors.New("scene with id 2 not found"))
	db.Scene.On("Find", mock.Anything, 1).Return(&models.Scene{ID: 1}, nil)
	db.Scene.On("Find", mock.Anything, 2).Return(nil, nil)
	db.Scene.On("Find", mock.Anything, 3).Return(&models.Scene{ID: 3}, nil)

	db.SceneMarker.On("FindMany", mock.Anything, []int{10, 11}).Return(nil, errors.New("scene marker with id 11 not found"))
	db.SceneMarker.On("Find", mock.Anything, 10).Return(&models.SceneMarker{ID: 10}, nil)
	db.SceneMarker.On("Find", mock.Anything, 11).Return(nil, nil)

	db.Image.On("FindMany", mock.Anything, []int{20, 21, 22}).Return(nil, errors.New("image with id 20 not found"))
	db.Image.On("Find", mock.Anything, 20).Return(nil, nil)
	db.Image.On("Find", mock.Anything, 21).Return(&models.Image{ID: 21}, nil)
	db.Image.On("Find", mock.Anything, 22).Return(nil, nil)

	db.Gallery.On("FindMany", mock.Anything, []int{30, 31}).Return(nil, errors.New("gallery with id 31 not found"))
	db.Gallery.On("Find", mock.Anything, 30).Return(&models.Gallery{ID: 30}, nil)
	db.Gallery.On("Find", mock.Anything, 31).Return(nil, nil)
	db.Image.On("FindByGalleryID", mock.Anything, 30).Return([]*models.Image{{ID: 40}, {ID: 41}}, nil)

	j := &GenerateJob{
		repository: db.Repository(),
		input: GenerateMetadataInput{
			SceneIDs:   []string{"1", "2", "3"},
			MarkerIDs:  []string{"10", "11"},
			ImageIDs:   []string{"20", "21", "22"},
			GalleryIDs: []string{"30", "31"},
		},
	}

	got, err := j.findTargets(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []int{1, 3}, sceneIDsOf(got.scenes))
	assert.Equal(t, []int{10}, markerIDsOf(got.markers))
	assert.Equal(t, []int{21, 40, 41}, imageIDsOf(got.images))
	assert.Equal(t, []string{
		"Generate: skipping scene ids that no longer exist: 2",
		"Generate: skipping marker ids that no longer exist: 11",
		"Generate: skipping image ids that no longer exist: 20, 22",
		"Generate: skipping gallery ids that no longer exist: 31",
	}, warns.warns)
	db.AssertExpectations(t)
}

// When every item exists, each list is found in one lookup and nothing is
// logged, as before.
func TestGenerateFindsAllExistingIDs(t *testing.T) {
	warns := recordWarnings(t)
	db := mocks.NewDatabase()

	db.Scene.On("FindMany", mock.Anything, []int{2, 1}).Return([]*models.Scene{{ID: 2}, {ID: 1}}, nil)
	db.SceneMarker.On("FindMany", mock.Anything, []int{10}).Return([]*models.SceneMarker{{ID: 10}}, nil)
	db.Image.On("FindMany", mock.Anything, []int{20}).Return([]*models.Image{{ID: 20}}, nil)
	// develop didn't look galleries up first
	db.Gallery.On("FindMany", mock.Anything, []int{30}).Return([]*models.Gallery{{ID: 30}}, nil).Maybe()
	db.Image.On("FindByGalleryID", mock.Anything, 30).Return([]*models.Image{{ID: 40}}, nil)

	j := &GenerateJob{
		repository: db.Repository(),
		input: GenerateMetadataInput{
			SceneIDs:   []string{"2", "1", "02"},
			MarkerIDs:  []string{"10"},
			ImageIDs:   []string{"20"},
			GalleryIDs: []string{"30"},
		},
	}

	got, err := j.findTargets(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []int{2, 1}, sceneIDsOf(got.scenes))
	assert.Equal(t, []int{10}, markerIDsOf(got.markers))
	assert.Equal(t, []int{20, 40}, imageIDsOf(got.images))
	assert.Empty(t, warns.warns)
	db.AssertExpectations(t)
	db.Scene.AssertNotCalled(t, "Find", mock.Anything, mock.Anything)
}

// An error that isn't a missing item, such as a database failure, still
// stops Generate.
func TestGenerateFailsOnOtherErrors(t *testing.T) {
	dbErr := errors.New("database is locked")

	t.Run("lookup of one item fails", func(t *testing.T) {
		recordWarnings(t)
		db := mocks.NewDatabase()
		db.Image.On("FindMany", mock.Anything, []int{1, 2}).Return(nil, dbErr)
		db.Image.On("Find", mock.Anything, 1).Return(nil, dbErr)

		j := &GenerateJob{repository: db.Repository(), input: GenerateMetadataInput{ImageIDs: []string{"1", "2"}}}
		_, err := j.findTargets(context.Background())
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("every item exists", func(t *testing.T) {
		warns := recordWarnings(t)
		db := mocks.NewDatabase()
		db.SceneMarker.On("FindMany", mock.Anything, []int{1}).Return(nil, dbErr)
		db.SceneMarker.On("Find", mock.Anything, 1).Return(&models.SceneMarker{ID: 1}, nil)

		j := &GenerateJob{repository: db.Repository(), input: GenerateMetadataInput{MarkerIDs: []string{"1"}}}
		_, err := j.findTargets(context.Background())
		assert.ErrorIs(t, err, dbErr)
		assert.Empty(t, warns.warns)
	})

	t.Run("gallery images", func(t *testing.T) {
		recordWarnings(t)
		db := mocks.NewDatabase()
		db.Gallery.On("FindMany", mock.Anything, []int{1}).Return([]*models.Gallery{{ID: 1}}, nil).Maybe()
		db.Image.On("FindByGalleryID", mock.Anything, 1).Return(nil, dbErr)

		j := &GenerateJob{repository: db.Repository(), input: GenerateMetadataInput{GalleryIDs: []string{"1"}}}
		_, err := j.findTargets(context.Background())
		assert.ErrorIs(t, err, dbErr)
	})
}
