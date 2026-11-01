package config

import (
	"path/filepath"
	"testing"

	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStashPathsConfig() *Config {
	return &Config{main: koanf.New("."), overrides: koanf.New(".")}
}

func TestGetStashPathsFromEnvironment(t *testing.T) {
	t.Setenv("STASH_STASH", "/data/")

	c := newStashPathsConfig()
	c.loadFromEnv()

	got := c.GetStashPaths()
	require.Len(t, got, 1)
	assert.Equal(t, filepath.FromSlash("/data"), got[0].Path, "no trailing slash, as gallery paths have none")
	assert.False(t, got[0].ExcludeVideo)
	assert.False(t, got[0].ExcludeImage)
}

func TestGetStashPathsNotSet(t *testing.T) {
	assert.Empty(t, newStashPathsConfig().GetStashPaths())
}

// Clearing every folder under Settings saves an empty list, which isn't a folder.
func TestGetStashPathsEmptyList(t *testing.T) {
	c := newStashPathsConfig()
	require.NoError(t, c.main.Set(Stash, []interface{}{}))

	assert.Empty(t, c.GetStashPaths())
}

// Folders saved in the config file win over the environment variable.
func TestGetStashPathsConfigFileWins(t *testing.T) {
	t.Setenv("STASH_STASH", "/data/")

	c := newStashPathsConfig()
	c.loadFromEnv()
	require.NoError(t, c.main.Set(Stash, []interface{}{
		map[string]interface{}{"path": "/saved/", "excludevideo": true},
	}))

	got := c.GetStashPaths()
	require.Len(t, got, 1)
	assert.Equal(t, "/saved/", got[0].Path)
	assert.True(t, got[0].ExcludeVideo)
}

// The older config file format is a plain list of paths.
func TestGetStashPathsLegacyList(t *testing.T) {
	c := newStashPathsConfig()
	require.NoError(t, c.main.Set(Stash, []interface{}{"/one/", "/two/"}))

	got := c.GetStashPaths()
	require.Len(t, got, 2)
	assert.Equal(t, "/one/", got[0].Path)
	assert.Equal(t, "/two/", got[1].Path)
}
