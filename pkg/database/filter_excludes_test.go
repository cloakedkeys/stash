//go:build integration

package database_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An EXCLUDES condition on a related object must keep every row that is not
// linked to the excluded object, including rows with no link at all. So for
// any id, the EXCLUDES count plus the INCLUDES count is the total count.
//
// Runs against SQLite normally and PostgreSQL when PGSQL_TEST is set.
func TestRelatedExcludesKeepsUnlinkedRows(t *testing.T) {
	multi := func(m models.CriterionModifier, id int) *models.MultiCriterionInput {
		return &models.MultiCriterionInput{Value: []string{strconv.Itoa(id)}, Modifier: m}
	}
	hier := func(m models.CriterionModifier, id int) *models.HierarchicalMultiCriterionInput {
		return &models.HierarchicalMultiCriterionInput{Value: []string{strconv.Itoa(id)}, Modifier: m}
	}

	tests := []struct {
		name  string
		count func(ctx context.Context, m *models.CriterionModifier) (int, error)
	}{
		{"scenes by galleries", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneFilterType
			if m != nil {
				f = &models.SceneFilterType{Galleries: multi(*m, galleryIDs[galleryIdxWithScene])}
			}
			return db.Scene().QueryCount(ctx, f, nil)
		}},
		{"scenes by movies", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneFilterType
			if m != nil {
				f = &models.SceneFilterType{Movies: multi(*m, groupIDs[groupIdxWithScene])}
			}
			return db.Scene().QueryCount(ctx, f, nil)
		}},
		{"scenes by groups", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneFilterType
			if m != nil {
				f = &models.SceneFilterType{Groups: hier(*m, groupIDs[groupIdxWithScene])}
			}
			return db.Scene().QueryCount(ctx, f, nil)
		}},
		{"scenes by performers", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneFilterType
			if m != nil {
				f = &models.SceneFilterType{Performers: multi(*m, performerIDs[performerIdxWithScene])}
			}
			return db.Scene().QueryCount(ctx, f, nil)
		}},
		{"scenes by studios", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneFilterType
			if m != nil {
				f = &models.SceneFilterType{Studios: hier(*m, studioIDs[studioIdxWithScene])}
			}
			return db.Scene().QueryCount(ctx, f, nil)
		}},
		{"scenes by tags", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneFilterType
			if m != nil {
				f = &models.SceneFilterType{Tags: hier(*m, tagIDs[tagIdxWithScene])}
			}
			return db.Scene().QueryCount(ctx, f, nil)
		}},
		{"galleries by scenes", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.GalleryFilterType
			if m != nil {
				f = &models.GalleryFilterType{Scenes: multi(*m, sceneIDs[sceneIdxWithGallery])}
			}
			return db.Gallery().QueryCount(ctx, f, nil)
		}},
		{"galleries by audios", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.GalleryFilterType
			if m != nil {
				f = &models.GalleryFilterType{Audios: multi(*m, audioIDs[audioIdxWithGallery])}
			}
			return db.Gallery().QueryCount(ctx, f, nil)
		}},
		{"audios by galleries", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.AudioFilterType
			if m != nil {
				f = &models.AudioFilterType{Galleries: multi(*m, galleryIDs[galleryIdxWithAudio])}
			}
			return db.Audio().QueryCount(ctx, f, nil)
		}},
		{"images by galleries", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.ImageFilterType
			if m != nil {
				f = &models.ImageFilterType{Galleries: multi(*m, galleryIDs[galleryIdxWithImage])}
			}
			return db.Image().QueryCount(ctx, f, nil)
		}},
		{"studios by parents", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.StudioFilterType
			if m != nil {
				f = &models.StudioFilterType{Parents: multi(*m, studioIDs[studioIdxWithChildStudio])}
			}
			return db.Studio().QueryCount(ctx, f, nil)
		}},
		{"scene markers by scenes", func(ctx context.Context, m *models.CriterionModifier) (int, error) {
			var f *models.SceneMarkerFilterType
			if m != nil {
				f = &models.SceneMarkerFilterType{Scenes: multi(*m, sceneIDs[sceneIdxWithMarkers])}
			}
			return db.SceneMarker().QueryCount(ctx, f, nil)
		}},
	}

	includes := models.CriterionModifierIncludes
	excludes := models.CriterionModifierExcludes
	for _, tt := range tests {
		runWithRollbackTxn(t, tt.name, func(t *testing.T, ctx context.Context) {
			total, err := tt.count(ctx, nil)
			require.NoError(t, err)
			included, err := tt.count(ctx, &includes)
			require.NoError(t, err)
			excluded, err := tt.count(ctx, &excludes)
			require.NoError(t, err)

			require.Positive(t, included, "test data should link the chosen id")
			require.Less(t, included, total, "test data should have rows without the chosen id")
			assert.Equal(t, total-included, excluded)
		})
	}
}
