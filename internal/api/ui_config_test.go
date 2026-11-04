package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stashapp/stash/pkg/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// #2079: numbers inside lists in UI and plugin settings were written to the
// config file as quoted strings, and came back as strings after a restart.
func TestConfigureUINumbersInLists(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yml")
	c := config.InitializeEmpty()
	c.SetConfigFile(configFile)
	c.SetString(config.Database, filepath.Join(dir, "stash-go.sqlite"))
	c.SetString(config.Generated, filepath.Join(dir, "generated"))
	c.SetString(config.BlobsPath, filepath.Join(dir, "blobs"))

	db := mocks.NewDatabase()
	db.UserUIConfig.On("Get", mock.Anything).Return(nil, nil)
	r := &mutationResolver{newResolver(db)}

	adminCtx := session.SetCurrentUser(context.Background(), models.User{
		Username: "admin",
		Roles:    models.Roles{models.RoleEnumAdmin},
	})

	_, err := r.ConfigureUI(adminCtx, map[string]interface{}{
		"plain": json.Number("7"),
		"frontPageContent": []interface{}{
			map[string]interface{}{"savedFilterId": json.Number("3"), "__typename": "SavedFilter"},
			map[string]interface{}{"nested": []interface{}{json.Number("4"), []interface{}{json.Number("5.5")}}},
		},
		"list": []interface{}{json.Number("1"), json.Number("2.5"), "text"},
		"map":  map[string]interface{}{"list": []interface{}{json.Number("6")}},
	}, nil)
	require.NoError(t, err)

	_, err = r.ConfigureUISetting(adminCtx, "setting", []interface{}{json.Number("8")})
	require.NoError(t, err)

	_, err = r.ConfigurePlugin(adminCtx, "plugin", map[string]interface{}{"ids": []interface{}{json.Number("9")}})
	require.NoError(t, err)

	want := map[string]interface{}{
		"plain": int64(7),
		"frontPageContent": []interface{}{
			map[string]interface{}{"savedFilterId": int64(3), "__typename": "SavedFilter"},
			map[string]interface{}{"nested": []interface{}{int64(4), []interface{}{5.5}}},
		},
		"list":    []interface{}{int64(1), 2.5, "text"},
		"map":     map[string]interface{}{"list": []interface{}{int64(6)}},
		"setting": []interface{}{int64(8)},
	}
	assert.Equal(t, want, c.GetUIConfiguration())

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	for _, quoted := range []string{`"3"`, `"4"`, `"5.5"`, `"1"`, `"2.5"`, `"6"`, `"8"`, `"9"`} {
		assert.NotContains(t, string(data), quoted)
	}
	assert.Contains(t, string(data), "savedFilterId: 3")

	// reload the config file as stash does at startup
	t.Setenv("STASH_CONFIG_FILE", configFile)
	reloaded, err := config.Initialize()
	require.NoError(t, err)

	// YAML numbers come back as int or float64
	assert.Equal(t, map[string]interface{}{
		"plain": 7,
		"frontPageContent": []interface{}{
			map[string]interface{}{"savedFilterId": 3, "__typename": "SavedFilter"},
			map[string]interface{}{"nested": []interface{}{4, []interface{}{5.5}}},
		},
		"list":    []interface{}{1, 2.5, "text"},
		"map":     map[string]interface{}{"list": []interface{}{6}},
		"setting": []interface{}{8},
	}, reloaded.GetUIConfiguration())
	assert.Equal(t, map[string]interface{}{"ids": []interface{}{9}}, reloaded.GetPluginConfiguration("plugin"))
}

// #2079: an invalid number inside a list is refused, naming where it is.
func TestConfigureUINumbersInListsError(t *testing.T) {
	c := config.InitializeEmpty()
	c.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	db := mocks.NewDatabase()
	db.UserUIConfig.On("Get", mock.Anything).Return(nil, nil)
	r := &mutationResolver{newResolver(db)}
	adminCtx := session.SetCurrentUser(context.Background(), models.User{
		Username: "admin",
		Roles:    models.Roles{models.RoleEnumAdmin},
	})

	_, err := r.ConfigureUI(adminCtx, map[string]interface{}{
		"list": []interface{}{json.Number("1"), map[string]interface{}{"huge": json.Number("1e400")}},
	}, nil)
	assert.ErrorContains(t, err, `"list"[1]."huge": invalid number "1e400"`)

	_, err = r.ConfigureUISetting(adminCtx, "setting", []interface{}{json.Number("1e400")})
	assert.ErrorContains(t, err, `"setting"[0]: invalid number "1e400"`)

	_, err = r.ConfigurePlugin(adminCtx, "plugin", map[string]interface{}{"ids": []interface{}{json.Number("1e400")}})
	assert.ErrorContains(t, err, `"ids"[0]: invalid number "1e400"`)

	assert.Empty(t, c.GetUIConfiguration())
	assert.Empty(t, c.GetPluginConfiguration("plugin"))
}

func TestUIConfig(t *testing.T) {
	c := config.InitializeEmpty()
	globalConfig := map[string]interface{}{"abbreviateCounters": true}
	c.SetUIConfiguration(globalConfig)

	userCtx := session.SetCurrentUser(context.Background(), models.User{
		ID:       2,
		Username: "user",
		Roles:    models.Roles{models.RoleEnumRead},
	})
	guestCtx := session.SetCurrentUser(context.Background(), models.User{
		Username: "guest",
		Roles:    models.Roles{models.RoleEnumRead},
	})

	t.Run("falls back to the config file", func(t *testing.T) {
		db := mocks.NewDatabase()
		r := newResolver(db)
		db.UserUIConfig.On("Get", mock.Anything).Return(nil, nil).Once()

		got, err := r.getUIConfig(userCtx)
		assert.NoError(t, err)
		assert.Equal(t, globalConfig, got)
		db.AssertExpectations(t)
	})

	t.Run("returns the user's configuration", func(t *testing.T) {
		db := mocks.NewDatabase()
		r := newResolver(db)
		userConfig := map[string]interface{}{"abbreviateCounters": false}
		db.UserUIConfig.On("Get", mock.Anything).Return(userConfig, nil).Once()

		got, err := r.getUIConfig(userCtx)
		assert.NoError(t, err)
		assert.Equal(t, userConfig, got)
		db.AssertExpectations(t)
	})

	t.Run("non-admin changes do not affect the config file", func(t *testing.T) {
		db := mocks.NewDatabase()
		r := newResolver(db)
		userConfig := map[string]interface{}{"abbreviateCounters": false}
		db.UserUIConfig.On("Set", mock.Anything, userConfig).Return(nil).Once()

		assert.NoError(t, r.setUIConfig(userCtx, userConfig))
		assert.Equal(t, globalConfig, c.GetUIConfiguration())
		db.AssertExpectations(t)
	})

	t.Run("guest cannot change the configuration", func(t *testing.T) {
		db := mocks.NewDatabase()
		r := newResolver(db)

		assert.Error(t, r.setUIConfig(guestCtx, map[string]interface{}{}))
		assert.Equal(t, globalConfig, c.GetUIConfiguration())
		db.AssertExpectations(t)
	})
}
