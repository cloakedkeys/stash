package generate

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/ffmpeg/transcoder"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
)

const (
	// thumbnailWidth   = 320
	// thumbnailQuality = 5

	screenshotQuality = 2

	screenshotDurationProportion = 0.2
)

type ScreenshotOptions struct {
	At *float64
}

func (g Generator) Screenshot(ctx context.Context, input string, videoWidth int, videoDuration float64, options ScreenshotOptions) ([]byte, error) {
	lockCtx := g.LockManager.ReadLock(ctx, input)
	defer lockCtx.Cancel()

	logger.Infof("Creating screenshot for %s", input)

	if g.Range.IsPart() && g.Range.End == nil {
		g.Range.End = &videoDuration
	}
	at := screenshotDurationProportion * videoDuration
	if g.Range.IsPart() {
		at, _ = g.rangeSegment(screenshotDurationProportion*g.Range.Duration(videoDuration), 0)
	}
	if options.At != nil {
		at = *options.At
	}

	if g.Range.IsPart() {
		start, _ := g.rangeSegment(0, 0)
		if at < start || at >= start+g.Range.Duration(videoDuration) {
			return nil, fmt.Errorf("cover time %g is outside the scene range", at)
		}
	}

	ret, err := g.generateBytes(lockCtx, g.ScenePaths, jpgPattern, g.screenshot(input, screenshotOptions{
		Time:    at,
		Quality: screenshotQuality,
		// default Width is video width
	}))
	if err != nil {
		return nil, err
	}

	return ret, nil
}

type screenshotOptions struct {
	Time    float64
	Width   int
	Quality int
}

func (g Generator) screenshot(input string, options screenshotOptions) generateFn {
	return func(lockCtx *fsutil.LockContext, tmpFn string) error {
		ssOptions := transcoder.ScreenshotOptions{
			SampleAspectRatio: g.SampleAspectRatio,
			OutputPath:        tmpFn,
			OutputType:        transcoder.ScreenshotOutputTypeImage2,
			Quality:           options.Quality,
			Width:             options.Width,
		}

		if g.Range.IsPart() {
			ssOptions.SlowSeek = true
			if g.Range.End != nil {
				ssOptions.Duration = *g.Range.End - options.Time
			}
		}

		args := transcoder.ScreenshotTime(input, options.Time, ssOptions)
		if err := g.generate(lockCtx, args); err != nil {
			logger.Warnf("[generator] fast screenshot seek failed for %s at %.3fs, retrying with accurate seek: %v", input, options.Time, err)

			ssOptions.SlowSeek = true
			args = transcoder.ScreenshotTime(input, options.Time, ssOptions)
			return g.generate(lockCtx, args)
		}

		return nil
	}
}
