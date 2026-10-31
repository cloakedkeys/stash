package manager

import (
	"net/url"
	"testing"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAudioStreamPaths(t *testing.T) {
	direct, err := url.Parse("http://localhost/audio/1/stream?sig=a")
	require.NoError(t, err)
	transcode, err := url.Parse("http://localhost/audio/1/stream.mp3?sig=b")
	require.NoError(t, err)

	t.Run("nil audio", func(t *testing.T) {
		_, err := GetAudioStreamPaths(nil, direct, transcode)
		assert.Error(t, err)
	})

	t.Run("no file", func(t *testing.T) {
		audio := &models.Audio{Files: models.NewRelatedAudioFiles([]*models.AudioFile{})}
		got, err := GetAudioStreamPaths(audio, direct, transcode)
		assert.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("direct then mp3", func(t *testing.T) {
		file := &models.AudioFile{
			BaseFile:   &models.BaseFile{Path: "/music/a.flac"},
			Format:     "flac",
			AudioCodec: "flac",
		}
		audio := &models.Audio{Files: models.NewRelatedAudioFiles([]*models.AudioFile{file})}

		got, err := GetAudioStreamPaths(audio, direct, transcode)
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, direct.String(), got[0].URL)
		assert.Equal(t, directAudioStreamLabel, *got[0].Label)

		assert.Equal(t, transcode.String(), got[1].URL)
		assert.Equal(t, transcodeAudioStreamLabel, *got[1].Label)
		assert.Equal(t, ffmpeg.MimeMp3Audio, *got[1].MimeType)
	})
}
