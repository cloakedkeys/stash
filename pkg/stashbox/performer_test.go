package stashbox

import (
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/stashbox/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformerFragmentAliases(t *testing.T) {
	want := []string{"Alice", "Alice, Alina"}
	scraped := performerFragmentToScrapedPerformer(graphql.PerformerFragment{
		Name:    "Jane",
		Aliases: []string{"Alice", "Alice, Alina", "alice", "JANE"},
	})

	// Both the API response and backend creation/update must preserve boundaries.
	data, err := json.Marshal(scraped)
	require.NoError(t, err)
	var response struct {
		AliasList []string `json:"alias_list"`
	}
	require.NoError(t, json.Unmarshal(data, &response))
	assert.Equal(t, want, response.AliasList)
	assert.Equal(t, "Alice, Alice, Alina", *scraped.Aliases)
	assert.Equal(t, want, scraped.ToPerformer("", nil).Aliases.List())
	partial := scraped.ToPartial("", nil, nil, nil)
	require.NotNil(t, partial.Aliases)
	assert.Equal(t, want, partial.Aliases.Values)
}
