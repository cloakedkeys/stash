package sqlite

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/require"
)

// Exercise the feature migration directly so parallel, reserved migrations are
// not needed to test voice credits against the pre-existing scene tables.
func voiceActorTestContext(t *testing.T) context.Context {
	t.Helper()
	db, err := sqlx.Open(sqlite3Driver, ":memory:?_fk=true")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`CREATE TABLE scenes (id INTEGER PRIMARY KEY);
CREATE TABLE performers (id INTEGER PRIMARY KEY);
CREATE TABLE performers_scenes (scene_id INTEGER REFERENCES scenes(id) ON DELETE CASCADE, performer_id INTEGER REFERENCES performers(id) ON DELETE CASCADE);
INSERT INTO scenes VALUES (1),(2); INSERT INTO performers VALUES (1); INSERT INTO performers_scenes VALUES (1,1);`)
	require.NoError(t, err)
	migration, err := migrationsBox.ReadFile("migrations/96_voice_actors.up.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	tx, err := db.Beginx()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	return context.WithValue(context.Background(), txnKey, tx)
}

func TestVoiceActorStore(t *testing.T) {
	ctx := voiceActorTestContext(t)
	store := &VoiceActorStore{}
	urls := []string{"https://example.com/Voice", "https://example.com/voice"}
	actor := &models.VoiceActor{Name: " Voice Artist ", Details: "ADR and animation", Aliases: []string{"Alias", "ALIAS", " "}, URLs: urls, Favorite: true}
	require.NoError(t, store.Create(ctx, actor))
	require.Positive(t, actor.ID)
	found, err := store.FindByName(ctx, "voice artist")
	require.NoError(t, err)
	require.Equal(t, "Voice Artist", found.Name)
	require.Equal(t, []string{"Alias"}, found.Aliases)
	require.Equal(t, urls, found.URLs, "URL paths can be case-sensitive")
	require.False(t, found.CreatedAt.IsZero())
	q := "Alias"
	yes := true
	actors, count, err := store.Query(ctx, &models.FindFilterType{Q: &q}, &yes)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Len(t, actors, 1)
	require.NoError(t, store.SetScene(ctx, 1, []int{actor.ID, actor.ID}))
	credits, err := store.FindBySceneID(ctx, 1)
	require.NoError(t, err)
	require.Len(t, credits, 1)
	scenes, err := store.SceneIDs(ctx, actor.ID)
	require.NoError(t, err)
	require.Equal(t, []int{1}, scenes)
	found.Name = "Renamed"
	found.Aliases = []string{}
	found.Favorite = false
	require.NoError(t, store.Update(ctx, found))
	actors, count, err = store.Query(ctx, nil, &yes)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, actors)
	credits, err = store.FindBySceneID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "Renamed", credits[0].Name)
	require.NoError(t, store.SetScene(ctx, 1, nil))
	scenes, err = store.SceneIDs(ctx, actor.ID)
	require.NoError(t, err)
	require.Empty(t, scenes)
	require.NoError(t, store.SetScene(ctx, 1, []int{actor.ID}))
	require.NoError(t, store.Destroy(ctx, actor.ID))
	credits, err = store.FindBySceneID(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, credits)
	var performers int
	require.NoError(t, dbWrapper.Get(ctx, &performers, "SELECT COUNT(*) FROM performers_scenes WHERE scene_id=1"))
	require.Equal(t, 1, performers, "voice credits must not alter performer credits")
}

func TestVoiceActorConstraintsAndCascade(t *testing.T) {
	ctx := voiceActorTestContext(t)
	store := &VoiceActorStore{}
	require.Error(t, store.Create(ctx, &models.VoiceActor{Name: " "}))
	a := &models.VoiceActor{Name: "Artist"}
	require.NoError(t, store.Create(ctx, a))
	require.Error(t, store.Create(ctx, &models.VoiceActor{Name: "ARTIST"}))
	require.Error(t, store.SetScene(ctx, 2, []int{9999}), "unknown voice actor must not produce a dangling credit")
	require.NoError(t, store.SetScene(ctx, 2, []int{a.ID}))
	_, err := dbWrapper.Exec(ctx, "DELETE FROM scenes WHERE id=2")
	require.NoError(t, err)
	ids, err := store.SceneIDs(ctx, a.ID)
	require.NoError(t, err)
	require.Empty(t, ids)
	found, err := store.Find(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, found, "deleting a scene must retain the voice actor")
}

func TestVoiceActorCreditsSurviveSceneMerge(t *testing.T) {
	ctx := voiceActorTestContext(t)
	store := &VoiceActorStore{}
	a, b := &models.VoiceActor{Name: "A"}, &models.VoiceActor{Name: "B"}
	require.NoError(t, store.Create(ctx, a))
	require.NoError(t, store.Create(ctx, b))
	require.NoError(t, store.SetScene(ctx, 1, []int{a.ID}))
	require.NoError(t, store.SetScene(ctx, 2, []int{a.ID, b.ID}))
	require.NoError(t, store.MergeScenes(ctx, 1, []int{2}))
	_, err := dbWrapper.Exec(ctx, "DELETE FROM scenes WHERE id=2")
	require.NoError(t, err)
	actors, err := store.FindBySceneID(ctx, 1)
	require.NoError(t, err)
	require.Len(t, actors, 2)
}
