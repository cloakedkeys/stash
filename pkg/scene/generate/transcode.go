package generate

import (
	"context"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/ffmpeg/transcoder"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
)

const transcodeCRF = 23

type TranscodeOptions struct {
	// MaxSize is the maximum resolution of the smaller dimension.
	// 0 means no scaling.
	MaxSize int
}

func (g Generator) Transcode(ctx context.Context, path string, width, height int, hash string, options TranscodeOptions) error {
	lockCtx := g.LockManager.ReadLock(ctx, path)
	defer lockCtx.Cancel()

	return g.makeTranscode(lockCtx, hash, g.transcode(path, width, height, options))
}

// TranscodeVideo transcodes the video, and removes the audio.
// In some videos where the audio codec is not supported by ffmpeg,
// ffmpeg fails if you try to transcode the audio
func (g Generator) TranscodeVideo(ctx context.Context, path string, width, height int, hash string, options TranscodeOptions) error {
	lockCtx := g.LockManager.ReadLock(ctx, path)
	defer lockCtx.Cancel()

	return g.makeTranscode(lockCtx, hash, g.transcodeVideo(path, width, height, options))
}

// TranscodeAudio will copy the video stream as is, and transcode audio.
func (g Generator) TranscodeAudio(ctx context.Context, input string, hash string) error {
	lockCtx := g.LockManager.ReadLock(ctx, input)
	defer lockCtx.Cancel()

	return g.makeTranscode(lockCtx, hash, g.transcodeAudio(input))
}

// TranscodeCopyVideo will copy the video stream as is, and drop the audio stream.
func (g Generator) TranscodeCopyVideo(ctx context.Context, input string, hash string) error {
	lockCtx := g.LockManager.ReadLock(ctx, input)
	defer lockCtx.Cancel()

	return g.makeTranscode(lockCtx, hash, g.transcodeCopyVideo(input))
}

func (g Generator) makeTranscode(lockCtx *fsutil.LockContext, hash string, generateFn generateFn) error {
	output := g.ScenePaths.GetTranscodePath(hash)
	if !g.Overwrite {
		if exists, _ := fsutil.FileExists(output); exists {
			return nil
		}
	}

	if err := g.generateFile(lockCtx, g.ScenePaths, mp4Pattern, output, generateFn); err != nil {
		return err
	}

	logger.Debug("created transcode: ", output)

	return nil
}

// transcodeTargetHeight returns the size of the shorter side to transcode to.
// MaxSize applies to the shorter side, which is what HWMaxResFilter expects.
func transcodeTargetHeight(width, height int, options TranscodeOptions) int {
	if options.MaxSize != 0 && options.MaxSize < min(width, height) {
		return options.MaxSize
	}
	return height
}

// transcodeVideoArgs returns the video arguments and the extra input
// arguments for a transcode with the given codec.
func (g Generator) transcodeVideoArgs(codec ffmpeg.VideoCodec, fullhw bool, width, height int, options TranscodeOptions) (videoArgs ffmpeg.Args, extraInputArgs ffmpeg.Args) {
	// Hardware encoders get their filter chain even when nothing is scaled:
	// VAAPI only encodes frames uploaded to the GPU (format=nv12,hwupload),
	// and AMF can't take 10-bit frames. For libx264 the filter is empty
	// unless the video is scaled.
	var videoFilter ffmpeg.VideoFilter
	if options.MaxSize != 0 || !isQSV(codec) {
		videoFilter = g.Encoder.HWMaxResFilter(codec, width, height, transcodeTargetHeight(width, height, options), fullhw)
	}

	videoArgs = codec.ExtraArgsHQ("superfast", transcodeCRF)
	videoArgs = videoArgs.VideoFilter(videoFilter)
	videoArgs = videoArgs.MovieTimescale(g.Encoder.Version())

	extraInputArgs = g.Encoder.HWDeviceInit(g.FFMpegConfig.GetTranscodeInputArgs(), codec, fullhw)
	return videoArgs, extraInputArgs
}

// isQSV reports whether codec is an Intel Quick Sync encoder. Its software
// decode chain uploads 10-bit frames as 10-bit (#2165), which h264_qsv
// can't encode, so Quick Sync transcodes at original size keep running
// without the chain and let ffmpeg convert the frames.
func isQSV(codec ffmpeg.VideoCodec) bool {
	return codec == ffmpeg.VideoCodecI264 || codec == ffmpeg.VideoCodecI264C || codec == ffmpeg.VideoCodecIVP9
}

func (g Generator) transcode(path string, width, height int, options TranscodeOptions) generateFn {
	return func(lockCtx *fsutil.LockContext, tmpFn string) error {
		targetHeight := transcodeTargetHeight(width, height, options)
		codec, fullhw := g.DetermineCodecAndHW(lockCtx, path, width, height, targetHeight)
		videoArgs, extraInputArgs := g.transcodeVideoArgs(codec, fullhw, width, height, options)

		args := transcoder.Transcode(path, transcoder.TranscodeOptions{
			OutputPath: tmpFn,
			VideoCodec: codec,
			VideoArgs:  videoArgs,
			AudioCodec: ffmpeg.AudioCodecAAC,

			ExtraInputArgs:  extraInputArgs,
			ExtraOutputArgs: g.FFMpegConfig.GetTranscodeOutputArgs(),
		})

		return g.generate(lockCtx, args)
	}
}

func (g Generator) transcodeVideo(path string, width, height int, options TranscodeOptions) generateFn {
	return func(lockCtx *fsutil.LockContext, tmpFn string) error {
		targetHeight := transcodeTargetHeight(width, height, options)
		codec, fullhw := g.DetermineCodecAndHW(lockCtx, path, width, height, targetHeight)
		videoArgs, extraInputArgs := g.transcodeVideoArgs(codec, fullhw, width, height, options)

		var audioArgs ffmpeg.Args
		audioArgs = audioArgs.SkipAudio()

		args := transcoder.Transcode(path, transcoder.TranscodeOptions{
			OutputPath: tmpFn,
			VideoCodec: codec,
			VideoArgs:  videoArgs,
			AudioArgs:  audioArgs,

			ExtraInputArgs:  extraInputArgs,
			ExtraOutputArgs: g.FFMpegConfig.GetTranscodeOutputArgs(),
		})

		return g.generate(lockCtx, args)
	}
}

func (g Generator) transcodeAudio(input string) generateFn {
	return func(lockCtx *fsutil.LockContext, tmpFn string) error {
		args := transcoder.Transcode(input, transcoder.TranscodeOptions{
			OutputPath: tmpFn,
			VideoCodec: ffmpeg.VideoCodecCopy,
			VideoArgs:  ffmpeg.Args{}.MovieTimescale(g.Encoder.Version()),
			AudioCodec: ffmpeg.AudioCodecAAC,
		})

		return g.generate(lockCtx, args)
	}
}

func (g Generator) transcodeCopyVideo(input string) generateFn {
	return func(lockCtx *fsutil.LockContext, tmpFn string) error {

		var audioArgs ffmpeg.Args
		audioArgs = audioArgs.SkipAudio()

		args := transcoder.Transcode(input, transcoder.TranscodeOptions{
			OutputPath: tmpFn,
			VideoCodec: ffmpeg.VideoCodecCopy,
			VideoArgs:  ffmpeg.Args{}.MovieTimescale(g.Encoder.Version()),
			AudioArgs:  audioArgs,
		})

		return g.generate(lockCtx, args)
	}
}
