package paths

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
)

const thumbDirDepth int = 2
const thumbDirLength int = 2 // thumbDirDepth * thumbDirLength must be smaller than the length of checksum

type generatedPaths struct {
	// Video and Image hold videos' and images' generated files, grouped by
	// hash. See videoFile and imageFile.
	Video string
	Image string

	// Screenshots, Vtt, Markers, Transcodes, InteractiveHeatmap and Thumbnails
	// are the legacy folders of generated files. Files made before the hash
	// layout may still be there.

	EmbeddedSubtitles  string
	Screenshots        string
	Thumbnails         string
	Vtt                string
	Markers            string
	Transcodes         string
	Downloads          string
	Tmp                string
	InteractiveHeatmap string
}

func newGeneratedPaths(path string) *generatedPaths {
	gp := generatedPaths{}
	gp.Video = filepath.Join(path, "video")
	gp.Image = filepath.Join(path, "image")
	gp.EmbeddedSubtitles = filepath.Join(path, "embedded_subtitles")
	gp.Screenshots = filepath.Join(path, "screenshots")
	gp.Thumbnails = filepath.Join(path, "thumbnails")
	gp.Vtt = filepath.Join(path, "vtt")
	gp.Markers = filepath.Join(path, "markers")
	gp.Transcodes = filepath.Join(path, "transcodes")
	gp.Downloads = filepath.Join(path, "download_stage")
	gp.Tmp = filepath.Join(path, "tmp")
	gp.InteractiveHeatmap = filepath.Join(path, "interactive_heatmaps")
	return &gp
}

func (gp *generatedPaths) GetTmpPath(fileName string) string {
	return filepath.Join(gp.Tmp, fileName)
}

// TempFile creates a temporary file using os.CreateTemp.
// It is the equivalent of calling os.CreateTemp using Tmp and pattern.
func (gp *generatedPaths) TempFile(pattern string) (*os.File, error) {
	if err := gp.EnsureTmpDir(); err != nil {
		logger.Warnf("Could not ensure existence of a temporary directory: %v", err)
	}
	return os.CreateTemp(gp.Tmp, pattern)
}

func (gp *generatedPaths) EnsureTmpDir() error {
	return fsutil.EnsureDir(gp.Tmp)
}

func (gp *generatedPaths) EmptyTmpDir() error {
	return fsutil.EmptyDir(gp.Tmp)
}

func (gp *generatedPaths) RemoveTmpDir() error {
	return fsutil.RemoveDir(gp.Tmp)
}

func (gp *generatedPaths) TempDir(pattern string) (string, error) {
	if err := gp.EnsureTmpDir(); err != nil {
		logger.Warnf("Could not ensure existence of a temporary directory: %v", err)
	}
	ret, err := os.MkdirTemp(gp.Tmp, pattern)
	if err != nil {
		return "", err
	}

	if err = fsutil.EmptyDir(ret); err != nil {
		logger.Warnf("could not recursively empty dir: %v", err)
	}

	return ret, nil
}

// imageFile returns the path of an image's generated file in the hash layout:
// image/ab/cd/<checksum>_<kind>.
func (gp *generatedPaths) imageFile(checksum, kind string) string {
	return filepath.Join(hashDir(gp.Image, checksum), checksum+"_"+kind)
}

// Thumbnail is an image's thumbnail, in the hash layout and where it was kept before.
func (gp *generatedPaths) Thumbnail(checksum string, width int) GeneratedFile {
	return GeneratedFile{
		Path:   gp.imageFile(checksum, fmt.Sprintf("thumb_%d.jpg", width)),
		Legacy: filepath.Join(gp.Thumbnails, fsutil.GetIntraDir(checksum, thumbDirDepth, thumbDirLength), fmt.Sprintf("%s_%d.jpg", checksum, width)),
	}
}

// ClipPreview is a video image's preview, in the hash layout and where it was kept before.
func (gp *generatedPaths) ClipPreview(checksum string, width int) GeneratedFile {
	return GeneratedFile{
		Path:   gp.imageFile(checksum, fmt.Sprintf("preview_%d.webm", width)),
		Legacy: filepath.Join(gp.Thumbnails, fsutil.GetIntraDir(checksum, thumbDirDepth, thumbDirLength), fmt.Sprintf("%s_%d.webm", checksum, width)),
	}
}

// GetThumbnailPath returns where the image's thumbnail is, or where a new one
// goes. See GeneratedFile.Find.
func (gp *generatedPaths) GetThumbnailPath(checksum string, width int) string {
	return gp.Thumbnail(checksum, width).Find()
}

// GetClipPreviewPath returns where the image's clip preview is, or where a new
// one goes. See GeneratedFile.Find.
func (gp *generatedPaths) GetClipPreviewPath(checksum string, width int) string {
	return gp.ClipPreview(checksum, width).Find()
}
