package migrations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration58IsSnakeCaseConfig(t *testing.T) {
	m := schema58Migrator{}
	for _, c := range []struct {
		name string
		ui   map[string]interface{}
		want bool
	}{
		{"old", map[string]interface{}{"show_studio_as_text": true, "front_page_content": []interface{}{map[string]interface{}{"savedfilterid": 1}}}, true},
		{"current", map[string]interface{}{"defaultFilters": map[string]interface{}{"scenes": map[string]interface{}{"object_filter": map[string]interface{}{}}}}, false},
		{"converted", map[string]interface{}{"showStudioAsText": true}, false},
		// nothing to convert
		{"one plain key", map[string]interface{}{"lang": "en"}, false},
		{"leading underscore only", map[string]interface{}{"__typename": "x"}, false},
		// an uppercase letter in a list is enough
		{"uppercase in a list", map[string]interface{}{"front_page_content": []interface{}{map[string]interface{}{"savedFilterId": 1}}}, false},
		// as the YAML parser of the time could return it
		{"yaml map", map[string]interface{}{"task_defaults": map[interface{}]interface{}{"scan": true}}, true},
	} {
		assert.Equal(t, c.want, m.isSnakeCaseConfig(c.ui), c.name)
	}
}

func TestMigration58PluginKeys(t *testing.T) {
	m := schema58Migrator{}
	for _, c := range []struct {
		name      string
		key       string
		installed []string
		want      []string
	}{
		{"not installed", "my_plugin", nil, []string{"myPlugin"}},
		{"camelCase installed", "my_plugin", []string{"myPlugin"}, []string{"myPlugin"}},
		{"snake_case installed", "my_plugin", []string{"my_plugin"}, []string{"my_plugin"}},
		{"both installed", "my_plugin", []string{"my_plugin", "myPlugin"}, []string{"myPlugin", "my_plugin"}},
		{"other plugin installed", "my_plugin", []string{"other_plugin"}, []string{"myPlugin"}},
		{"no _", "plugin", []string{"plugin"}, []string{"plugin"}},
		// mixed case, as saved through toSnakeCase
		{"mixed case installed", "my_plugin", []string{"My_plugin"}, []string{"My_plugin"}},
		{"mixed case saved with __", "my__plugin", []string{"My_Plugin"}, []string{"My_Plugin"}},
		{"My_Plugin was not saved as my_plugin", "my_plugin", []string{"My_Plugin"}, []string{"myPlugin"}},
		{"two spellings installed", "my_plugin", []string{"My_plugin", "my_plugin"}, []string{"My_plugin", "my_plugin"}},
		{"three spellings installed", "my_plugin", []string{"My_plugin", "my_plugin", "myPlugin"}, []string{"My_plugin", "myPlugin", "my_plugin"}},
		// an ID without _ is converted as before
		{"PascalCase installed", "my_plugin", []string{"MyPlugin"}, []string{"myPlugin"}},
	} {
		installed := make(map[string]bool)
		for _, id := range c.installed {
			installed[id] = true
		}
		assert.Equal(t, c.want, m.pluginKeys(c.key, installed), c.name)
	}
}

func TestMigration58ToSnakeCase(t *testing.T) {
	m := schema58Migrator{}
	for id, want := range map[string]string{
		"myPlugin":  "my_plugin",
		"my_plugin": "my_plugin",
		"My_plugin": "my_plugin",
		"My_Plugin": "my__plugin",
		"someIDs":   "some_ids",
	} {
		assert.Equal(t, want, m.toSnakeCase(id), id)
	}
}

func TestMigration58InstalledPluginIDs(t *testing.T) {
	m := schema58Migrator{}
	dir := t.TempDir()
	for _, f := range []string{"top_plugin.yml", "pkg/my_plugin.yml", "pkg/deep/myPlugin.yml", "pkg/readme.md", "pkg/data.yaml"} {
		fp := filepath.Join(dir, filepath.FromSlash(f))
		require.NoError(t, os.MkdirAll(filepath.Dir(fp), 0755))
		require.NoError(t, os.WriteFile(fp, []byte("name: x\n"), 0644))
	}
	// a folder is not a plugin file
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "folder.yml"), 0755))

	ids, err := m.installedPluginIDs(dir)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"top_plugin": true, "my_plugin": true, "myPlugin": true}, ids)

	// a missing folder has no plugins
	ids, err = m.installedPluginIDs(filepath.Join(dir, "missing"))
	require.NoError(t, err)
	assert.Empty(t, ids)

	// a folder that can't be read is an error, so every ID is converted
	_, err = m.installedPluginIDs(filepath.Join(dir, "bad\x00"))
	assert.Error(t, err)
	_, err = m.installedPluginIDs("")
	assert.Error(t, err)
}
