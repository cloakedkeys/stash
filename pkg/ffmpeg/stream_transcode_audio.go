package ffmpeg

import (
	"context"
	"errors"
	"net/http"

	"github.com/stashapp/stash/pkg/logger"
)

const (
	audioCodecLibMp3Lame AudioCodec = "libmp3lame"
	formatMP3            Format     = "mp3"
)

// AudioTranscodeOptions describes a live transcode of an audio file to mp3, for
// files whose format the browser can't play directly.
type AudioTranscodeOptions struct {
	Path      string
	StartTime float64
}

func (o AudioTranscodeOptions) makeStreamArgs() Args {
	args := Args{"-hide_banner"}
	args = args.LogLevel(LogLevelError)

	if o.StartTime != 0 {
		args = args.Seek(o.StartTime)
	}

	args = args.Input(o.Path)
	// cover art is exposed as a video stream, so only map the first audio stream
	args = append(args, "-map", "0:a:0", "-vn")
	args = args.AudioCodec(audioCodecLibMp3Lame)
	args = append(args,
		"-q:a", "2",
		// mp3 is at most stereo
		"-ac", "2",
		// the header can't be rewritten at the end of a pipe
		"-write_xing", "0",
	)
	args = args.Format(formatMP3)
	args = args.Output("pipe:")

	return args
}

// ServeAudioTranscode streams the audio file as mp3.
func (sm *StreamManager) ServeAudioTranscode(w http.ResponseWriter, r *http.Request, options AudioTranscodeOptions) {
	streamRequestCtx := NewStreamRequestContext(w, r)
	lockCtx := sm.lockManager.ReadLock(streamRequestCtx, options.Path)

	handler, err := sm.startTranscodeStream(lockCtx, options.makeStreamArgs(), MimeMp3Audio)
	if err != nil {
		// don't log context canceled errors
		if !errors.Is(err, context.Canceled) {
			logger.Errorf("[transcode] error transcoding audio file: %v", err)
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	handler(w, r)
}
