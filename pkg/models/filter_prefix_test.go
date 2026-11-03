package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStringCriterionPrefixIsNotSaved(t *testing.T) {
	criterion := StringCriterionInput{
		Value:       "my_dir/",
		Modifier:    CriterionModifierEquals,
		MatchPrefix: true,
	}
	data, err := json.Marshal(criterion)
	require.NoError(t, err)
	require.JSONEq(t, `{"value":"my_dir/","modifier":"EQUALS"}`, string(data))

	var restored StringCriterionInput
	require.NoError(t, json.Unmarshal(data, &restored))
	require.False(t, restored.MatchPrefix)
	require.Equal(t, criterion.Value, restored.Value)
	require.Equal(t, criterion.Modifier, restored.Modifier)
}
