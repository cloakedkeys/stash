package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type apiVoiceActorStore struct {
	models.VoiceActorReaderWriter
	actor *models.VoiceActor
}

func (s *apiVoiceActorStore) Create(_ context.Context, a *models.VoiceActor) error {
	a.ID = 42
	s.actor = a
	return nil
}
func (s *apiVoiceActorStore) Find(_ context.Context, id int) (*models.VoiceActor, error) {
	if s.actor != nil && s.actor.ID == id {
		return s.actor, nil
	}
	return nil, nil
}
func (s *apiVoiceActorStore) Update(_ context.Context, a *models.VoiceActor) error {
	s.actor = a
	return nil
}
func (s *apiVoiceActorStore) Destroy(_ context.Context, id int) error { s.actor = nil; return nil }
func (s *apiVoiceActorStore) Query(_ context.Context, _ *models.FindFilterType, _ *bool) ([]*models.VoiceActor, int, error) {
	return []*models.VoiceActor{s.actor}, 1, nil
}
func (s *apiVoiceActorStore) SceneIDs(_ context.Context, id int) ([]int, error) { return []int{7}, nil }
func (s *apiVoiceActorStore) FindBySceneID(_ context.Context, id int) ([]*models.VoiceActor, error) {
	return []*models.VoiceActor{s.actor}, nil
}

func TestVoiceActorResolvers(t *testing.T) {
	db := mocks.NewDatabase()
	r := newResolver(db)
	store := &apiVoiceActorStore{}
	r.repository.VoiceActor = store
	ctx := context.Background()
	details := "Voice work"
	favorite := true
	actor, err := r.Mutation().VoiceActorCreate(ctx, models.VoiceActorCreateInput{Name: "Voice Artist", Details: &details, Aliases: []string{"Alias"}, URLs: []string{"https://example.com"}, Favorite: &favorite})
	require.NoError(t, err)
	require.Equal(t, 42, actor.ID)
	require.True(t, actor.Favorite)
	require.False(t, actor.CreatedAt.IsZero())
	name := "Updated"
	no := false
	actor, err = r.Mutation().VoiceActorUpdate(ctx, models.VoiceActorUpdateInput{ID: "42", Name: &name, Favorite: &no, Aliases: []string{}})
	require.NoError(t, err)
	require.Equal(t, "Updated", actor.Name)
	require.Equal(t, details, actor.Details)
	require.Empty(t, actor.Aliases)
	require.False(t, actor.Favorite)
	found, err := r.Query().FindVoiceActor(ctx, "42")
	require.NoError(t, err)
	require.Equal(t, actor, found)
	result, err := r.Query().FindVoiceActors(ctx, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.Count)
	db.Scene.On("FindMany", mock.Anything, []int{7}).Return([]*models.Scene{{ID: 7, Title: "Animation"}}, nil).Once()
	scenes, err := r.VoiceActor().Scenes(ctx, actor)
	require.NoError(t, err)
	require.Equal(t, "Animation", scenes[0].Title)
	count, err := r.VoiceActor().SceneCount(ctx, actor)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	credits, err := r.Scene().VoiceActors(ctx, &models.Scene{ID: 7})
	require.NoError(t, err)
	require.Equal(t, actor, credits[0])
	ok, err := r.Mutation().VoiceActorDestroy(ctx, "42")
	require.NoError(t, err)
	require.True(t, ok)
	found, err = r.Query().FindVoiceActor(ctx, "42")
	require.NoError(t, err)
	require.Nil(t, found)
	_, err = r.Mutation().VoiceActorUpdate(ctx, models.VoiceActorUpdateInput{ID: "42"})
	require.Error(t, err)
	db.AssertExpectations(t)
}
