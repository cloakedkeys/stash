package paths

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type sceneMarkerPaths struct {
	generatedPaths
}

func newSceneMarkerPaths(p Paths) *sceneMarkerPaths {
	sp := sceneMarkerPaths{
		generatedPaths: *p.Generated,
	}
	return &sp
}

// markerFilePrefix starts the kind of every marker file in the hash layout.
const markerFilePrefix = "marker_"

// GetFolderPath returns the legacy folder of the video's marker files.
func (sp *sceneMarkerPaths) GetFolderPath(checksum string) string {
	return filepath.Join(sp.Markers, checksum)
}

func (sp *sceneMarkerPaths) markerFile(checksum string, seconds int, ext string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, fmt.Sprintf("%s%d.%s", markerFilePrefix, seconds, ext)),
		Legacy: filepath.Join(sp.GetFolderPath(checksum), strconv.Itoa(seconds)+"."+ext),
	}
}

func (sp *sceneMarkerPaths) VideoPreview(checksum string, seconds int) GeneratedFile {
	return sp.markerFile(checksum, seconds, "mp4")
}

func (sp *sceneMarkerPaths) WebpPreview(checksum string, seconds int) GeneratedFile {
	return sp.markerFile(checksum, seconds, "webp")
}

func (sp *sceneMarkerPaths) Screenshot(checksum string, seconds int) GeneratedFile {
	return sp.markerFile(checksum, seconds, "jpg")
}

// MarkerFiles returns the generated files of the video's marker at seconds.
func (sp *sceneMarkerPaths) MarkerFiles(checksum string, seconds int) []GeneratedFile {
	return []GeneratedFile{
		sp.VideoPreview(checksum, seconds),
		sp.WebpPreview(checksum, seconds),
		sp.Screenshot(checksum, seconds),
	}
}

// ListMarkerFiles returns the marker files of the video in the hash layout.
// Its legacy marker files are all in GetFolderPath.
func (sp *sceneMarkerPaths) ListMarkerFiles(checksum string) []string {
	return sp.listVideoFiles(checksum, markerFilePrefix)
}

// RenamedMarkerFile returns the new path of a marker file that ListMarkerFiles
// returned for oldHash, when the video's hash changes to newHash.
func (sp *sceneMarkerPaths) RenamedMarkerFile(path, oldHash, newHash string) string {
	return sp.videoFile(newHash, strings.TrimPrefix(filepath.Base(path), oldHash+"_"))
}

// The Get...Path functions return where the file is, or where a new one goes.
// See GeneratedFile.Find.

func (sp *sceneMarkerPaths) GetVideoPreviewPath(checksum string, seconds int) string {
	return sp.VideoPreview(checksum, seconds).Find()
}

func (sp *sceneMarkerPaths) GetWebpPreviewPath(checksum string, seconds int) string {
	return sp.WebpPreview(checksum, seconds).Find()
}

func (sp *sceneMarkerPaths) GetScreenshotPath(checksum string, seconds int) string {
	return sp.Screenshot(checksum, seconds).Find()
}
