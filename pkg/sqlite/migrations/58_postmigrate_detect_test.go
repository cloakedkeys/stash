package migrations

import (
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
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

// schema58PluginFolder makes a plugin folder holding my_plugin/my_plugin.yml.
func schema58PluginFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "my_plugin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my_plugin", "my_plugin.yml"), []byte("name: x\n"), 0644))
	return dir
}

func assertSchema58FindsMyPlugin(t *testing.T, dir string) {
	t.Helper()
	ids, err := (&schema58Migrator{}).installedPluginIDs(dir)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"my_plugin": true}, ids)
}

// A subfolder that can't be read is skipped, as pkg/plugin skips it.
func TestMigration58InstalledPluginIDsLockedFolder(t *testing.T) {
	dir := schema58PluginFolder(t)
	locked := filepath.Join(dir, "locked")
	require.NoError(t, os.MkdirAll(locked, 0755))
	if runtime.GOOS == "windows" {
		u, err := user.Current()
		require.NoError(t, err)
		if out, err := exec.Command("icacls", locked, "/deny", u.Username+":(OI)(CI)(RD)").CombinedOutput(); err != nil {
			t.Skipf("icacls: %v %s", err, out)
		}
		t.Cleanup(func() { _ = exec.Command("icacls", locked, "/remove:d", u.Username).Run() })
	} else {
		require.NoError(t, os.Chmod(locked, 0))
		t.Cleanup(func() { _ = os.Chmod(locked, 0755) })
	}
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("the folder can still be read, as by root")
	}

	assertSchema58FindsMyPlugin(t, dir)
}

// A broken link, as in a Python plugin's virtualenv, is skipped.
func TestMigration58InstalledPluginIDsBrokenLink(t *testing.T) {
	dir := schema58PluginFolder(t)
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "my_plugin", "broken")); err != nil {
		t.Skipf("can't make a link: %v", err)
	}

	assertSchema58FindsMyPlugin(t, dir)
}

// The same, without OS permissions: entries that fail to read, as the walk
// reports them.
func TestMigration58InstalledPluginIDsReadErrors(t *testing.T) {
	dir := schema58PluginFolder(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locked"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locked", "hidden.yml"), []byte("name: x\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my_plugin", "broken"), nil, 0644))

	walk := schema58WalkPlugins
	t.Cleanup(func() { schema58WalkPlugins = walk })
	schema58WalkPlugins = func(path string, walkFn filepath.WalkFunc) error {
		return walk(path, func(fp string, info os.FileInfo, err error) error {
			switch filepath.Base(fp) {
			case "locked":
				// a folder that can't be listed, so its files go unseen
				if info.IsDir() {
					return walkFn(fp, info, fs.ErrPermission)
				}
			case "broken":
				// a link that can't be followed
				return walkFn(fp, info, fs.ErrNotExist)
			}
			return walkFn(fp, info, err)
		})
	}

	assertSchema58FindsMyPlugin(t, dir)

	// the plugin folder itself is still an error
	schema58WalkPlugins = func(path string, walkFn filepath.WalkFunc) error {
		return walkFn(path, nil, fs.ErrPermission)
	}
	_, err := (&schema58Migrator{}).installedPluginIDs(dir)
	assert.ErrorIs(t, err, fs.ErrPermission)
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
