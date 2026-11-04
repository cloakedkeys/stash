package manager

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

type GenerateClipPreviewTask struct {
	Image     models.Image
	Overwrite bool
}

func (t *GenerateClipPreviewTask) GetDescription() string {
	return fmt.Sprintf("Generating Preview for image Clip %s", t.Image.Path)
}

func (t *GenerateClipPreviewTask) Start(ctx context.Context) {
	if !t.required() {
		return
	}

	prevPath := GetInstance().Paths.Generated.GetClipPreviewPath(t.Image.Checksum, models.DefaultGthumbWidth)
	f := t.Image.Files.Primary()
	if f == nil {
		return
	}
	filePath := f.Base().Path

	clipPreviewOptions := image.ClipPreviewOptions{
		InputArgs:  GetInstance().Config.GetTranscodeInputArgs(),
		OutputArgs: GetInstance().Config.GetTranscodeOutputArgs(),
		Preset:     GetInstance().Config.GetPreviewPreset().String(),
		HWAccel:    GetInstance().Config.GetGenerationHardwareAcceleration(),
	}

	// ffmpeg writes straight to the file, so its folder must exist
	if err := fsutil.EnsureDirAll(filepath.Dir(prevPath)); err != nil {
		logger.Errorf("creating folder for %s: %v", prevPath, err)
		return
	}

	encoder := image.NewThumbnailEncoder(GetInstance().FFMpeg, GetInstance().FFProbe, clipPreviewOptions)
	err := encoder.GetPreview(filePath, prevPath, models.DefaultGthumbWidth)
	if err != nil {
		logger.Errorf("getting preview for image %s: %w", filePath, err)
		return
	}

}

func (t *GenerateClipPreviewTask) required() bool {
	f, ok := t.Image.Files.Primary().(*models.VideoFile)
	// ffprobe and ffmpeg require a real path, not a ZIP entry path.
	if !ok || f.Base().ZipFileID != nil {
		return false
	}

	if t.Overwrite {
		return true
	}

	prevPath := GetInstance().Paths.Generated.GetClipPreviewPath(t.Image.Checksum, models.DefaultGthumbWidth)
	if exists, _ := fsutil.FileExists(prevPath); exists {
		return false
	}

	return true
}
