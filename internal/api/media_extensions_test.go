package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMediaExtensions(t *testing.T) {
	c := config.InitializeEmpty()
	t.Cleanup(func() { config.InitializeEmpty() })

	// Every shipped default remains valid.
	require.NoError(t, validateMediaExtensions(ConfigGeneralInput{
		VideoExtensions:   c.GetVideoExtensions(),
		AudioExtensions:   c.GetAudioExtensions(),
		ImageExtensions:   c.GetImageExtensions(),
		StoryExtensions:   c.GetStoryExtensions(),
		GalleryExtensions: c.GetGalleryExtensions(),
	}, ConfigGeneralInput{}))
	require.NoError(t, validateMediaExtensions(ConfigGeneralInput{}, ConfigGeneralInput{}))
	require.NoError(t, validateMediaExtensions(ConfigGeneralInput{
		VideoExtensions:   []string{},
		AudioExtensions:   []string{},
		ImageExtensions:   []string{},
		StoryExtensions:   []string{},
		GalleryExtensions: []string{},
	}, ConfigGeneralInput{}))

	// An unusual extension already saved can be kept while the list is
	// edited, but not moved to another list.
	saved := ConfigGeneralInput{VideoExtensions: []string{"mp4", "foo"}}
	require.NoError(t, validateMediaExtensions(ConfigGeneralInput{VideoExtensions: []string{"mp4", "mkv", "FOO"}}, saved))
	require.Error(t, validateMediaExtensions(ConfigGeneralInput{ImageExtensions: []string{"png", "foo"}}, saved))

	for _, tt := range []struct {
		name  string
		set   func(*ConfigGeneralInput, []string)
		valid []string
		wrong string
	}{
		{"video", func(i *ConfigGeneralInput, v []string) { i.VideoExtensions = v }, []string{"MP4", "ts", "m2ts", "mts", "vob", "ogv", "3gp", "gif", "ogg", "qt"}, "mp3"},
		{"audio", func(i *ConfigGeneralInput, v []string) { i.AudioExtensions = v }, []string{"MP3", "flac", "ac3", "dts", "mp2", "mp4", "webm", "m4r", "spx"}, "png"},
		{"image", func(i *ConfigGeneralInput, v []string) { i.ImageExtensions = v }, []string{"JPEG", "avif", "jxl", "bmp", "tiff", "webm", "mp4", "jfif", "ogg"}, "mp3"},
		{"story", func(i *ConfigGeneralInput, v []string) { i.StoryExtensions = v }, []string{"TXT", "text", "md", "markdown"}, "pdf"},
		{"gallery ZIP", func(i *ConfigGeneralInput, v []string) { i.GalleryExtensions = v }, []string{"ZIP", "cbz"}, "rar"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var input ConfigGeneralInput
			tt.set(&input, tt.valid)
			require.NoError(t, validateMediaExtensions(input, ConfigGeneralInput{}))

			for _, extension := range []string{"exe", "docx", "mp44", "", "." + tt.valid[0], " " + tt.valid[0], "*", "foo/bar", tt.wrong} {
				tt.set(&input, []string{tt.valid[0], extension})
				err := validateMediaExtensions(input, ConfigGeneralInput{})
				require.Error(t, err, "extension %q", extension)
				assert.Contains(t, err.Error(), "isn't a format Stash can read as "+tt.name)
			}
		})
	}
}

func TestConfigureGeneralRejectsUnsupportedMediaExtensions(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input ConfigGeneralInput
	}{
		{"video", ConfigGeneralInput{VideoExtensions: []string{"mp4", "exe"}}},
		{"audio", ConfigGeneralInput{AudioExtensions: []string{"mp3", "exe"}}},
		{"image", ConfigGeneralInput{ImageExtensions: []string{"png", "exe"}}},
		{"story", ConfigGeneralInput{StoryExtensions: []string{"txt", "exe"}}},
		{"gallery ZIP", ConfigGeneralInput{GalleryExtensions: []string{"zip", "exe"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := config.InitializeEmpty()
			t.Cleanup(func() { config.InitializeEmpty() })
			// The old code reaches Write after changing the live config. A missing
			// parent makes it return there without needing a running manager.
			configFile := filepath.Join(t.TempDir(), "missing", "config.yml")
			c.SetConfigFile(configFile)
			original := ConfigGeneralInput{
				VideoExtensions:   c.GetVideoExtensions(),
				AudioExtensions:   c.GetAudioExtensions(),
				ImageExtensions:   c.GetImageExtensions(),
				StoryExtensions:   c.GetStoryExtensions(),
				GalleryExtensions: c.GetGalleryExtensions(),
			}
			parallelTasks := c.GetParallelTasks() + 1
			tt.input.ParallelTasks = &parallelTasks
			r := &mutationResolver{}
			_, err := r.ConfigureGeneral(context.Background(), tt.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "\"exe\" isn't a format Stash can read as "+tt.name)
			assert.Equal(t, original.VideoExtensions, c.GetVideoExtensions())
			assert.Equal(t, original.AudioExtensions, c.GetAudioExtensions())
			assert.Equal(t, original.ImageExtensions, c.GetImageExtensions())
			assert.Equal(t, original.StoryExtensions, c.GetStoryExtensions())
			assert.Equal(t, original.GalleryExtensions, c.GetGalleryExtensions())
			assert.NotEqual(t, parallelTasks, c.GetParallelTasks())
			_, err = os.Stat(configFile)
			assert.True(t, os.IsNotExist(err))
		})
	}
}
