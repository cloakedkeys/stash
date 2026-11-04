package savedfilter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// an export file as v0.32.1 and upstream Stash write it: no literal_wildcards
const oldExport = `{
  "mode": "SCENES",
  "name": "parts",
  "find_filter": {"q": "x_y", "page": 1, "per_page": 40, "sort": "title", "direction": "ASC"},
  "object_filter": {
    "title": {"value": "part_1", "modifier": "INCLUDES"},
    "url": {"value": "site_x", "modifier": "EXCLUDES"},
    "resolution": {"value": "FULL_HD", "modifier": "EQUALS"},
    "performers_filter": {"name": {"value": "a%", "modifier": "EQUALS"}}
  },
  "ui_options": {"display_mode": 0, "zoom_index": 1}
}`

func loadExport(t *testing.T, data string) *jsonschema.SavedFilter {
	t.Helper()
	path := filepath.Join(t.TempDir(), "filter.json")
	require.NoError(t, os.WriteFile(path, []byte(data), 0644))
	f, err := jsonschema.LoadSavedFilterFile(path)
	require.NoError(t, err)
	return f
}

func TestImportOldExportRewritesWildcards(t *testing.T) {
	input := loadExport(t, oldExport)
	assert.False(t, input.LiteralWildcards)

	i := Importer{Input: *input}
	require.NoError(t, i.PreImport(testCtx))

	assert.Equal(t, map[string]interface{}{
		"title":             map[string]interface{}{"value": "(?is)(part.1)", "modifier": "MATCHES_REGEX"},
		"url":               map[string]interface{}{"value": "site_x", "modifier": "EXCLUDES"},
		"resolution":        map[string]interface{}{"value": "FULL_HD", "modifier": "EQUALS"},
		"performers_filter": map[string]interface{}{"name": map[string]interface{}{"value": "(?is)^a.*$", "modifier": "MATCHES_REGEX"}},
	}, i.savedFilter.ObjectFilter)
	// the rest comes in as it was
	assert.Equal(t, "parts", i.savedFilter.Name)
	assert.Equal(t, models.FilterModeScenes, i.savedFilter.Mode)
	require.NotNil(t, i.savedFilter.FindFilter)
	assert.Equal(t, "x_y", *i.savedFilter.FindFilter.Q)
	assert.Equal(t, map[string]interface{}{"display_mode": float64(0), "zoom_index": float64(1)}, i.savedFilter.UIOptions)
}

// An export doesn't say which database it came from, so it's read as SQLite,
// and upstream Stash, did: case is ignored only for A to Z.
func TestImportReadsFiltersAsSQLiteDid(t *testing.T) {
	input := loadExport(t, `{"mode":"SCENES","name":"accents","object_filter":{"title":{"value":"été_a","modifier":"INCLUDES"}}}`)
	i := Importer{Input: *input}
	require.NoError(t, i.PreImport(testCtx))
	assert.Equal(t, map[string]interface{}{
		"title": map[string]interface{}{"value": "(?is)((?-i:é)t(?-i:é).a)", "modifier": "MATCHES_REGEX"},
	}, i.savedFilter.ObjectFilter)
}

func TestImportNewExportIsUnchanged(t *testing.T) {
	q := "x_y"
	original := models.SavedFilter{
		Name:       "parts",
		Mode:       models.FilterModeScenes,
		FindFilter: &models.FindFilterType{Q: &q},
		ObjectFilter: map[string]interface{}{
			// "part_1" here already means only part_1
			"title": map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"},
		},
		UIOptions: map[string]interface{}{"display_mode": float64(0)},
	}

	exported, err := ToJSON(testCtx, &original)
	require.NoError(t, err)
	assert.True(t, exported.LiteralWildcards)

	path := filepath.Join(t.TempDir(), exported.Filename())
	require.NoError(t, jsonschema.SaveSavedFilterFile(path, exported))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"literal_wildcards": true`)

	input, err := jsonschema.LoadSavedFilterFile(path)
	require.NoError(t, err)
	i := Importer{Input: *input}
	require.NoError(t, i.PreImport(testCtx))
	assert.Equal(t, original, i.savedFilter)
}
