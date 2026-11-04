package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/pkg/file"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/paths"
	"github.com/stretchr/testify/require"
)

const (
	layoutHash    = "0123456789abcdef"
	layoutNewHash = "fedcba9876543210"
)

func writeGenerated(t *testing.T, files ...string) {
	t.Helper()
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte(filepath.Base(f)), 0o644))
	}
}

// allGenerated returns every generated file of the video with hash, with a
// marker at 30 seconds, in both layouts.
func allGenerated(p paths.Paths, hash string) []string {
	var ret []string
	for _, f := range p.Scene.GeneratedFiles(hash) {
		ret = append(ret, f.Both()...)
	}
	for _, f := range p.SceneMarkers.MarkerFiles(hash, 30) {
		ret = append(ret, f.Both()...)
	}
	return ret
}

// #1139: deleting a scene deletes its generated files in both layouts.
func TestMarkGeneratedFilesBothLayouts(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	files := allGenerated(p, layoutHash)
	writeGenerated(t, files...)
	other := allGenerated(p, layoutNewHash)
	writeGenerated(t, other...)

	d := FileDeleter{Deleter: file.NewDeleter(), Paths: &p, FileNamingAlgo: models.HashAlgorithmOshash}
	require.NoError(t, d.MarkGeneratedFiles(&models.Scene{ID: 1, OSHash: layoutHash}))
	d.Commit()

	for _, f := range files {
		require.NoFileExists(t, f)
	}
	for _, f := range other {
		require.FileExists(t, f, "another video's file")
	}
}

func TestMarkMarkerFilesBothLayouts(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	var at30, at60 []string
	for _, f := range p.SceneMarkers.MarkerFiles(layoutHash, 30) {
		at30 = append(at30, f.Both()...)
	}
	for _, f := range p.SceneMarkers.MarkerFiles(layoutHash, 60) {
		at60 = append(at60, f.Both()...)
	}
	writeGenerated(t, at30...)
	writeGenerated(t, at60...)

	d := FileDeleter{Deleter: file.NewDeleter(), Paths: &p, FileNamingAlgo: models.HashAlgorithmOshash}
	require.NoError(t, d.MarkMarkerFiles(&models.Scene{ID: 1, OSHash: layoutHash}, 30))
	d.Commit()

	for _, f := range at30 {
		require.NoFileExists(t, f)
	}
	for _, f := range at60 {
		require.FileExists(t, f, "another marker's file")
	}
}

// Changing the naming hash renames files in each layout within that layout.
func TestMigrateHashBothLayouts(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	old := allGenerated(p, layoutHash)
	writeGenerated(t, old...)
	vttContents := "WEBVTT\n\n00:00.000 --> 00:05.000\n" + layoutHash + "_sprite.jpg#xywh=0,0,160,90\n"
	for _, vtt := range p.Scene.SpriteVtt(layoutHash).Both() {
		require.NoError(t, os.WriteFile(vtt, []byte(vttContents), 0o644))
	}

	MigrateHash(&p, layoutHash, layoutNewHash)

	for _, f := range old {
		require.NoFileExists(t, f)
	}
	for _, f := range allGenerated(p, layoutNewHash) {
		require.FileExists(t, f)
	}
	for _, vtt := range p.Scene.SpriteVtt(layoutNewHash).Both() {
		contents, err := os.ReadFile(vtt)
		require.NoError(t, err)
		require.Contains(t, string(contents), layoutNewHash+"_sprite.jpg")
		require.NotContains(t, string(contents), layoutHash)
	}
}

func TestInvalidateGeneratedFilesBothLayouts(t *testing.T) {
	p := paths.NewPaths(t.TempDir(), "")
	files := allGenerated(p, layoutHash)
	writeGenerated(t, files...)

	InvalidateGeneratedFiles(&p, layoutHash)

	for _, f := range files {
		require.NoFileExists(t, f)
	}
}
