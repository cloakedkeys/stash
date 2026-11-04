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
	changed, notes := ConvertLegacyWildcardDefaultFiltersMap(LegacySQLite, m)
	assert.True(t, changed)
	assert.Equal(t, []string{`default filter for performers: aliases: "excludes" on a field with several values can't be rewritten as a regular expression`}, notes)

	want := fileUIConfig("part_1")
	scenes := want["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
	scenes["object_filter"] = map[string]interface{}{"title": map[string]interface{}{"value": "(?is)(part.1)", "modifier": "MATCHES_REGEX"}}
	assert.Equal(t, want, m)

	t.Run("nothing to change", func(t *testing.T) {
		m := fileUIConfig("part-1")
		changed, _ := ConvertLegacyWildcardDefaultFiltersMap(LegacySQLite, m)
		assert.False(t, changed)
		assert.Equal(t, fileUIConfig("part-1"), m)
		changed, notes := ConvertLegacyWildcardDefaultFiltersMap(LegacySQLite, map[string]interface{}{"theme": "dark"})
		assert.False(t, changed)
		assert.Empty(t, notes)
		changed, _ = ConvertLegacyWildcardDefaultFiltersMap(LegacySQLite, nil)
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
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacySQLite, cfg, "105")

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

	t.Run("nothing to change still marks the file", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part-1"))
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacySQLite, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part-1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
		backups, err := filepath.Glob(cfg.GetConfigFile() + ".105.*")
		require.NoError(t, err)
		assert.Empty(t, backups)
		assert.True(t, cfg.GetLegacyWildcardsConverted())
		saved, err := os.ReadFile(cfg.GetConfigFile())
		require.NoError(t, err)
		assert.Regexp(t, `(?m)^legacy_wildcards_converted: true$`, string(saved))
	})

	t.Run("a marked config file is left alone", func(t *testing.T) {
		// default filters this build saved: part_1 means only part_1
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		cfg.SetBool(config.LegacyWildcardsConverted, true)
		require.NoError(t, cfg.Write())
		before, err := os.ReadFile(cfg.GetConfigFile())
		require.NoError(t, err)

		// as when a new database is created, or one is copied to PostgreSQL
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacyPostgres, cfg, "pg25")

		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
		after, err := os.ReadFile(cfg.GetConfigFile())
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
		backups, err := filepath.Glob(cfg.GetConfigFile() + ".pg25.*")
		require.NoError(t, err)
		assert.Empty(t, backups)
	})

	t.Run("converted once", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacySQLite, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "(?is)(part.1)", "modifier": "MATCHES_REGEX"}, titleFilter(t, cfg))

		// the marker is in the file, outside the ui map
		saved, err := os.ReadFile(cfg.GetConfigFile())
		require.NoError(t, err)
		assert.Regexp(t, `(?m)^legacy_wildcards_converted: true$`, string(saved))
		assert.True(t, cfg.GetLegacyWildcardsConverted())

		// the UI saves its whole ui map, with a new default filter that means
		// only part_1, and the server starts with another new database
		cfg.SetUIConfiguration(fileUIConfig("part_1"))
		require.NoError(t, cfg.Write())
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacyPostgres, cfg, "pg25")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
		backups, err := filepath.Glob(cfg.GetConfigFile() + ".*.*")
		require.NoError(t, err)
		assert.Len(t, backups, 1)
	})

	t.Run("skipped in the setup context", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		ConvertLegacyWildcardConfigDefaultFilters(models.WithSetupContext(ctx), LegacySQLite, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
	})

	t.Run("skipped without a config file", func(t *testing.T) {
		cfg := config.InitializeEmpty()
		cfg.SetUIConfiguration(fileUIConfig("part_1"))
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacySQLite, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
	})

	t.Run("a failed write keeps the config as it was", func(t *testing.T) {
		cfg := setupConfigFile(t, fileUIConfig("part_1"))
		// a directory where the file should be can't be written
		dir := filepath.Join(t.TempDir(), "config.yml")
		require.NoError(t, os.Mkdir(dir, 0755))
		cfg.SetConfigFile(dir)
		ConvertLegacyWildcardConfigDefaultFilters(ctx, LegacySQLite, cfg, "105")
		assert.Equal(t, map[string]interface{}{"value": "part_1", "modifier": "INCLUDES"}, titleFilter(t, cfg))
		assert.False(t, cfg.GetLegacyWildcardsConverted())
	})
}
