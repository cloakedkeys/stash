package ffmpeg

import "context"

// maximum width of a cover image extracted from an audio file
const audioCoverMaxWidth = "1000"

// ExtractAudioCover returns the cover image embedded in the audio file as a jpeg.
func (f *FFMpeg) ExtractAudioCover(ctx context.Context, path string) ([]byte, error) {
	args := Args{"-hide_banner"}
	args = args.LogLevel(LogLevelError)
	args = args.Input(path)
	args = append(args,
		"-map", "0:v:0",
		"-an",
		"-frames:v", "1",
		"-vf", "scale='min("+audioCoverMaxWidth+",iw)':-2",
		"-c:v", "mjpeg",
		"-q:v", "3",
	)
	args = args.Format(FormatImage2)
	args = args.Output("pipe:")

	return f.GenerateOutput(ctx, args, nil)
}
