package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

func writeContent(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func requireContent(t *testing.T, path, content string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err, path)
	require.Equal(t, content, string(got), path)
}

// #1139: the move task moves every kind of generated file from its legacy
// folder into the hash layout, and leaves everything else alone.
func TestMigrateGeneratedLayout(t *testing.T) {
	const (
		oshash   = "0123456789abcdef"
		md5      = "fedcba9876543210fedcba9876543210" // a scene named after its MD5
		checksum = "0123456789abcdef0123456789abcdef" // an image
		done     = "1111222233334444"                 // already has a preview in the new layout
	)
	root := t.TempDir()
	p := paths.NewPaths(root, "")

	var files []paths.GeneratedFile
	for _, h := range []string{oshash, md5} {
		files = append(files, p.Scene.GeneratedFiles(h)...)
		files = append(files, p.SceneMarkers.MarkerFiles(h, 30)...)
		files = append(files, p.SceneMarkers.MarkerFiles(h, 0)...)
	}
	files = append(files,
		p.Generated.Thumbnail(checksum, 640), p.Generated.ClipPreview(checksum, 640),
		p.Generated.Thumbnail(checksum, 1280),
	)
	for _, f := range files {
		writeContent(t, f.Legacy, f.Legacy)
	}

	// already moved: the new layout's file is kept, and the legacy one left
	donePreview := p.Scene.VideoPreview(done)
	writeContent(t, donePreview.Legacy, "old")
	writeContent(t, donePreview.Path, "new")

	// not generated files of the hash layout
	others := []string{
		p.Scene.GetLegacyScreenshotPath(oshash), // goes to blobs, by its own migration
		filepath.Join(p.Generated.Vtt, "notes.txt"),
		filepath.Join(p.Generated.Screenshots, "foo.mp4"), // where a preview of a video hashed "foo" would be
		filepath.Join(p.Generated.Markers, "foo", "30.mp4"),
		filepath.Join(p.Generated.Markers, oshash+"x", "30.mp4"),
		filepath.Join(p.Generated.Markers, md5, "notes.txt"),
		filepath.Join(p.Generated.Thumbnails, "ff", "ff", checksum+"_640.jpg"), // in the wrong folder
		filepath.Join(p.Generated.Thumbnails, "01", "23", checksum+"_big.jpg"),
	}
	for _, o := range others {
		writeContent(t, o, "other")
	}

	j := MigrateGeneratedLayoutJob{Paths: &p}
	require.NoError(t, j.Execute(context.Background(), nil))

	for _, f := range files {
		require.NoFileExists(t, f.Legacy)
		requireContent(t, f.Path, f.Legacy)
	}
	requireContent(t, donePreview.Path, "new")
	requireContent(t, donePreview.Legacy, "old")
	for _, o := range others {
		requireContent(t, o, "other")
	}
	require.Equal(t, len(files), j.moved)
	require.Equal(t, 1, j.alreadyMoved)
	require.Zero(t, j.failed)

	// emptied legacy folders are removed, unless something else is in them
	require.NoDirExists(t, p.SceneMarkers.GetFolderPath(oshash))
	require.DirExists(t, p.SceneMarkers.GetFolderPath(md5))
	require.DirExists(t, filepath.Join(p.Generated.Thumbnails, "01", "23"))

	// running it again changes nothing
	j = MigrateGeneratedLayoutJob{Paths: &p}
	require.NoError(t, j.Execute(context.Background(), nil))
	require.Zero(t, j.moved)
	require.Equal(t, 1, j.alreadyMoved)
}

// The image folders left empty are removed.
func TestMigrateGeneratedLayoutRemovesEmptyImageFolders(t *testing.T) {
	const checksum = "0123456789abcdef0123456789abcdef"
	p := paths.NewPaths(t.TempDir(), "")
	thumb := p.Generated.Thumbnail(checksum, 640)
	writeContent(t, thumb.Legacy, "x")

	j := MigrateGeneratedLayoutJob{Paths: &p}
	require.NoError(t, j.Execute(context.Background(), nil))

	requireContent(t, thumb.Path, "x")
	require.NoDirExists(t, filepath.Join(p.Generated.Thumbnails, "01"))
	require.DirExists(t, p.Generated.Thumbnails)
}

// A stopped task moves nothing more.
func TestMigrateGeneratedLayoutCancelled(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	preview := p.Scene.VideoPreview("0123456789abcdef")
	writeContent(t, preview.Legacy, "x")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j := MigrateGeneratedLayoutJob{Paths: &p}
	require.NoError(t, j.Execute(ctx, nil))

	requireContent(t, preview.Legacy, "x")
	require.Zero(t, j.moved)
}

// A library started after the hash layout has no legacy folders.
func TestMigrateGeneratedLayoutFreshLibrary(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	j := MigrateGeneratedLayoutJob{Paths: &p}
	require.NoError(t, j.Execute(context.Background(), nil))
	require.Zero(t, j.moved+j.alreadyMoved+j.failed)
}
