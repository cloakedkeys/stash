package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVoiceActorSceneMetadataEmptyCredits(t *testing.T) {
	data, err := json.Marshal(Scene{VoiceActors: []string{}})
	require.NoError(t, err)
	var imported Scene
	require.NoError(t, json.Unmarshal(data, &imported))
	require.NotNil(t, imported.VoiceActors, "an exported empty credit list must clear existing credits when imported")
	require.Empty(t, imported.VoiceActors)
	var older Scene
	require.NoError(t, json.Unmarshal([]byte(`{"title":"Older archive"}`), &older))
	require.Nil(t, older.VoiceActors, "older archives must not erase credits that they cannot represent")
}
