//go:build integration

package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schema58CurrentUI is a UI configuration as this build saves it. The default
// filters keep the keys of a saved filter, which use _.
func schema58CurrentUI() map[string]interface{} {
	return map[string]interface{}{
		"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{
			"mode":          "SCENES",
			"find_filter":   map[string]interface{}{"per_page": 40, "sort": "date"},
			"object_filter": map[string]interface{}{"rating100": map[string]interface{}{"value": 60, "modifier": "GREATER_THAN"}},
		}},
		"frontPageContent": []interface{}{map[string]interface{}{"__typename": "SavedFilter", "savedFilterId": 1}},
	}
}

// schema58OldUI is a UI configuration as it was saved before schema 58: in
// snake_case, then made lowercase throughout by the config library of the time,
// which is why the front page keys need renaming.
func schema58OldUI() map[string]interface{} {
	return map[string]interface{}{
		"show_studio_as_text": true,
		"front_page_content":  []interface{}{map[string]interface{}{"__typename": "SavedFilter", "savedfilterid": 1, "sortby": "date"}},
	}
}

// schema58ConvertedUI is schema58OldUI after step 58.
func schema58ConvertedUI() map[string]interface{} {
	return map[string]interface{}{
		"showStudioAsText": true,
		"frontPageContent": []interface{}{map[string]interface{}{"__typename": "SavedFilter", "savedFilterId": 1, "sortBy": "date"}},
	}
}

func schema58Plugins() map[string]interface{} {
	return map[string]interface{}{"my_plugin": map[string]interface{}{"skip_existing": true}}
}

var (
	schema58PluginsKept      = map[string]map[string]interface{}{"my_plugin": {"skip_existing": true}}
	schema58PluginsConverted = map[string]map[string]interface{}{"myPlugin": {"skipExisting": true}}
)

// schema58Config sets up a config file with the UI settings and the plugin settings.
func schema58Config(t *testing.T, dir string, ui map[string]interface{}) (*config.Config, string) {
	t.Helper()
	cfg := config.InitializeEmpty()
	configFile := filepath.Join(dir, "config.yml")
	cfg.SetConfigFile(configFile)
	cfg.SetUIConfiguration(ui)
	cfg.SetInterface(config.PluginsSetting, schema58Plugins())
	// a config file this build wrote already has its default filters converted
	cfg.SetBool(config.LegacyWildcardsConverted, true)
	require.NoError(t, cfg.Write())
	return cfg, configFile
}

func schema58Backups(t *testing.T, configFile string) []string {
	t.Helper()
	backups, err := filepath.Glob(configFile + ".57.*")
	require.NoError(t, err)
	return backups
}

// schema58NewDatabase creates a new database, which runs every step from the start.
func schema58NewDatabase(ctx context.Context, t *testing.T, dir string) {
	t.Helper()
	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, filepath.Join(dir, "stash.sqlite")))
	require.NoError(t, db.Close())
}

// schema58OldDatabase makes a database at schema 57, ready for step 58, as the
// migrate task upgrades it.
func schema58OldDatabase(ctx context.Context, t *testing.T, dir string) *sqlite.Migrator {
	t.Helper()
	// the earlier steps read the config, so a test can run on its own
	config.InitializeEmpty()
	dbPath := filepath.Join(dir, "stash.sqlite")
	raw, err := sqlx.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	initial, err := os.ReadFile("1_initial.up.sql")
	require.NoError(t, err)
	_, err = raw.Exec(string(initial) + "\nCREATE TABLE schema_migrations(version uint64, dirty bool); INSERT INTO schema_migrations VALUES(1,false);")
	require.NoError(t, err)

	db := sqlite.NewDatabase()
	require.NoError(t, db.Open(ctx, dbPath))
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	m, err := sqlite.NewMigrator(db)
	require.NoError(t, err)
	t.Cleanup(m.Close)
	for version := uint(2); version <= 57; version++ {
		require.NoErrorf(t, m.RunMigration(ctx, version), "schema %d", version)
	}
	return m
}

func assertSchema58Kept(t *testing.T, cfg *config.Config, configFile string) {
	t.Helper()
	assert.Equal(t, schema58CurrentUI(), cfg.GetUIConfiguration())
	assert.Equal(t, schema58PluginsKept, cfg.GetAllPluginConfiguration())
	assert.Empty(t, schema58Backups(t, configFile))

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	for _, key := range []string{"object_filter:", "find_filter:", "per_page:", "my_plugin:", "skip_existing:"} {
		assert.Contains(t, string(data), key)
	}
	assert.NotContains(t, string(data), "objectFilter")
	assert.NotContains(t, string(data), "myPlugin")
}

func assertSchema58Converted(t *testing.T, cfg *config.Config, configFile string) {
	t.Helper()
	assert.Equal(t, schema58ConvertedUI(), cfg.GetUIConfiguration())
	assert.Equal(t, schema58PluginsConverted, cfg.GetAllPluginConfiguration())
	assert.Len(t, schema58Backups(t, configFile), 1)
}

// A new database leaves a config file this build wrote as it was.
func TestMigration58NewDatabaseCurrentConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg, configFile := schema58Config(t, dir, schema58CurrentUI())
	schema58NewDatabase(ctx, t, dir)
	assertSchema58Kept(t, cfg, configFile)
}

// A new database with a config file from before schema 58 converts it, as
// when the database of an old install is deleted.
func TestMigration58NewDatabaseOldConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg, configFile := schema58Config(t, dir, schema58OldUI())
	schema58NewDatabase(ctx, t, dir)
	assertSchema58Converted(t, cfg, configFile)
}

// Upgrading a restored old database leaves a config file this build wrote as it was.
func TestMigration58OldDatabaseCurrentConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	// after the earlier steps, which write the config file too
	cfg, configFile := schema58Config(t, dir, schema58CurrentUI())
	require.NoError(t, m.RunMigration(ctx, 58))
	assertSchema58Kept(t, cfg, configFile)
}

// Upgrading an old database with its old config file converts the config file.
func TestMigration58OldDatabaseOldConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, configFile := schema58Config(t, dir, schema58OldUI())
	require.NoError(t, m.RunMigration(ctx, 58))
	assertSchema58Converted(t, cfg, configFile)
}

// schema58InstallPlugins sets the plugin folder of cfg to dir/plugins and puts
// a plugin file in it for each ID, in a subfolder as a plugin package installs it.
func schema58InstallPlugins(t *testing.T, cfg *config.Config, dir string, ids ...string) {
	t.Helper()
	pluginsPath := filepath.Join(dir, "plugins")
	for _, id := range ids {
		pluginDir := filepath.Join(pluginsPath, id)
		require.NoError(t, os.MkdirAll(pluginDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, id+".yml"), []byte("name: "+id+"\n"), 0644))
	}
	cfg.SetInterface(config.PluginsPath, pluginsPath)
	require.NoError(t, cfg.Write())
}

// An installed plugin whose ID has an _ keeps the settings saved under its ID.
func TestMigration58OldConfigSnakeCasePluginKept(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, configFile := schema58Config(t, dir, schema58OldUI())
	schema58InstallPlugins(t, cfg, dir, "my_plugin")
	require.NoError(t, m.RunMigration(ctx, 58))

	assert.Equal(t, schema58ConvertedUI(), cfg.GetUIConfiguration())
	// the setting names are converted as before: the plugin read them in camelCase
	assert.Equal(t, map[string]map[string]interface{}{"my_plugin": {"skipExisting": true}}, cfg.GetAllPluginConfiguration())
	assert.Len(t, schema58Backups(t, configFile), 1)
}

// An installed plugin with a camelCase ID gets its settings back as before.
func TestMigration58OldConfigCamelCasePluginConverted(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, configFile := schema58Config(t, dir, schema58OldUI())
	schema58InstallPlugins(t, cfg, dir, "myPlugin", "other_plugin")
	require.NoError(t, m.RunMigration(ctx, 58))
	assertSchema58Converted(t, cfg, configFile)
}

// myPlugin and my_plugin both read the settings saved as my_plugin, so when
// both are installed each gets a copy.
func TestMigration58OldConfigBothPluginForms(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, _ := schema58Config(t, dir, schema58OldUI())
	schema58InstallPlugins(t, cfg, dir, "myPlugin", "my_plugin")
	require.NoError(t, m.RunMigration(ctx, 58))

	assert.Equal(t, map[string]map[string]interface{}{
		"my_plugin": {"skipExisting": true},
		"myPlugin":  {"skipExisting": true},
	}, cfg.GetAllPluginConfiguration())
}

// Plugins with uppercase letters and an _ in their ID get the settings saved
// under their ID in snake_case, with the ID spelled as installed.
func TestMigration58OldConfigMixedCasePlugins(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, _ := schema58Config(t, dir, schema58OldUI())
	// as saved for My_plugin and My_Plugin before schema 58
	cfg.SetInterface(config.PluginsSetting, map[string]interface{}{
		"my_plugin":    map[string]interface{}{"skip_existing": true},
		"other__thing": map[string]interface{}{"dry_run": true},
	})
	schema58InstallPlugins(t, cfg, dir, "My_plugin", "Other_Thing")
	require.NoError(t, m.RunMigration(ctx, 58))

	assert.Equal(t, map[string]map[string]interface{}{
		"My_plugin":   {"skipExisting": true},
		"Other_Thing": {"dryRun": true},
	}, cfg.GetAllPluginConfiguration())
}

// my_plugin and my__plugin (My_Plugin) both convert to myPlugin, which gets
// the settings it read before, saved as my_plugin, every time.
func TestMigration58OldConfigConvertedKeysCollide(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, _ := schema58Config(t, dir, schema58OldUI())
	// ten pairs, so that map order can't pass the test by chance
	saved := make(map[string]interface{})
	want := make(map[string]map[string]interface{})
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("plugin%d", i)
		saved["my_"+name] = map[string]interface{}{"from": "my_" + name}
		saved["my__"+name] = map[string]interface{}{"from": "my__" + name}
		want["my"+strings.ToUpper(name[:1])+name[1:]] = map[string]interface{}{"from": "my_" + name}
	}
	cfg.SetInterface(config.PluginsSetting, saved)
	require.NoError(t, cfg.Write())
	require.NoError(t, m.RunMigration(ctx, 58))

	assert.Equal(t, want, cfg.GetAllPluginConfiguration())
}

// When the plugin folder can't be read, every plugin ID is converted, as before.
func TestMigration58OldConfigUnreadablePluginFolder(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := schema58OldDatabase(ctx, t, dir)
	cfg, configFile := schema58Config(t, dir, schema58OldUI())
	// a NUL in the path fails on every platform
	cfg.SetInterface(config.PluginsPath, filepath.Join(dir, "plugins\x00"))
	require.NoError(t, cfg.Write())
	require.NoError(t, m.RunMigration(ctx, 58))
	assertSchema58Converted(t, cfg, configFile)
}

// 106 rewrites the default filters of a config file without its marker once,
// on a new database too, now that 58 no longer renames their keys first.
func TestMigration106NewDatabaseConfigFileOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
	saveDefault := func() {
		t.Helper()
		cfg.SetUIConfiguration(map[string]interface{}{"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{
			"mode":          "SCENES",
			"object_filter": map[string]interface{}{"title": map[string]interface{}{"value": "a_b", "modifier": "EQUALS"}},
		}}})
		require.NoError(t, cfg.Write())
	}
	defaultTitle := func() interface{} {
		t.Helper()
		scenes := cfg.GetUIConfiguration()["defaultFilters"].(map[string]interface{})["scenes"].(map[string]interface{})
		objectFilter, ok := scenes["object_filter"].(map[string]interface{})
		require.True(t, ok, "object_filter is gone: %v", scenes)
		return objectFilter["title"]
	}
	openNewDatabase := func(name string) {
		t.Helper()
		db := sqlite.NewDatabase()
		require.NoError(t, db.Open(ctx, filepath.Join(dir, name)))
		require.NoError(t, db.Close())
	}

	saveDefault()
	openNewDatabase("first.sqlite")
	assert.Equal(t, map[string]interface{}{"value": "(?is)^a.b$", "modifier": "MATCHES_REGEX"}, defaultTitle())
	assert.True(t, cfg.GetLegacyWildcardsConverted())

	// a default filter this build saves, then another new database
	saveDefault()
	openNewDatabase("second.sqlite")
	assert.Equal(t, map[string]interface{}{"value": "a_b", "modifier": "EQUALS"}, defaultTitle())
}
