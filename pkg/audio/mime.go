package audio

import (
	"mime"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/models"
)

// containerMimeTypes maps the container reported by ffprobe (see
// ffmpeg.MatchContainer) to the mime type used to serve the file.
var containerMimeTypes = map[string]string{
	"mp3":      ffmpeg.MimeMp3Audio,
	"mp4":      ffmpeg.MimeMp4Audio,
	"m4v":      ffmpeg.MimeMp4Audio,
	"mov":      ffmpeg.MimeMp4Audio,
	"flac":     ffmpeg.MimeFlacAudio,
	"ogg":      ffmpeg.MimeOggAudio,
	"wav":      ffmpeg.MimeWavAudio,
	"aac":      ffmpeg.MimeAacAudio,
	"webm":     ffmpeg.MimeWebmAudio,
	"matroska": ffmpeg.MimeMkvAudio,
}

// MimeType returns the mime type for the audio file's container, falling back
// to the extension and finally to mpeg.
func MimeType(f *models.AudioFile) string {
	if mimeType, found := containerMimeTypes[strings.ToLower(f.Format)]; found {
		return mimeType
	}

	// fall back to the extension - covers containers ffprobe reports under a
	// name we don't recognise
	if mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(f.Base().Path))); mimeType != "" {
		return mimeType
	}

	return ffmpeg.MimeMp3Audio
}
