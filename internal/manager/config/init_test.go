package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromEnvEmpty(t *testing.T) {
	for name, key := range envBinds {
		t.Run(name, func(t *testing.T) {
			t.Setenv("STASH_"+strings.ToUpper(name), "")

			for _, source := range []string{"unset", "config", "command line"} {
				t.Run(source, func(t *testing.T) {
					cfg := &Config{main: koanf.New("."), overrides: koanf.New(".")}
					want := ""
					if source != "unset" {
						want = "configured-value"
						require.NoError(t, cfg.main.Set(key, want))
					}
					if source == "command line" {
						want = "flag-value"
						require.NoError(t, cfg.overrides.Set(key, want))
					}

					cfg.loadFromEnv()

					assert.Equal(t, source == "command line", cfg.HasOverride(key))
					assert.Equal(t, want, cfg.getString(key))
				})
			}
		})
	}
}

func TestLoadFromEnvEmptyDatabaseUsesDefault(t *testing.T) {
	t.Setenv("STASH_DATABASE", "")
	cfg := &Config{main: koanf.New("."), overrides: koanf.New(".")}
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.loadFromEnv()
	cfg.setDefaultValues()

	assert.Equal(t, cfg.GetDefaultDatabaseFilePath(), cfg.GetDatabasePath())
	assert.False(t, cfg.HasOverride(Database))
}

func TestLoadFromEnvNonEmpty(t *testing.T) {
	for name, key := range envBinds {
		t.Run(name, func(t *testing.T) {
			// Only the empty string is ignored; false, zero and whitespace remain values.
			for _, value := range []string{"env-value", "false", "0", " "} {
				t.Run(value, func(t *testing.T) {
					t.Setenv("STASH_"+strings.ToUpper(name), value)
					cfg := &Config{main: koanf.New("."), overrides: koanf.New(".")}
					require.NoError(t, cfg.main.Set(key, "configured-value"))
					require.NoError(t, cfg.overrides.Set(key, "flag-value"))

					cfg.loadFromEnv()

					assert.True(t, cfg.HasOverride(key))
					assert.Equal(t, value, cfg.getString(key))
				})
			}
		})
	}
}
