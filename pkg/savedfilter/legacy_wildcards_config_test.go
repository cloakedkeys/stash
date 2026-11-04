package savedfilter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a UI configuration as the config file gives it: plain maps, and numbers as
// YAML reads them
func fileUIConfig(titleValue string) map[string]interface{} {
	return map[string]interface{}{
		"theme":              "dark",
		"minimumPlayPercent": 50,
		"defaultFilters": map[string]interface{}{
			"scenes": map[string]interface{}{
				"mode":          "SCENES",
				"find_filter":   map[string]interface{}{"q": "", "per_page": 40},
				"object_filter": map[string]interface{}{"title": map[string]interface{}{"value": titleValue, "modifier": "INCLUDES"}},
			},
			"performers": map[string]interface{}{
				"object_filter": map[string]interface{}{"aliases": map[string]interface{}{"value": "a_", "modifier": "EXCLUDES"}},
			},
		},
	}
}

func TestConvertLegacyWildcardDefaultFiltersMap(t *testing.T) {
	m := fileUIConfig("part_1")
	changed, notes := ConvertLegacyWildcardDefaultFiltersMap(m)
	assert.True(t, changed)
	assert.Equal(t, []string{`default filter for performers: aliases: "excludes" on a field with several values can't be rewritten as a regular expression`}, notes)

	want := fileUIConfig("part_1")
	scenes := want["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	scenes["object_filter"] = map[string]interface{}{"title": map[string]interface{}{"value": "(?is)(part.1)", "modifier": "MATCHES_REGEX"}}
	assert.Equal(t, want, m)

	t.Run("nothing to change", func(t *testing.T) {
		m := fileUIConfig("part-1")
		changed, _ := ConvertLegacyWildcardDefaultFiltersMap(m)
		assert.False(t, changed)
		assert.Equal(t, fileUIConfig("part-1"), m)
		changed, notes := ConvertLegacyWildcardDefaultFiltersMap(map[string]interface{}{"theme": "dark"})
		assert.False(t, changed)
		assert.Empty(t, notes)
		changed, _ = ConvertLegacyWildcardDefaultFiltersMap(nil)
		assert.False(t, changed)
	})
}

func setupConfigFile(t *testing.T, ui map[string]interface{}) *config.Config {
	t.Helper()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetUIConfiguration(ui)
	require.NoError(t, cfg.Write())
	return cfg
}

func titleFilter(t *testing.T, cfg *config.Config) interface{} {
	t.Helper()
	ui := cfg.GetUIConfiguration()
	scenes := ui["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	return scenes["object_filter"].(map[string]interface{})["title"]
}

func TestConvertLegacyWildcardConfigDefaultFilters(t *testing.T) {
	ctx := context.Background()

	t.Run("rewrites and backs up", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		ConvertLegacyWildcardConfigDefaultFilters(ctx, cfg, "105")

		want := map[string]interface{}{"value": "(?is)(part.1)", "modifier": "MATCHES_REGEX"}
		assert.Equal(t, want, titleFilter(t, cfg))
		assert.Equal(t, "dark", cfg.GetUIConfiguration()["theme"])

		// saved to the file
		saved, err := os.ReadFile(cfg.GetConfigFile())
		require.NoError(t, err)
		assert.Contains(t, string(saved), "MATCHES_REGEX")
		assert.Contains(t, string(saved), "(?is)(part.1)")
		assert.NotContains(t, string(saved), "part_1")

		backups, err := filepath.Glob(cfg.GetConfigFile() + ".105.*")
		require.NoError(t, err)
		require.Len(t, backups, 1)
		backup, err := os.ReadFile(backups[0])
		require.NoError(t, err)
		assert.Contains(t, string(backup), "part_1")
		assert.NotContains(t, string(backup), "MATCHES_REGEX")
	})

	t.Run("nothing to change writes nothing", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part-1"))
		ConvertLegacyWildcardConfigDefaultFilters(ctx, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part-1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
		backups, err := filepath.Glob(cfg.GetConfigFile() + ".105.*")
		require.NoError(t, err)
		assert.Empty(t, backups)
	})

	t.Run("skipped in the setup context", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		ConvertLegacyWildcardConfigDefaultFilters(models.WithSetupContext(ctx), cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
	})

	t.Run("skipped without a config file", func(t *testing.T) {
		cfg := config.InitializeEmpty()
		cfg.SetUIConfiguration(fileUIConfig("part_1"))
		ConvertLegacyWildcardConfigDefaultFilters(ctx, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
	})

	t.Run("a failed write keeps the config as it was", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		// a directory where the file should be can't be written
		dir := filepath.Join(t.TempDir(), "config.yml")
		require.NoError(t, os.Mkdir(dir, 0755))
		cfg.SetConfigFile(dir)
		ConvertLegacyWildcardConfigDefaultFilters(ctx, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
	})
}
