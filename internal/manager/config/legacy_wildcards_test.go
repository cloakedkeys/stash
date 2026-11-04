package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A new config file is written by this build, so its default filters never
// need the upgrade's rewrite. An existing one without the marker does.
func TestLegacyWildcardsConvertedMarker(t *testing.T) {
	newCfg := &Config{main: koanf.New("."), overrides: koanf.New("."), isNewSystem: true}
	newCfg.setNewSystemDefaults()
	newCfg.setExistingSystemDefaults()
	assert.True(t, newCfg.GetLegacyWildcardsConverted())

	existing := &Config{main: koanf.New("."), overrides: koanf.New(".")}
	existing.setExistingSystemDefaults()
	assert.False(t, existing.GetLegacyWildcardsConverted())

	// it's kept as a top-level key when the file is written and read again,
	// and the UI saving its settings doesn't remove it
	path := filepath.Join(t.TempDir(), "config.yml")
	newCfg.SetConfigFile(path)
	require.NoError(t, newCfg.Write())
	newCfg.SetUIConfiguration(map[string]interface{}{"theme": "dark"})
	require.NoError(t, newCfg.Write())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Regexp(t, `(?m)^legacy_wildcards_converted: true$`, string(data))

	loaded := &Config{main: koanf.New("."), overrides: koanf.New(".")}
	require.NoError(t, loaded.load(path))
	assert.True(t, loaded.GetLegacyWildcardsConverted())
}
