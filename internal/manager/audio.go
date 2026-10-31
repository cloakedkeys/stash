package manager

import (
	"fmt"
	"net/url"

	audiopkg "github.com/stashapp/stash/pkg/audio"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
)

type AudioStreamEndpoint struct {
	URL      string  `json:"url"`
	MimeType *string `json:"mime_type"`
	Label    *string `json:"label"`
}

const (
	directAudioStreamLabel    = "Direct stream"
	transcodeAudioStreamLabel = "MP3"
)

// GetAudioStreamPaths returns the stream endpoints for the audio: the original
// file, then a live mp3 transcode for formats the client can't play directly.
func GetAudioStreamPaths(audio *models.Audio, directStreamURL *url.URL, transcodeStreamURL *url.URL) ([]*AudioStreamEndpoint, error) {
	if audio == nil {
		return nil, fmt.Errorf("nil audio")
	}

	pf := audio.Files.Primary()
	if pf == nil {
		return nil, nil
	}

	mimeType := audiopkg.MimeType(pf)
	label := directAudioStreamLabel
	transcodeMimeType := ffmpeg.MimeMp3Audio
	transcodeLabel := transcodeAudioStreamLabel

	return []*AudioStreamEndpoint{
		{
			URL:      directStreamURL.String(),
			MimeType: &mimeType,
			Label:    &label,
		},
		{
			URL:      transcodeStreamURL.String(),
			MimeType: &transcodeMimeType,
			Label:    &transcodeLabel,
		},
	}, nil
}
