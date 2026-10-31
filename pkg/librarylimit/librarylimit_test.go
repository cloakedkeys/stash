package librarylimit

import (
	"context"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestWithoutALimit(t *testing.T) {
	ctx := context.Background()

	_, ok := Where(ctx, Scenes)
	assert.False(t, ok)
	assert.Equal(t, "SELECT 1", WrapQuery(ctx, "SELECT 1", Scenes))

	_, ok = Where(models.WithLibraryLimit(ctx, nil), Scenes)
	assert.False(t, ok)
}

func TestEmptyLimitAllowsNothing(t *testing.T) {
	ctx := models.WithLibraryLimit(context.Background(), &models.LibraryLimit{})

	where, ok := Where(ctx, Scenes)
	assert.True(t, ok)
	assert.Contains(t, where, nothing)

	for _, kind := range []Kind{Scenes, Images, Galleries, Audios, SceneMarkers, Performers, Tags, Studios, Groups, VoiceActors} {
		_, ok := Where(ctx, kind)
		assert.True(t, ok)
		assert.NotContains(t, visible(&models.LibraryLimit{}, kind), "IN ()")
	}
}

func TestUnknownKindAllowsNothing(t *testing.T) {
	assert.Equal(t, nothing, visible(&models.LibraryLimit{TagIDs: []int{1}}, Kind(1000)))
}

func TestOnlyIntegersReachTheSQL(t *testing.T) {
	sql := visible(&models.LibraryLimit{FolderIDs: []int{1, -2}, TagIDs: []int{3}, StudioIDs: []int{4}}, Scenes)
	assert.Contains(t, sql, "IN (1,-2)")
	assert.Contains(t, sql, "IN (3)")
	assert.Contains(t, sql, "IN (4)")
	assert.False(t, strings.Contains(sql, "?"))
}

func TestKeepVisible(t *testing.T) {
	assert.Equal(t, []int{3, 1}, KeepVisible([]int{3, 2, 1}, []int{1, 3}))
	assert.Empty(t, KeepVisible([]int{1}, nil))
}

func TestIDsQueryWithoutIDs(t *testing.T) {
	assert.Empty(t, IDsQuery(&models.LibraryLimit{}, Scenes, nil))
}
