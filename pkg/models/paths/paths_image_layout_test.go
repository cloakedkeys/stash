package paths

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const testChecksum = "0123456789abcdef0123456789abcdef"

// #1139: an image's generated files go together, two folders deep by checksum.
func TestImageFilesGroupedByHash(t *testing.T) {
	root := t.TempDir()
	p := NewPaths(root, "")
	dir := filepath.Join(root, "image", "01", "23")

	require.Equal(t, filepath.Join(dir, testChecksum+"_thumb_640.jpg"), p.Generated.GetThumbnailPath(testChecksum, 640))
	require.Equal(t, filepath.Join(dir, testChecksum+"_preview_640.webm"), p.Generated.GetClipPreviewPath(testChecksum, 640))
}

// Thumbnails made before the hash layout are still found where they are.
func TestImageFilesFoundInLegacyFolder(t *testing.T) {
	root := t.TempDir()
	p := NewPaths(root, "")
	legacyDir := filepath.Join(root, "thumbnails", "01", "23")

	thumb := filepath.Join(legacyDir, testChecksum+"_640.jpg")
	clip := filepath.Join(legacyDir, testChecksum+"_640.webm")
	touch(t, thumb)
	touch(t, clip)
	require.Equal(t, thumb, p.Generated.GetThumbnailPath(testChecksum, 640))
	require.Equal(t, clip, p.Generated.GetClipPreviewPath(testChecksum, 640))

	// a file in the hash layout comes first
	newThumb := p.Generated.Thumbnail(testChecksum, 640).Path
	touch(t, newThumb)
	require.Equal(t, newThumb, p.Generated.GetThumbnailPath(testChecksum, 640))
}
