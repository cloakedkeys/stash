package video

import (
	"context"
	"errors"
	"fmt"

	"github.com/stashapp/stash/pkg/ffmpeg"
	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
)

// Decorator adds video specific fields to a File.
type Decorator struct {
	FFProbe         *ffmpeg.FFProbe
	InferVRMetadata bool
}

func (d *Decorator) Decorate(ctx context.Context, fs models.FS, f models.File) (models.File, error) {
	if d.FFProbe == nil {
		return f, errors.New("ffprobe not configured")
	}

	base := f.Base()
	// TODO - copy to temp file if not an OsFS
	if _, isOs := fs.(*file.OsFS); !isOs {
		return f, fmt.Errorf("video.constructFile: only OsFS is supported")
	}

	probe := d.FFProbe
	videoFile, err := probe.NewVideoFile(base.Path)
	if err != nil {
		return f, fmt.Errorf("running ffprobe on %q: %w", base.Path, err)
	}

	container, err := ffmpeg.MatchContainer(videoFile.Container, base.Path)
	if err != nil {
		return f, fmt.Errorf("matching container for %q: %w", base.Path, err)
	}

	// check if there is a funscript file
	interactive := false
	if _, err := fs.Lstat(GetFunscriptPath(base.Path)); err == nil {
		interactive = true
	}

	decorated := &models.VideoFile{
		BaseFile:    base,
		Format:      string(container),
		VideoCodec:  videoFile.VideoCodec,
		AudioCodec:  videoFile.AudioCodec,
		Width:       videoFile.Width,
		Height:      videoFile.Height,
		Duration:    videoFile.FileDuration,
		FrameRate:   videoFile.FrameRate,
		BitRate:     videoFile.Bitrate,
		Interactive: interactive,
	}
	if old, ok := f.(*models.VideoFile); ok {
		preserveVRMetadata(decorated, old)
	}
	if d.InferVRMetadata {
		applyFilenameVRMetadata(decorated, inferVRMetadataFromFilename(base.Basename))
	}
	return decorated, nil
}

func preserveVRMetadata(dst, src *models.VideoFile) {
	dst.Projection = src.Projection
	dst.StereoMode = src.StereoMode
	dst.VRCorrections = src.VRCorrections
	dst.VRPassthrough = src.VRPassthrough
}

func (d *Decorator) IsMissingMetadata(ctx context.Context, fs models.FS, f models.File) bool {
	const (
		unsetString = "unset"
		unsetNumber = -1
	)

	vf, ok := f.(*models.VideoFile)
	if !ok {
		return true
	}

	interactive := false
	if _, err := fs.Lstat(GetFunscriptPath(vf.Base().Path)); err == nil {
		interactive = true
	}

	mediaMetadataMissing := vf.VideoCodec == unsetString || vf.AudioCodec == unsetString ||
		vf.Format == unsetString || vf.Width == unsetNumber ||
		vf.Height == unsetNumber || vf.FrameRate == unsetNumber ||
		vf.Duration == unsetNumber ||
		vf.BitRate == unsetNumber || interactive != vf.Interactive
	if mediaMetadataMissing {
		return true
	}
	if d.InferVRMetadata {
		inferred := inferVRMetadataFromFilename(vf.Basename)
		return (vf.Projection == nil && inferred.projection != nil) ||
			(vf.StereoMode == nil && inferred.stereoMode != nil) ||
			(inferred.alphaPacked != nil && (vf.VRPassthrough == nil || vf.VRPassthrough.AlphaPacked == nil))
	}
	return false
}
