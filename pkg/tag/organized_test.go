package tag

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stretchr/testify/require"
)

func TestImportOrganized(t *testing.T) {
	for _, input := range []string{`{"name":"Old export"}`, `{"name":"New export","organized":false}`, `{"name":"New export","organized":true}`} {
		t.Run(input, func(t *testing.T) {
			var data jsonschema.Tag
			require.NoError(t, json.Unmarshal([]byte(input), &data))
			i := Importer{Input: data}
			require.NoError(t, i.PreImport(context.Background()))
			require.Equal(t, input == `{"name":"New export","organized":true}`, i.tag.Organized)
			exported, err := json.Marshal(data)
			require.NoError(t, err)
			if i.tag.Organized {
				require.Contains(t, string(exported), `"organized":true`)
			} else {
				require.NotContains(t, string(exported), `"organized"`)
			}
		})
	}
}
