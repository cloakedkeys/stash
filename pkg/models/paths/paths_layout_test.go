package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const testHash = "0123456789abcdef"

func touch(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(path), 0o644))
}

// #1139: a video's generated files go together, two folders deep by hash.
func TestVideoFilesGroupedByHash(t *testing.T) {
	root := t.TempDir()
	p := NewPaths(root, "")
	dir := filepath.Join(root, "video", "01", "23")

	for want, got := range map[string]string{
		"0123456789abcdef_preview.mp4":    p.Scene.GetVideoPreviewPath(testHash),
		"0123456789abcdef_preview.webp":   p.Scene.GetWebpPreviewPath(testHash),
		"0123456789abcdef_sprite.jpg":     p.Scene.GetSpriteImageFilePath(testHash),
		"0123456789abcdef_thumbs.vtt":     p.Scene.GetSpriteVttFilePath(testHash),
		"0123456789abcdef_heatmap.png":    p.Scene.GetInteractiveHeatmapPath(testHash),
		"0123456789abcdef_transcode.mp4":  p.Scene.GetTranscodePath(testHash),
		"0123456789abcdef_marker_30.mp4":  p.SceneMarkers.GetVideoPreviewPath(testHash, 30),
		"0123456789abcdef_marker_30.webp": p.SceneMarkers.GetWebpPreviewPath(testHash, 30),
		"0123456789abcdef_marker_30.jpg":  p.SceneMarkers.GetScreenshotPath(testHash, 30),
	} {
		require.Equal(t, filepath.Join(dir, want), got)
	}
}

// Files made before the hash layout are still found where they are.
func TestVideoFilesFoundInLegacyFolders(t *testing.T) {
	root := t.TempDir()
	p := NewPaths(root, "")

	legacy := map[string]func() string{
		filepath.Join("screenshots", testHash+".mp4"):          func() string { return p.Scene.GetVideoPreviewPath(testHash) },
		filepath.Join("screenshots", testHash+".webp"):         func() string { return p.Scene.GetWebpPreviewPath(testHash) },
		filepath.Join("vtt", testHash+"_sprite.jpg"):           func() string { return p.Scene.GetSpriteImageFilePath(testHash) },
		filepath.Join("vtt", testHash+"_thumbs.vtt"):           func() string { return p.Scene.GetSpriteVttFilePath(testHash) },
		filepath.Join("interactive_heatmaps", testHash+".png"): func() string { return p.Scene.GetInteractiveHeatmapPath(testHash) },
		filepath.Join("transcodes", testHash+".mp4"):           func() string { return p.Scene.GetTranscodePath(testHash) },
		filepath.Join("markers", testHash, "30.mp4"):           func() string { return p.SceneMarkers.GetVideoPreviewPath(testHash, 30) },
		filepath.Join("markers", testHash, "30.webp"):          func() string { return p.SceneMarkers.GetWebpPreviewPath(testHash, 30) },
		filepath.Join("markers", testHash, "30.jpg"):           func() string { return p.SceneMarkers.GetScreenshotPath(testHash, 30) },
	}
	for rel, get := range legacy {
		touch(t, filepath.Join(root, rel))
		require.Equal(t, filepath.Join(root, rel), get(), rel)
	}

	// a file in the hash layout comes first
	newPreview := p.Scene.VideoPreview(testHash).Path
	touch(t, newPreview)
	require.Equal(t, newPreview, p.Scene.GetVideoPreviewPath(testHash))
}

func TestListMarkerFiles(t *testing.T) {
	p := NewPaths(t.TempDir(), "")
	mine := []string{
		p.SceneMarkers.VideoPreview(testHash, 5).Path,
		p.SceneMarkers.Screenshot(testHash, 70).Path,
	}
	for _, f := range mine {
		touch(t, f)
	}
	// in the same folder, but not this video's markers
	touch(t, p.Scene.SpriteImage(testHash).Path)
	touch(t, p.SceneMarkers.VideoPreview(testHash+"0123456789abcdef", 5).Path)

	require.ElementsMatch(t, mine, p.SceneMarkers.ListMarkerFiles(testHash))

	moved := p.SceneMarkers.RenamedMarkerFile(mine[0], testHash, "fedcba9876543210")
	require.Equal(t, p.SceneMarkers.VideoPreview("fedcba9876543210", 5).Path, moved)
}
