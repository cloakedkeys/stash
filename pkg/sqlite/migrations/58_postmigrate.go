package migrations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cast"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/utils"
)

type schema58Migrator struct {
	migrator
}

func post58(ctx context.Context, db *sqlx.DB) error {
	logger.Info("Running post-migration for schema version 58")

	m := schema58Migrator{
		migrator: migrator{
			db: db,
		},
	}

	return m.migrate()
}

func (m *schema58Migrator) migrate() error {
	if err := m.migrateConfig(); err != nil {
		return fmt.Errorf("failed to migrate config: %w", err)
	}

	return nil
}

// fromSnakeCase converts a string from snake_case to camelCase
func (m *schema58Migrator) fromSnakeCase(v string) string {
	var buf bytes.Buffer
	leadingUnderscore := true
	capvar := false
	for i, c := range v {
		switch {
		case c == '_' && !leadingUnderscore && i > 0:
			capvar = true
		case c == '_' && leadingUnderscore:
			buf.WriteRune(c)
		case capvar:
			buf.WriteRune(unicode.ToUpper(c))
			capvar = false
		default:
			leadingUnderscore = false
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

// fromSnakeCaseMap recursively converts a map using snake_case keys to camelCase keys
func (m *schema58Migrator) fromSnakeCaseMap(mm map[string]interface{}) map[string]interface{} {
	return m.fromSnakeCaseValue(mm).(map[string]interface{})
}

func (m *schema58Migrator) fromSnakeCaseValue(val interface{}) interface{} {
	switch v := val.(type) {
	case map[interface{}]interface{}:
		ret := cast.ToStringMap(v)
		for k, vv := range ret {
			adjKey := m.fromSnakeCase(k)
			ret[adjKey] = m.fromSnakeCaseValue(vv)
		}
		return ret
	case map[string]interface{}:
		ret := make(map[string]interface{})
		for k, vv := range v {
			adjKey := m.fromSnakeCase(k)
			ret[adjKey] = m.fromSnakeCaseValue(vv)
		}
		return ret
	case []interface{}:
		ret := make([]interface{}, len(v))
		for i, vv := range v {
			ret[i] = m.fromSnakeCaseValue(vv)
		}
		return ret
	default:
		return v
	}
}

// isSnakeCaseConfig reports whether a UI configuration was saved before schema
// 58. The config library used then made every key lowercase, at any depth, and
// the UI settings were saved with snake_case keys to survive that. A config
// file saved since has camelCase keys, such as defaultFilters. So the settings
// are old when no key has an uppercase letter and at least one key has an _
// after its first character.
func (m *schema58Migrator) isSnakeCaseConfig(ui map[string]interface{}) bool {
	hasUpper, hasUnderscore := false, false
	var walk func(val interface{})
	checkKey := func(k string) {
		if strings.IndexFunc(k, unicode.IsUpper) >= 0 {
			hasUpper = true
		}
		if strings.Contains(strings.TrimLeft(k, "_"), "_") {
			hasUnderscore = true
		}
	}
	walk = func(val interface{}) {
		switch v := val.(type) {
		case map[interface{}]interface{}:
			for k, vv := range v {
				checkKey(cast.ToString(k))
				walk(vv)
			}
		case map[string]interface{}:
			for k, vv := range v {
				checkKey(k)
				walk(vv)
			}
		case []interface{}:
			for _, vv := range v {
				walk(vv)
			}
		}
	}
	walk(ui)

	return !hasUpper && hasUnderscore
}

// renameKey renames a fully qualified key name in a map
func (m *schema58Migrator) renameKey(mm map[string]interface{}, from, to string) {
	nm := utils.NestedMap(mm)
	v, found := nm.Get(from)
	if !found {
		return
	}

	nm.Delete(from)
	nm.Set(to, v)
}

func (m *schema58Migrator) renameFrontPageContentKeys(ui map[string]interface{}) {
	frontPageContent, found := ui["frontPageContent"].([]interface{})
	if !found {
		return
	}

	for _, v := range frontPageContent {
		vm := v.(map[string]interface{})
		m.renameKey(vm, "savedfilterid", "savedFilterId")
		m.renameKey(vm, "sortby", "sortBy")
	}
}

// installedPluginIDs lists the IDs of the plugins in the plugin folder. As in
// pkg/plugin, a plugin's ID is the name of its .yml file without the extension,
// case-sensitive, and the folder is read with its subfolders. The files are not
// parsed: a .yml file that is not a plugin only keeps a stored key with its own
// name as it is, and only when no installed plugin has the camelCase form.
func (m *schema58Migrator) installedPluginIDs(path string) (map[string]bool, error) {
	if path == "" {
		return nil, fmt.Errorf("no plugin folder set")
	}

	ids := make(map[string]bool)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		// no plugins installed
		return ids, nil
	}

	err := fsutil.SymWalk(path, func(fp string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(fp) == ".yml" {
			name := filepath.Base(fp)
			ids[name[:strings.LastIndex(name, ".")]] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// pluginKeys returns the IDs to store the settings saved under key at. Before
// schema 58 a plugin ID was saved in snake_case, so myPlugin and my_plugin
// were both saved as my_plugin, and both plugins read the same settings. Those
// settings go to the camelCase ID, as before, unless key is the ID of an
// installed plugin, which keeps them. When both plugins are installed, each
// gets a copy, as each read them before. Without the installed IDs every key
// is converted.
func (m *schema58Migrator) pluginKeys(key string, installed map[string]bool) []string {
	converted := m.fromSnakeCase(key)
	switch {
	case converted == key || !installed[key]:
		return []string{converted}
	case installed[converted]:
		return []string{key, converted}
	default:
		return []string{key}
	}
}

func (m *schema58Migrator) migrateConfig() error {
	c := config.GetInstance()

	orgPath := c.GetConfigFile()

	if orgPath == "" {
		// no config file to migrate (usually in a test)
		return nil
	}

	ui := c.GetUIConfiguration()
	if len(ui) == 0 {
		// no UI config to migrate
		return nil
	}

	// This step can meet a config file of any age: a new database, a reset or
	// a restored old database can each come with a config file this build
	// wrote. Converting one of those breaks the settings that use _, such as
	// default filters and plugin IDs.
	if !m.isSnakeCaseConfig(ui) {
		logger.Info("The UI settings in the config file are already in camelCase, leaving the config file as it is")
		return nil
	}

	// save a backup of the original config file
	backupPath := fmt.Sprintf("%s.57.%s", orgPath, time.Now().Format("20060102_150405"))

	data, err := c.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal backup config: %w", err)
	}

	logger.Infof("Backing up config to %s", backupPath)
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup config: %w", err)
	}

	// migrate the plugin and UI configs from snake_case to camelCase
	if ui != nil {
		ui = m.fromSnakeCaseMap(ui)

		// find and rename specific frontEndPage keys
		m.renameFrontPageContentKeys(ui)

		c.SetUIConfiguration(ui)
	}

	installed, err := m.installedPluginIDs(c.GetPluginsPath())
	if err != nil {
		logger.Warnf("Could not list the installed plugins, converting every plugin ID to camelCase: %v", err)
	}

	plugins := c.GetAllPluginConfiguration()
	newPlugins := make(map[string]interface{})
	for key, value := range plugins {
		for _, newKey := range m.pluginKeys(key, installed) {
			newPlugins[newKey] = m.fromSnakeCaseMap(value)
		}
	}

	c.SetInterface(config.PluginsSetting, newPlugins)
	if err := c.Write(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func init() {
	sqlite.RegisterPostMigration(58, post58)
}
