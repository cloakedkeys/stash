package paths

import (
	"path/filepath"
	"strconv"

	"github.com/stashapp/stash/pkg/fsutil"
)

type scenePaths struct {
	generatedPaths
}

func newScenePaths(p Paths) *scenePaths {
	sp := scenePaths{
		generatedPaths: *p.Generated,
	}
	return &sp
}

func (sp *scenePaths) GetLegacyScreenshotPath(checksum string) string {
	return filepath.Join(sp.Screenshots, checksum+".jpg")
}

// Each of a video's generated files, in the hash layout and where it was kept before.

func (sp *scenePaths) Transcode(checksum string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, "transcode.mp4"),
		Legacy: filepath.Join(sp.Transcodes, checksum+".mp4"),
	}
}

func (sp *scenePaths) VideoPreview(checksum string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, "preview.mp4"),
		Legacy: filepath.Join(sp.Screenshots, checksum+".mp4"),
	}
}

func (sp *scenePaths) WebpPreview(checksum string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, "preview.webp"),
		Legacy: filepath.Join(sp.Screenshots, checksum+".webp"),
	}
}

func (sp *scenePaths) SpriteImage(checksum string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, "sprite.jpg"),
		Legacy: filepath.Join(sp.Vtt, checksum+"_sprite.jpg"),
	}
}

func (sp *scenePaths) SpriteVtt(checksum string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, "thumbs.vtt"),
		Legacy: filepath.Join(sp.Vtt, checksum+"_thumbs.vtt"),
	}
}

func (sp *scenePaths) Heatmap(checksum string) GeneratedFile {
	return GeneratedFile{
		Path:   sp.videoFile(checksum, "heatmap.png"),
		Legacy: filepath.Join(sp.InteractiveHeatmap, checksum+".png"),
	}
}

// GeneratedFiles returns all of a video's generated files, apart from its
// markers' files.
func (sp *scenePaths) GeneratedFiles(checksum string) []GeneratedFile {
	return []GeneratedFile{
		sp.VideoPreview(checksum),
		sp.WebpPreview(checksum),
		sp.Transcode(checksum),
		sp.SpriteVtt(checksum),
		sp.SpriteImage(checksum),
		sp.Heatmap(checksum),
	}
}

// The Get...Path functions return where the file is, or where a new one goes.
// See GeneratedFile.Find.

func (sp *scenePaths) GetTranscodePath(checksum string) string {
	return sp.Transcode(checksum).Find()
}

func (sp *scenePaths) GetStreamPath(scenePath string, checksum string) string {
	transcodePath := sp.GetTranscodePath(checksum)
	transcodeExists, _ := fsutil.FileExists(transcodePath)
	if transcodeExists {
		return transcodePath
	}
	return scenePath
}

func (sp *scenePaths) GetVideoPreviewPath(checksum string) string {
	return sp.VideoPreview(checksum).Find()
}

func (sp *scenePaths) GetWebpPreviewPath(checksum string) string {
	return sp.WebpPreview(checksum).Find()
}

func (sp *scenePaths) GetSpriteImageFilePath(checksum string) string {
	return sp.SpriteImage(checksum).Find()
}

func (sp *scenePaths) GetSpriteVttFilePath(checksum string) string {
	return sp.SpriteVtt(checksum).Find()
}

func (sp *scenePaths) GetInteractiveHeatmapPath(checksum string) string {
	return sp.Heatmap(checksum).Find()
}

// GetEmbeddedSubtitleDir uses only a numeric database ID, never a media path.
func (sp *scenePaths) GetEmbeddedSubtitleDir(sceneID int) string {
	return filepath.Join(sp.EmbeddedSubtitles, strconv.Itoa(sceneID))
}
