package manager

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type storyPurgeStore struct {
	models.StoryReaderWriter
	story     *models.Story
	files     []models.File
	destroyed bool
	primary   *models.FileID
}

func (s *storyPurgeStore) FindByFileID(context.Context, models.FileID) ([]*models.Story, error) {
	return []*models.Story{s.story}, nil
}
func (s *storyPurgeStore) GetFiles(context.Context, int) ([]models.File, error) { return s.files, nil }
func (s *storyPurgeStore) Destroy(context.Context, int) error                   { s.destroyed = true; return nil }
func (s *storyPurgeStore) UpdatePartial(_ context.Context, _ int, p models.StoryPartial) (*models.Story, error) {
	s.primary = p.PrimaryFileID
	return s.story, nil
}

func TestPurgeRelatedStories(t *testing.T) {
	missing, remaining := models.FileID(1), models.FileID(2)
	for _, test := range []struct {
		name          string
		files         []models.File
		primary       models.FileID
		destroy       bool
		changePrimary bool
	}{
		{"last file", []models.File{&models.BaseFile{ID: missing}}, missing, true, false},
		{"missing primary", []models.File{&models.BaseFile{ID: missing}, &models.BaseFile{ID: remaining}}, missing, false, true},
		{"missing secondary", []models.File{&models.BaseFile{ID: missing}, &models.BaseFile{ID: remaining}}, remaining, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &storyPurgeStore{story: &models.Story{ID: 7, PrimaryFileID: &test.primary}, files: test.files}
			require.NoError(t, purgeRelatedStories(context.Background(), store, missing))
			assert.Equal(t, test.destroy, store.destroyed)
			if test.changePrimary {
				assert.Equal(t, &remaining, store.primary)
			} else {
				assert.Nil(t, store.primary)
			}
		})
	}
}
