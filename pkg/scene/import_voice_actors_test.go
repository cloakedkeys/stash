package scene

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stretchr/testify/require"
)

type importVoiceActorStore struct {
	models.VoiceActorReaderWriter
	actors  map[string]*models.VoiceActor
	sceneID int
	credits []int
}

func (s *importVoiceActorStore) SetScene(_ context.Context, id int, ids []int) error {
	s.sceneID = id
	s.credits = ids
	return nil
}

func (s *importVoiceActorStore) FindByName(_ context.Context, name string) (*models.VoiceActor, error) {
	return s.actors[name], nil
}
func (s *importVoiceActorStore) Create(_ context.Context, a *models.VoiceActor) error {
	a.ID = len(s.actors) + 1
	s.actors[a.Name] = a
	return nil
}

func TestVoiceActorImportMissingReferences(t *testing.T) {
	for _, behavior := range []models.ImportMissingRefEnum{models.ImportMissingRefEnumFail, models.ImportMissingRefEnumIgnore, models.ImportMissingRefEnumCreate} {
		t.Run(string(behavior), func(t *testing.T) {
			store := &importVoiceActorStore{actors: map[string]*models.VoiceActor{"Known": {ID: 5, Name: "Known"}}}
			i := Importer{VoiceActorWriter: store, Input: jsonschema.Scene{VoiceActors: []string{"Known", "Missing"}}, MissingRefBehaviour: behavior}
			err := i.populateVoiceActors(context.Background())
			if behavior == models.ImportMissingRefEnumFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, i.PostImport(context.Background(), 7))
			require.Equal(t, 7, store.sceneID)
			require.Equal(t, i.voiceActorIDs, store.credits)
			if behavior == models.ImportMissingRefEnumIgnore {
				require.Equal(t, []int{5}, i.voiceActorIDs)
				require.Len(t, store.actors, 1)
			} else {
				require.Len(t, i.voiceActorIDs, 2)
				require.Contains(t, store.actors, "Missing")
			}
		})
	}
}

func TestVoiceActorImportClearsExplicitEmptyCredits(t *testing.T) {
	ctx := context.Background()
	store := &importVoiceActorStore{actors: map[string]*models.VoiceActor{}, credits: []int{5}}
	i := Importer{VoiceActorWriter: store, Input: jsonschema.Scene{VoiceActors: []string{}}}
	require.NoError(t, i.populateVoiceActors(ctx))
	require.NoError(t, i.PostImport(ctx, 7))
	require.Equal(t, 7, store.sceneID)
	require.Empty(t, store.credits)
	// Older metadata without the field preserves credits and needs no repository.
	i = Importer{}
	require.NoError(t, i.PostImport(ctx, 7))
}
