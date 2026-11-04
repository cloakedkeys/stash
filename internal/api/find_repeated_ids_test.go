package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Find queries given the same id twice failed before #2078, so they must
// still fail, now because the id is repeated. This includes the deprecated
// number lists and ids written differently, such as "1" and "01".
func TestFindQueriesRejectRepeatedIDs(t *testing.T) {
	r := newResolver(mocks.NewDatabase())
	q := r.Query()
	ctx := context.Background()
	repeated := []string{"1", "01"}

	tests := []struct {
		name    string
		find    func() error
		wantErr string
	}{
		{"findScenes ids", func() error { _, err := q.FindScenes(ctx, nil, nil, repeated, nil); return err }, "validating ids: duplicate id found: 1"},
		{"findScenes scene_ids", func() error { _, err := q.FindScenes(ctx, nil, []int{1, 2, 1}, nil, nil); return err }, "validating scene_ids: duplicate id found: 1"},
		{"findImages ids", func() error { _, err := q.FindImages(ctx, nil, nil, repeated, nil); return err }, "validating ids: duplicate id found: 1"},
		{"findImages image_ids", func() error { _, err := q.FindImages(ctx, nil, []int{1, 1}, nil, nil); return err }, "validating image_ids: duplicate id found: 1"},
		{"findPerformers ids", func() error { _, err := q.FindPerformers(ctx, nil, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findPerformers performer_ids", func() error { _, err := q.FindPerformers(ctx, nil, nil, []int{1, 1}, nil); return err }, "validating performer_ids: duplicate id found: 1"},
		{"findAudios", func() error { _, err := q.FindAudios(ctx, nil, repeated, nil); return err }, "validating ids: duplicate id found: 1"},
		{"findFolders", func() error { _, err := q.FindFolders(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findGalleries", func() error { _, err := q.FindGalleries(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findGroups", func() error { _, err := q.FindGroups(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findMovies", func() error { _, err := q.FindMovies(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findSceneMarkers", func() error { _, err := q.FindSceneMarkers(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findStudios", func() error { _, err := q.FindStudios(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findTags", func() error { _, err := q.FindTags(ctx, nil, nil, repeated); return err }, "validating ids: duplicate id found: 1"},
		{"findTags written the same", func() error { _, err := q.FindTags(ctx, nil, nil, []string{"1", "1"}); return err }, "validating ids: duplicate id found: 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.find(), tt.wantErr)
		})
	}
}

type voiceActorSceneStore struct {
	models.VoiceActorReaderWriter
	sceneID int
	ids     []int
}

func (s *voiceActorSceneStore) SetScene(_ context.Context, sceneID int, ids []int) error {
	s.sceneID = sceneID
	s.ids = ids
	return nil
}

// Voice actor credits never went through FindMany: the store skips repeats,
// so "1" and "01" set one credit as before.
func TestSceneUpdateVoiceActorIDsWrittenDifferently(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	store := &voiceActorSceneStore{}
	r.repository.VoiceActor = store

	scene := &models.Scene{ID: 1}
	db.Scene.On("Find", mock.Anything, 1).Return(scene, nil).Once()
	db.Scene.On("UpdatePartial", mock.Anything, 1, mock.Anything).Return(scene, nil).Once()

	ids := []string{"1", "01"}
	input := models.SceneUpdateInput{ID: "1", VoiceActorIds: ids}
	translator := changesetTranslator{inputMap: map[string]interface{}{"voice_actor_ids": []interface{}{"1", "01"}}}

	got, err := r.Mutation().(*mutationResolver).sceneUpdate(context.Background(), input, translator)
	require.NoError(t, err)
	assert.Equal(t, scene, got)
	assert.Equal(t, 1, store.sceneID)
	assert.Equal(t, []int{1, 1}, store.ids)
	db.AssertExpectations(t)
}
