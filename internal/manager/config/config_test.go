package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_GetAllPluginConfiguration(t *testing.T) {
	i := InitializeEmpty()

	assert.Equal(t, i.GetAllPluginConfiguration(), map[string]map[string]interface{}{})

	i.SetPluginConfiguration("plugin1", map[string]interface{}{"key1": "value1"})

	assert.Equal(t, map[string]map[string]interface{}{
		"plugin1": {"key1": "value1"},
	}, i.GetAllPluginConfiguration())

	i.SetPluginConfiguration("plugin2", map[string]interface{}{"key2": "value2"})

	assert.Equal(t, map[string]map[string]interface{}{
		"plugin1": {"key1": "value1"},
		"plugin2": {"key2": "value2"},
	}, i.GetAllPluginConfiguration())

	// ensure SetPluginConfiguration overwrites existing configuration
	i.SetPluginConfiguration("plugin2", map[string]interface{}{"key3": "value3"})

	assert.Equal(t, map[string]map[string]interface{}{
		"plugin1": {"key1": "value1"},
		"plugin2": {"key3": "value3"},
	}, i.GetAllPluginConfiguration())
}

func TestConfig_ImageLightboxScrollToChangeImage(t *testing.T) {
	i := InitializeEmpty()

	assert.True(t, i.GetImageLightboxOptions().ScrollToChangeImage, "enabled for existing configurations")

	i.SetInterface(ImageLightboxScrollToChangeImage, false)
	assert.False(t, i.GetImageLightboxOptions().ScrollToChangeImage)

	i.SetInterface(ImageLightboxScrollToChangeImage, true)
	assert.True(t, i.GetImageLightboxOptions().ScrollToChangeImage)

	i.SetInterface(ImageLightboxScrollToChangeImage, nil)
	assert.True(t, i.GetImageLightboxOptions().ScrollToChangeImage, "clearing the setting restores the default")
}
