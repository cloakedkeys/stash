package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

type metadataVoiceActorStore struct {
	models.VoiceActorReaderWriter
	actors []*models.VoiceActor
}

func (s *metadataVoiceActorStore) Query(_ context.Context, _ *models.FindFilterType, _ *bool) ([]*models.VoiceActor, int, error) {
	return s.actors, len(s.actors), nil
}
func (s *metadataVoiceActorStore) FindByName(_ context.Context, name string) (*models.VoiceActor, error) {
	for _, a := range s.actors {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, nil
}
func (s *metadataVoiceActorStore) Create(_ context.Context, a *models.VoiceActor) error {
	a.ID = len(s.actors) + 100
	s.actors = append(s.actors, a)
	return nil
}
func (s *metadataVoiceActorStore) Update(_ context.Context, a *models.VoiceActor) error {
	for n, existing := range s.actors {
		if existing.ID == a.ID {
			s.actors[n] = a
		}
	}
	return nil
}

func TestVoiceActorMetadataRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := &metadataVoiceActorStore{actors: []*models.VoiceActor{
		{ID: 1, Name: "A/B", Details: "Animation", Aliases: []string{"Alias"}, URLs: []string{"https://example.com"}, Favorite: true},
		{ID: 2, Name: "A:B", Details: "ADR"},
	}}
	db := metadataTxnManager{}
	repo := models.Repository{TxnManager: &db, VoiceActor: store}
	dir := t.TempDir()
	export := ExportTask{repository: repo, baseDir: dir, full: true}
	require.NoError(t, export.exportVoiceActors(ctx))
	files, err := os.ReadDir(filepath.Join(dir, "voice_actors"))
	require.NoError(t, err)
	require.Len(t, files, 2, "sanitised names must not overwrite each other")
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(dir, "voice_actors", file.Name()))
		require.NoError(t, err)
		var document map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &document))
		require.NotContains(t, document, "id")
	}
	store.actors = nil
	importTask := ImportTask{repository: repo, BaseDir: dir, DuplicateBehaviour: ImportDuplicateEnumFail}
	importTask.ImportVoiceActors(ctx)
	require.Len(t, store.actors, 2)
	actor, err := store.FindByName(ctx, "A/B")
	require.NoError(t, err)
	require.Equal(t, "Animation", actor.Details)
	require.Equal(t, []string{"Alias"}, actor.Aliases)
	require.True(t, actor.Favorite)
	require.NotEqual(t, 1, actor.ID)
	actor.Details = "Edited"
	importTask.DuplicateBehaviour = ImportDuplicateEnumIgnore
	importTask.ImportVoiceActors(ctx)
	require.Equal(t, "Edited", actor.Details)
	importTask.DuplicateBehaviour = ImportDuplicateEnumOverwrite
	importTask.ImportVoiceActors(ctx)
	actor, err = store.FindByName(ctx, "A/B")
	require.NoError(t, err)
	require.Equal(t, "Animation", actor.Details)
	require.Len(t, store.actors, 2)
}

type metadataTxnManager struct{}

func (*metadataTxnManager) Begin(ctx context.Context, _ bool) (context.Context, error) {
	return ctx, nil
}
func (*metadataTxnManager) Commit(context.Context) error   { return nil }
func (*metadataTxnManager) Rollback(context.Context) error { return nil }
func (*metadataTxnManager) IsLocked(error) bool            { return false }
func (*metadataTxnManager) WithDatabase(ctx context.Context) (context.Context, error) {
	return ctx, nil
}
